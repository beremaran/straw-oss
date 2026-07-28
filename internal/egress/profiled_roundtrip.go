package egress

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	utls "github.com/bogdanfinn/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/beremaran/straw-oss/internal/egress/profilecatalog"
)

const (
	profiledDefaultHTTP2StreamID    = 1
	profiledHTTP2Major              = 2
	profiledHTTP2MaxFrameSize       = 16 << 10
	profiledDefaultFlowWindow       = 65535
	profiledDefaultHeaderTableSize  = 4096
	profiledDefaultMaxHeaderList    = 10 << 20
	profiledDefaultConnectionWindow = 15663105
	profiledNextClientStreamIDStep  = 2
	profiledDefaultHeaderWeight     = 255
)

var (
	errProfiledHTTP2GoAway        = errors.New("profiled HTTP/2 GOAWAY")
	errProfiledHTTP2InvalidStatus = errors.New("invalid profiled HTTP/2 response status")
)

func doProfiledRoundTrip(
	ctx context.Context,
	dial func(context.Context, string, string) (net.Conn, error),
	target target,
	profile profilecatalog.ClientProfile,
	tlsConfig profiledTLSConfig,
	req profiledRequest,
) (*http.Response, net.Conn, error) {
	address := net.JoinHostPort(target.host, strconv.FormatUint(uint64(target.port), 10))

	conn, err := dial(ctx, "tcp", address)
	if err != nil {
		return nil, nil, fmt.Errorf("dial profiled target: %w", err)
	}

	closeOnError := func(err error) (*http.Response, net.Conn, error) {
		_ = conn.Close()

		return nil, nil, err
	}

	if req.request.URL.Scheme == schemeHTTPS {
		profiledConn, tlsErr := upgradeProfiledTLS(ctx, conn, target.host, profile.GetClientHelloId(), tlsConfig)
		if tlsErr != nil {
			return closeOnError(tlsErr)
		}

		conn = profiledConn
	}

	stopCancellationWatch := watchProfiledCancellation(ctx, conn)

	response, err := roundTripProfiledProtocol(req, conn, profile)
	if err != nil {
		stopCancellationWatch()

		return closeOnError(err)
	}

	response.Body = &profiledWatchedBody{ReadCloser: response.Body, stop: stopCancellationWatch}

	return response, conn, nil
}

func upgradeProfiledTLS(ctx context.Context, conn net.Conn, host string, profile utls.ClientHelloID, config profiledTLSConfig) (*utls.UConn, error) {
	profiledConn := utls.UClient(conn, &utls.Config{
		ServerName:         host,
		RootCAs:            config.rootCAs,
		InsecureSkipVerify: config.insecureSkipVerify,
		ClientSessionCache: config.sessionCache,
		OmitEmptyPsk:       true,
	}, profile, true, false, true)

	err := profiledConn.HandshakeContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("profiled TLS handshake: %w", err)
	}

	return profiledConn, nil
}

func roundTripProfiledProtocol(req profiledRequest, conn net.Conn, profile profilecatalog.ClientProfile) (*http.Response, error) {
	if stateConn, ok := conn.(interface{ ConnectionState() utls.ConnectionState }); ok && stateConn.ConnectionState().NegotiatedProtocol == http2.NextProtoTLS {
		return doProfiledHTTP2(req, conn, newProfiledHTTP2Config(profile))
	}

	return doProfiledHTTP1(req, conn)
}

func watchProfiledCancellation(ctx context.Context, conn net.Conn) func() {
	done := make(chan struct{})

	var once sync.Once

	stop := func() { once.Do(func() { close(done) }) }

	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	return stop
}

type profiledWatchedBody struct {
	io.ReadCloser
	stop func()
}

func (b *profiledWatchedBody) Close() error {
	b.stop()

	err := b.ReadCloser.Close()
	if err != nil {
		return fmt.Errorf("close profiled response body: %w", err)
	}

	return nil
}

func doProfiledHTTP1(req profiledRequest, conn net.Conn) (*http.Response, error) {
	wire, err := encodeProfiledHTTP1Request(req)
	if err != nil {
		return nil, err
	}

	_, err = conn.Write(wire)
	if err != nil {
		return nil, fmt.Errorf("write profiled HTTP/1.1 request: %w", err)
	}

	return readProfiledHTTP1Response(bufio.NewReader(conn), req.request)
}

func encodeProfiledHTTP1Request(req profiledRequest) ([]byte, error) {
	var wire bytes.Buffer

	requestURI := req.request.URL.RequestURI()
	if requestURI == "" {
		requestURI = "/"
	}

	fmt.Fprintf(&wire, "%s %s HTTP/1.1\r\nHost: %s\r\n", req.request.Method, requestURI, req.request.URL.Host)

	for _, name := range req.headerOrder {
		for _, value := range req.request.Header.Values(name) {
			fmt.Fprintf(&wire, "%s: %s\r\n", http.CanonicalHeaderKey(name), value)
		}
	}

	if req.request.ContentLength > 0 {
		fmt.Fprintf(&wire, "Content-Length: %d\r\n", req.request.ContentLength)
	}

	wire.WriteString("Connection: close\r\n\r\n")

	if req.request.Body != nil {
		body, err := io.ReadAll(req.request.Body)
		if err != nil {
			return nil, fmt.Errorf("read profiled HTTP/1.1 request body: %w", err)
		}

		wire.Write(body)
	}

	return wire.Bytes(), nil
}

func readProfiledHTTP1Response(reader *bufio.Reader, request *http.Request) (*http.Response, error) {
	for {
		response, err := http.ReadResponse(reader, request)
		if err != nil {
			return nil, fmt.Errorf("read profiled HTTP/1.1 response: %w", err)
		}

		if response.StatusCode >= 200 || response.StatusCode == http.StatusSwitchingProtocols {
			return response, nil
		}

		_ = response.Body.Close()
	}
}

type profiledHTTP2Config struct {
	streamID          uint32
	settings          []http2.Setting
	connectionWindow  uint32
	pseudoHeaderOrder []string
	priorities        []profilecatalog.Priority
	headerPriority    http2.PriorityParam
	headerTableSize   uint32
	maxHeaderListSize uint32
}

func newProfiledHTTP2Config(profile profilecatalog.ClientProfile) profiledHTTP2Config {
	settingsMap := profile.GetSettings()
	settingsOrder := profile.GetSettingsOrder()

	settings := make([]http2.Setting, 0, len(settingsOrder))
	for _, id := range settingsOrder {
		settings = append(settings, http2.Setting{ID: id, Val: settingsMap[id]})
	}

	headerTableSize := uint32(profiledDefaultHeaderTableSize)
	if value, ok := settingsMap[http2.SettingHeaderTableSize]; ok {
		headerTableSize = value
	}

	maxHeaderListSize := uint32(profiledDefaultMaxHeaderList)
	if value := settingsMap[http2.SettingMaxHeaderListSize]; value != 0 {
		maxHeaderListSize = value
		if value == ^uint32(0) {
			maxHeaderListSize = 0
		}
	}

	connectionWindow := profile.GetConnectionFlow()
	if connectionWindow == 0 {
		connectionWindow = profiledDefaultConnectionWindow
	}

	streamID := profile.GetStreamID()
	if streamID == 0 {
		streamID = profiledDefaultHTTP2StreamID
	}

	for _, priority := range profile.GetPriorities() {
		streamID = priority.StreamID + profiledNextClientStreamIDStep
	}

	headerPriority := http2.PriorityParam{Exclusive: true, Weight: profiledDefaultHeaderWeight}
	if configured := profile.GetHeaderPriority(); configured != nil {
		headerPriority = *configured
	}

	return profiledHTTP2Config{
		streamID:          streamID,
		settings:          settings,
		connectionWindow:  connectionWindow,
		pseudoHeaderOrder: append([]string(nil), profile.GetPseudoHeaderOrder()...),
		priorities:        append([]profilecatalog.Priority(nil), profile.GetPriorities()...),
		headerPriority:    headerPriority,
		headerTableSize:   headerTableSize,
		maxHeaderListSize: maxHeaderListSize,
	}
}

func doProfiledHTTP2(req profiledRequest, conn net.Conn, config profiledHTTP2Config) (*http.Response, error) {
	framer := http2.NewFramer(conn, conn)
	framer.ReadMetaHeaders = hpack.NewDecoder(config.headerTableSize, nil)
	framer.MaxHeaderListSize = config.maxHeaderListSize

	err := initializeProfiledHTTP2(framer, conn, config)
	if err != nil {
		return nil, err
	}

	headerBlock, err := encodeProfiledHTTP2Headers(req, config.pseudoHeaderOrder)
	if err != nil {
		return nil, err
	}

	hasBody := req.request.Body != nil && req.request.ContentLength != 0

	err = framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      config.streamID,
		BlockFragment: headerBlock,
		EndHeaders:    true,
		EndStream:     !hasBody,
		Priority:      config.headerPriority,
	})
	if err != nil {
		return nil, fmt.Errorf("write profiled HTTP/2 request headers: %w", err)
	}

	var earlyResponse *http2.MetaHeadersFrame

	if hasBody {
		earlyResponse, err = sendProfiledHTTP2Body(framer, config.streamID, req.request.Body)
		if err != nil {
			return nil, err
		}
	}

	return readProfiledHTTP2Response(req.request, framer, conn, config.streamID, earlyResponse)
}

func initializeProfiledHTTP2(framer *http2.Framer, conn net.Conn, config profiledHTTP2Config) error {
	_, err := io.WriteString(conn, http2.ClientPreface)
	if err != nil {
		return fmt.Errorf("write profiled HTTP/2 preface: %w", err)
	}

	err = framer.WriteSettings(config.settings...)
	if err != nil {
		return fmt.Errorf("write profiled HTTP/2 settings: %w", err)
	}

	if config.connectionWindow > 0 {
		err = framer.WriteWindowUpdate(0, config.connectionWindow)
		if err != nil {
			return fmt.Errorf("write profiled HTTP/2 connection window: %w", err)
		}
	}

	for _, priority := range config.priorities {
		err = framer.WritePriority(priority.StreamID, priority.PriorityParam)
		if err != nil {
			return fmt.Errorf("write profiled HTTP/2 priority: %w", err)
		}
	}

	return nil
}

func sendProfiledHTTP2Body(framer *http2.Framer, streamID uint32, bodyReader io.Reader) (*http2.MetaHeadersFrame, error) {
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, fmt.Errorf("read profiled HTTP/2 request body: %w", err)
	}

	return writeProfiledHTTP2Data(framer, streamID, body)
}

func encodeProfiledHTTP2Headers(req profiledRequest, pseudoHeaderOrder []string) ([]byte, error) {
	var block bytes.Buffer

	encoder := hpack.NewEncoder(&block)

	pseudoHeaders := map[string]string{
		":method":    req.request.Method,
		":authority": req.request.URL.Host,
		":scheme":    req.request.URL.Scheme,
		":path":      req.request.URL.RequestURI(),
	}
	for _, name := range pseudoHeaderOrder {
		value, ok := pseudoHeaders[name]
		if !ok {
			continue
		}

		err := encoder.WriteField(hpack.HeaderField{Name: name, Value: value})
		if err != nil {
			return nil, fmt.Errorf("encode profiled HTTP/2 pseudo-header: %w", err)
		}
	}

	for _, name := range req.headerOrder {
		for _, value := range req.request.Header.Values(name) {
			err := encoder.WriteField(hpack.HeaderField{Name: strings.ToLower(name), Value: value})
			if err != nil {
				return nil, fmt.Errorf("encode profiled HTTP/2 header: %w", err)
			}
		}
	}

	if req.request.ContentLength > 0 {
		err := encoder.WriteField(hpack.HeaderField{Name: headerContentLength, Value: strconv.FormatInt(req.request.ContentLength, 10)})
		if err != nil {
			return nil, fmt.Errorf("encode profiled HTTP/2 content length: %w", err)
		}
	}

	return block.Bytes(), nil
}

func writeProfiledHTTP2Data(framer *http2.Framer, streamID uint32, body []byte) (*http2.MetaHeadersFrame, error) {
	flow := profiledSendFlow{
		connection:    profiledDefaultFlowWindow,
		stream:        profiledDefaultFlowWindow,
		initialStream: profiledDefaultFlowWindow,
		maxFrame:      profiledHTTP2MaxFrameSize,
	}

	for !flow.seenSettings {
		early, err := readProfiledSendControl(framer, streamID, &flow)
		if err != nil || early != nil {
			return early, err
		}
	}

	for len(body) > 0 {
		available := min(flow.connection, flow.stream, flow.maxFrame, len(body))
		if available == 0 {
			early, err := readProfiledSendControl(framer, streamID, &flow)
			if err != nil || early != nil {
				return early, err
			}

			continue
		}

		end := available == len(body)

		err := framer.WriteData(streamID, end, body[:available])
		if err != nil {
			return nil, fmt.Errorf("write profiled HTTP/2 request body: %w", err)
		}

		flow.connection -= available
		flow.stream -= available
		body = body[available:]
	}

	return nil, nil
}

type profiledSendFlow struct {
	connection    int
	stream        int
	initialStream int
	maxFrame      int
	seenSettings  bool
}

func readProfiledSendControl(framer *http2.Framer, streamID uint32, flow *profiledSendFlow) (*http2.MetaHeadersFrame, error) {
	frame, err := framer.ReadFrame()
	if err != nil {
		return nil, fmt.Errorf("read profiled HTTP/2 send control: %w", err)
	}

	return processProfiledSendControl(framer, streamID, flow, frame)
}

func processProfiledSendControl(framer *http2.Framer, streamID uint32, flow *profiledSendFlow, incoming http2.Frame) (*http2.MetaHeadersFrame, error) {
	switch frame := incoming.(type) {
	case *http2.SettingsFrame:
		return nil, applyProfiledPeerSettings(framer, flow, frame)
	case *http2.WindowUpdateFrame:
		applyProfiledWindowUpdate(streamID, flow, frame)
	case *http2.PingFrame:
		return nil, acknowledgeProfiledPing(framer, frame)
	case *http2.RSTStreamFrame:
		return nil, profiledResetError(streamID, frame)
	case *http2.GoAwayFrame:
		return nil, fmt.Errorf("%w: %s", errProfiledHTTP2GoAway, frame.ErrCode)
	case *http2.MetaHeadersFrame:
		return finalProfiledResponseHeaders(streamID, frame)
	}

	return nil, nil
}

func finalProfiledResponseHeaders(streamID uint32, frame *http2.MetaHeadersFrame) (*http2.MetaHeadersFrame, error) {
	if frame.StreamID != streamID {
		return nil, nil
	}

	status, err := strconv.Atoi(frame.PseudoValue("status"))
	if err != nil || status < http.StatusContinue || status > 999 {
		return nil, errProfiledHTTP2InvalidStatus
	}

	if status < http.StatusOK {
		return nil, nil
	}

	return frame, nil
}

func applyProfiledPeerSettings(framer *http2.Framer, flow *profiledSendFlow, frame *http2.SettingsFrame) error {
	if frame.IsAck() {
		return nil
	}

	err := frame.ForeachSetting(func(setting http2.Setting) error {
		switch setting.ID {
		case http2.SettingInitialWindowSize:
			flow.stream += int(setting.Val) - flow.initialStream
			flow.initialStream = int(setting.Val)
		case http2.SettingMaxFrameSize:
			flow.maxFrame = int(setting.Val)
		case http2.SettingHeaderTableSize,
			http2.SettingEnablePush,
			http2.SettingMaxConcurrentStreams,
			http2.SettingMaxHeaderListSize,
			http2.SettingEnableConnectProtocol,
			http2.SettingNoRFC7540Priorities:
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("apply profiled HTTP/2 peer settings: %w", err)
	}

	flow.seenSettings = true

	err = framer.WriteSettingsAck()
	if err != nil {
		return fmt.Errorf("acknowledge profiled HTTP/2 settings: %w", err)
	}

	return nil
}

func applyProfiledWindowUpdate(streamID uint32, flow *profiledSendFlow, frame *http2.WindowUpdateFrame) {
	switch frame.StreamID {
	case 0:
		flow.connection += int(frame.Increment)
	case streamID:
		flow.stream += int(frame.Increment)
	}
}

func acknowledgeProfiledPing(framer *http2.Framer, frame *http2.PingFrame) error {
	if frame.Flags.Has(http2.FlagPingAck) {
		return nil
	}

	err := framer.WritePing(true, frame.Data)
	if err != nil {
		return fmt.Errorf("acknowledge profiled HTTP/2 ping: %w", err)
	}

	return nil
}

func profiledResetError(streamID uint32, frame *http2.RSTStreamFrame) error {
	if frame.StreamID != streamID {
		return nil
	}

	return http2.StreamError{StreamID: frame.StreamID, Code: frame.ErrCode}
}

func readProfiledHTTP2Response(request *http.Request, framer *http2.Framer, conn net.Conn, streamID uint32, initial *http2.MetaHeadersFrame) (*http.Response, error) {
	for {
		frame, remainingInitial, err := nextProfiledHTTP2Frame(framer, initial)
		if err != nil {
			return nil, err
		}

		initial = remainingInitial

		response, err := processProfiledHTTP2ResponseFrame(request, framer, conn, streamID, frame)
		if err != nil {
			return nil, err
		}

		if response != nil {
			return response, nil
		}
	}
}

func nextProfiledHTTP2Frame(framer *http2.Framer, initial *http2.MetaHeadersFrame) (http2.Frame, *http2.MetaHeadersFrame, error) {
	if initial != nil {
		return initial, nil, nil
	}

	frame, err := framer.ReadFrame()
	if err != nil {
		return nil, nil, fmt.Errorf("read profiled HTTP/2 response frame: %w", err)
	}

	return frame, nil, nil
}

func processProfiledHTTP2ResponseFrame(request *http.Request, framer *http2.Framer, conn net.Conn, streamID uint32, incoming http2.Frame) (*http.Response, error) {
	switch frame := incoming.(type) {
	case *http2.SettingsFrame:
		return nil, acknowledgeProfiledSettings(framer, frame)
	case *http2.PingFrame:
		return nil, acknowledgeProfiledPing(framer, frame)
	case *http2.RSTStreamFrame:
		return nil, profiledResetError(streamID, frame)
	case *http2.GoAwayFrame:
		return nil, fmt.Errorf("%w: %s", errProfiledHTTP2GoAway, frame.ErrCode)
	case *http2.MetaHeadersFrame:
		return buildProfiledHTTP2Response(request, framer, conn, streamID, frame)
	}

	return nil, nil
}

func acknowledgeProfiledSettings(framer *http2.Framer, frame *http2.SettingsFrame) error {
	if frame.IsAck() {
		return nil
	}

	err := framer.WriteSettingsAck()
	if err != nil {
		return fmt.Errorf("acknowledge profiled HTTP/2 response settings: %w", err)
	}

	return nil
}

func buildProfiledHTTP2Response(request *http.Request, framer *http2.Framer, conn net.Conn, streamID uint32, frame *http2.MetaHeadersFrame) (*http.Response, error) {
	if frame.StreamID != streamID {
		return nil, nil
	}

	status, err := strconv.Atoi(frame.PseudoValue("status"))
	if err != nil || status < http.StatusContinue || status > 999 {
		return nil, errProfiledHTTP2InvalidStatus
	}

	if status < http.StatusOK {
		return nil, nil
	}

	headers := make(http.Header)

	for _, field := range frame.RegularFields() {
		headers.Add(field.Name, field.Value)
	}

	trailers := make(http.Header)
	body := &profiledHTTP2Body{
		framer:   framer,
		conn:     conn,
		streamID: streamID,
		trailers: trailers,
		done:     frame.StreamEnded(),
	}

	return &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Proto:      httpProtocol20,
		ProtoMajor: profiledHTTP2Major,
		Header:     headers,
		Trailer:    trailers,
		Body:       body,
		Request:    request,
	}, nil
}

type profiledHTTP2Body struct {
	framer   *http2.Framer
	conn     net.Conn
	streamID uint32
	trailers http.Header
	pending  []byte
	done     bool
}

func (b *profiledHTTP2Body) Read(destination []byte) (int, error) {
	if len(b.pending) > 0 {
		return b.consumePending(destination), nil
	}

	if b.done {
		return 0, io.EOF
	}

	for {
		frame, err := b.framer.ReadFrame()
		if err != nil {
			return 0, fmt.Errorf("read profiled HTTP/2 response body frame: %w", err)
		}

		err = b.processFrame(frame)
		if err != nil {
			return 0, err
		}

		if len(b.pending) > 0 {
			return b.consumePending(destination), nil
		}

		if b.done {
			return 0, io.EOF
		}
	}
}

func (b *profiledHTTP2Body) Close() error {
	err := b.conn.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close profiled HTTP/2 connection: %w", err)
	}

	return nil
}

func (b *profiledHTTP2Body) consumePending(destination []byte) int {
	n := copy(destination, b.pending)
	b.pending = b.pending[n:]

	return n
}

func (b *profiledHTTP2Body) processFrame(incoming http2.Frame) error {
	switch frame := incoming.(type) {
	case *http2.DataFrame:
		b.acceptData(frame)
	case *http2.MetaHeadersFrame:
		b.acceptTrailers(frame)
	case *http2.SettingsFrame:
		return acknowledgeProfiledSettings(b.framer, frame)
	case *http2.PingFrame:
		return acknowledgeProfiledPing(b.framer, frame)
	case *http2.RSTStreamFrame:
		return profiledResetError(b.streamID, frame)
	case *http2.GoAwayFrame:
		return fmt.Errorf("%w: %s", errProfiledHTTP2GoAway, frame.ErrCode)
	}

	return nil
}

func (b *profiledHTTP2Body) acceptData(frame *http2.DataFrame) {
	if frame.StreamID != b.streamID {
		return
	}

	data := frame.Data()
	if len(data) > 0 {
		b.pending = append(b.pending[:0], data...)
	}

	b.done = frame.StreamEnded()
}

func (b *profiledHTTP2Body) acceptTrailers(frame *http2.MetaHeadersFrame) {
	if frame.StreamID != b.streamID {
		return
	}

	for _, field := range frame.RegularFields() {
		b.trailers.Add(field.Name, field.Value)
	}

	b.done = frame.StreamEnded()
}
