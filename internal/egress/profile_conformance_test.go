package egress

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/beremaran/straw-oss/internal/fingerprint"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	profileGoldenGREASE  = "GREASE"
	profileObserverHost  = "profile.observer.test"
	priorityBehaviorNone = "none"
)

// Keep the wire contract next to the observer so a registry edit cannot make
// the conformance test pass without exercising the actual client.
var (
	//go:embed testdata/chrome_120_profile_catalog_v1_15_1.json
	chrome120GoldenJSON []byte
)

func TestProfileConformanceChrome120MatchesGoldenOnLocalWireAndDiffersFromBaseline(t *testing.T) {
	golden := loadChrome120Golden(t)

	named := runProfileObservation(t, chrome120FingerprintProfile)
	if named.protocol != http2.NextProtoTLS {
		t.Fatalf("named protocol = %q, want h2", named.protocol)
	}
	if named.serverName != profileObserverHost {
		t.Fatalf("named SNI = %q, want profile.observer.test", named.serverName)
	}
	compareTLSObservation(t, named.hello, golden.TLS)
	compareHTTP2Observation(t, named, golden.HTTP2)

	baseline := runProfileObservation(t, "")
	if baseline.protocol == http2.NextProtoTLS {
		t.Fatalf("baseline negotiated h2, want baseline HTTP/1.1")
	}
	if slices.Equal(named.hello.CipherSuites, baseline.hello.CipherSuites) {
		t.Fatalf("named and baseline cipher suites are identical: %v", named.hello.CipherSuites)
	}
	if slices.Equal(sortedStrings(named.hello.Extensions), sortedStrings(baseline.hello.Extensions)) {
		t.Fatalf("named and baseline extension sets are identical: %v", named.hello.Extensions)
	}
	for name, same := range map[string]bool{
		"supported versions":   slices.Equal(named.hello.SupportedVersions, baseline.hello.SupportedVersions),
		"supported groups":     slices.Equal(named.hello.SupportedGroups, baseline.hello.SupportedGroups),
		"signature algorithms": slices.Equal(named.hello.SignatureAlgorithms, baseline.hello.SignatureAlgorithms),
		"key shares":           slices.Equal(named.hello.KeyShares, baseline.hello.KeyShares),
		"ALPN":                 slices.Equal(named.hello.ALPN, baseline.hello.ALPN),
	} {
		if same {
			t.Errorf("named and baseline %s are identical", name)
		}
	}
	if len(baseline.settings) != 0 || baseline.connectionWindow != 0 || len(baseline.pseudoHeaderOrder) != 0 || len(baseline.applicationHeaderOrder) != 0 {
		t.Fatalf("baseline exposed profiled HTTP/2 dimensions: %+v", baseline)
	}
}

func TestProfileCatalogueRepresentativeFamiliesMatchProfileDrivenHTTP2Wire(t *testing.T) {
	t.Parallel()

	representatives := []string{
		"brave_146",
		"chrome_120",
		"firefox_120",
		"okhttp4_android_13",
		"opera_91",
		"safari_ios_17_0",
	}
	seenTLS := make(map[string]string, len(representatives))
	for _, name := range representatives {
		t.Run(name, func(t *testing.T) {
			observation := runProfileObservation(t, name)
			profile := executableFingerprintProfiles[name]
			config := newProfiledHTTP2Config(profile)

			signature := fmt.Sprintf("%v|%v|%v|%v|%v", observation.hello.CipherSuites, observation.hello.Extensions, observation.hello.SupportedGroups, observation.hello.SignatureAlgorithms, observation.hello.ALPN)
			if previous, duplicate := seenTLS[signature]; duplicate {
				t.Fatalf("TLS wire signature duplicates representative %q", previous)
			}
			seenTLS[signature] = name

			if observation.protocol != http2.NextProtoTLS {
				return
			}

			wantSettings := make([]goldenSetting, 0, len(config.settings))
			for _, setting := range config.settings {
				wantSettings = append(wantSettings, goldenSetting{ID: setting.ID.String(), Value: setting.Val})
			}
			if !slices.Equal(observation.settings, wantSettings) {
				t.Fatalf("settings = %v, want %v", observation.settings, wantSettings)
			}
			if observation.connectionWindow != config.connectionWindow {
				t.Fatalf("connection window = %d, want %d", observation.connectionWindow, config.connectionWindow)
			}
			if !slices.Equal(observation.pseudoHeaderOrder, config.pseudoHeaderOrder) {
				t.Fatalf("pseudo-header order = %v, want %v", observation.pseudoHeaderOrder, config.pseudoHeaderOrder)
			}
			if observation.prioritySeen != (len(config.priorities) > 0) {
				t.Fatalf("priority frames seen = %t, configured = %d", observation.prioritySeen, len(config.priorities))
			}
		})
	}
}

func TestEveryAdvertisedProfileCompletesALocalWireRequest(t *testing.T) {
	for _, name := range fingerprint.Names() {
		t.Run(name, func(t *testing.T) {
			observation := runProfileObservation(t, name)
			if observation.serverName != profileObserverHost {
				t.Fatalf("SNI = %q, want profile.observer.test", observation.serverName)
			}
			if len(observation.hello.CipherSuites) == 0 {
				t.Fatal("ClientHello advertised no cipher suites")
			}
		})
	}
}

func TestPSKProfileResumesOnlyAfterReceivingASessionTicket(t *testing.T) {
	const profile = "chrome_144_PSK"

	observer := newConformanceObserverConnections(t, 2)
	exec := NewExecutor(ExecutorOptions{
		Resolver:           staticResolver{profileObserverHost: netip.MustParseAddr("127.0.0.1")},
		InsecureSkipVerify: true,
	})

	first := executeProfileObservation(t, exec, observer, profile)
	if slices.Contains(first.hello.Extensions, "pre_shared_key") {
		t.Fatal("first ClientHello advertised an empty pre-shared key")
	}

	second := executeProfileObservation(t, exec, observer, profile)
	if !slices.Contains(second.hello.Extensions, "pre_shared_key") {
		t.Fatalf("second ClientHello extensions = %v, want pre_shared_key after session ticket", second.hello.Extensions)
	}
}

type chrome120Golden struct {
	TLS   goldenTLS   `json:"tls"`
	HTTP2 goldenHTTP2 `json:"http2"`
}

type goldenTLS struct {
	CipherSuites        []string         `json:"cipher_suites"`
	Extensions          []string         `json:"extensions"`
	SupportedVersions   []string         `json:"supported_versions"`
	SupportedGroups     []string         `json:"supported_groups"`
	SignatureAlgorithms []string         `json:"signature_algorithms"`
	KeyShares           []goldenKeyShare `json:"key_shares"`
	ALPN                []string         `json:"alpn"`
}

type goldenKeyShare struct {
	Group             string `json:"group"`
	KeyExchangeLength int    `json:"key_exchange_length"`
}

type goldenHTTP2 struct {
	Settings               []goldenSetting `json:"settings"`
	ConnectionWindowUpdate uint32          `json:"connection_window_update"`
	PseudoHeaderOrder      []string        `json:"pseudo_header_order"`
	PriorityBehavior       string          `json:"priority_behavior"`
	ApplicationHeaderOrder []string        `json:"application_header_order"`
}

type goldenSetting struct {
	ID    string `json:"id"`
	Value uint32 `json:"value"`
}

func loadChrome120Golden(t *testing.T) chrome120Golden {
	t.Helper()

	var golden struct {
		TLS   goldenTLS   `json:"tls"`
		HTTP2 goldenHTTP2 `json:"http2"`
	}
	err := json.Unmarshal(chrome120GoldenJSON, &golden)
	if err != nil {
		t.Fatalf("decode chrome_120 golden: %v", err)
	}

	return chrome120Golden{TLS: golden.TLS, HTTP2: golden.HTTP2}
}

type observedClientHello struct {
	CipherSuites        []string
	Extensions          []string
	SupportedVersions   []string
	SupportedGroups     []string
	SignatureAlgorithms []string
	KeyShares           []goldenKeyShare
	ALPN                []string
}

func compareTLSObservation(t *testing.T, got observedClientHello, want goldenTLS) {
	t.Helper()

	if !slices.Equal(got.CipherSuites, want.CipherSuites) {
		t.Fatalf("cipher suites = %v, want %v", got.CipherSuites, want.CipherSuites)
	}
	if !slices.Equal(sortedStrings(got.Extensions), sortedStrings(want.Extensions)) {
		t.Fatalf("normalized extension set = %v, want %v", got.Extensions, want.Extensions)
	}
	if !slices.Equal(got.SupportedVersions, want.SupportedVersions) {
		t.Fatalf("supported versions = %v, want %v", got.SupportedVersions, want.SupportedVersions)
	}
	if !slices.Equal(got.SupportedGroups, want.SupportedGroups) {
		t.Fatalf("supported groups = %v, want %v", got.SupportedGroups, want.SupportedGroups)
	}
	if !slices.Equal(got.SignatureAlgorithms, want.SignatureAlgorithms) {
		t.Fatalf("signature algorithms = %v, want %v", got.SignatureAlgorithms, want.SignatureAlgorithms)
	}
	if !slices.Equal(got.KeyShares, want.KeyShares) {
		t.Fatalf("key shares = %v, want %v", got.KeyShares, want.KeyShares)
	}
	if !slices.Equal(got.ALPN, want.ALPN) {
		t.Fatalf("ALPN = %v, want %v", got.ALPN, want.ALPN)
	}
}

type observedConformance struct {
	hello                  observedClientHello
	serverName             string
	protocol               string
	settings               []goldenSetting
	connectionWindow       uint32
	pseudoHeaderOrder      []string
	applicationHeaderOrder []string
	prioritySeen           bool
}

func compareHTTP2Observation(t *testing.T, got observedConformance, want goldenHTTP2) {
	t.Helper()

	if !slices.Equal(got.settings, want.Settings) {
		t.Fatalf("HTTP/2 settings = %v, want %v", got.settings, want.Settings)
	}
	if got.connectionWindow != want.ConnectionWindowUpdate {
		t.Fatalf("HTTP/2 connection window = %d, want %d", got.connectionWindow, want.ConnectionWindowUpdate)
	}
	if !slices.Equal(got.pseudoHeaderOrder, want.PseudoHeaderOrder) {
		t.Fatalf("pseudo-header order = %v, want %v", got.pseudoHeaderOrder, want.PseudoHeaderOrder)
	}
	if want.PriorityBehavior == priorityBehaviorNone && got.prioritySeen {
		t.Fatal("profile sent an unexpected HTTP/2 priority frame")
	}
	if !slices.Equal(got.applicationHeaderOrder, want.ApplicationHeaderOrder) {
		t.Fatalf("application header order = %v, want %v", got.applicationHeaderOrder, want.ApplicationHeaderOrder)
	}
}

func runProfileObservation(t *testing.T, fingerprint string) observedConformance {
	t.Helper()

	observer := newConformanceObserver(t)
	exec := NewExecutor(ExecutorOptions{
		Resolver:           staticResolver{profileObserverHost: netip.MustParseAddr("127.0.0.1")},
		InsecureSkipVerify: true,
	})

	return executeProfileObservation(t, exec, observer, fingerprint)
}

func executeProfileObservation(t *testing.T, exec *Executor, observer *conformanceObserver, fingerprint string) observedConformance {
	t.Helper()

	start := requestStart(
		fmt.Sprintf("https://%s:%d/", profileObserverHost, observer.port()),
		directPolicy(true),
	)
	start.Method = http.MethodGet
	start.Headers = []*strawpb.Header{
		{Name: "user-agent", Value: []byte("profile-conformance")},
		{Name: "accept", Value: []byte("*/*")},
	}
	start.InjectionOperations = nil
	start.FingerprintInstruction = fingerprint

	frames := exec.Execute(context.Background(), start, nil, 1, nil)
	errFrame := terminalErrorOrNil(frames)
	if errFrame != nil {
		select {
		case result := <-observer.result:
			t.Fatalf("%q profile request error = %#v; observer = %+v", fingerprint, errFrame, result.err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%q profile request error = %#v; observer did not finish", fingerprint, errFrame)
		}
	}

	return observer.wait(t)
}

type conformanceObserver struct {
	listener net.Listener
	config   *tls.Config
	result   chan conformanceResult
	mu       sync.Mutex
	hello    *tls.ClientHelloInfo
}

type conformanceResult struct {
	observation observedConformance
	err         error
}

func newConformanceObserver(t *testing.T) *conformanceObserver {
	return newConformanceObserverConnections(t, 1)
}

func newConformanceObserverConnections(t *testing.T, connections int) *conformanceObserver {
	t.Helper()

	certificateSource := httptest.NewUnstartedServer(nil)
	certificateSource.EnableHTTP2 = true
	certificateSource.StartTLS()
	config := certificateSource.TLS.Clone()
	certificateSource.Close()
	config.NextProtos = []string{http2.NextProtoTLS, profileHTTP11}

	observer := &conformanceObserver{
		result: make(chan conformanceResult, connections),
	}
	config.GetConfigForClient = func(info *tls.ClientHelloInfo) (*tls.Config, error) {
		observer.mu.Lock()
		observer.hello = cloneClientHelloInfo(info)
		observer.mu.Unlock()

		return nil, nil
	}
	observer.config = config

	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for conformance observer: %v", err)
	}
	observer.listener = listener
	t.Cleanup(func() { _ = observer.listener.Close() })
	go observer.serve(connections)

	return observer
}

func (o *conformanceObserver) port() int {
	_, port, err := net.SplitHostPort(o.listener.Addr().String())
	if err != nil {
		panic(err)
	}
	var value int
	_, err = fmt.Sscanf(port, "%d", &value)
	if err != nil {
		panic(err)
	}

	return value
}

func (o *conformanceObserver) wait(t *testing.T) observedConformance {
	t.Helper()

	select {
	case result := <-o.result:
		if result.err != nil {
			t.Fatalf("conformance observer: %v", result.err)
		}

		return result.observation
	case <-time.After(2 * time.Second):
		t.Fatal("conformance observer timed out")

		return observedConformance{}
	}
}

func (o *conformanceObserver) serve(connections int) {
	for range connections {
		o.serveConnection()
	}
}

func (o *conformanceObserver) serveConnection() {
	conn, err := o.listener.Accept()
	if err != nil {
		o.result <- conformanceResult{err: err}

		return
	}
	defer func() { _ = conn.Close() }()

	recorded := &recordingConn{Conn: conn}
	tlsConn := tls.Server(recorded, o.config)
	err = tlsConn.HandshakeContext(context.Background())
	if err != nil {
		o.result <- conformanceResult{err: fmt.Errorf("TLS handshake: %w", err)}

		return
	}
	_ = tlsConn.SetDeadline(time.Now().Add(2 * time.Second))

	o.mu.Lock()
	helloInfo := cloneClientHelloInfo(o.hello)
	o.mu.Unlock()
	if helloInfo == nil {
		o.result <- conformanceResult{err: errors.New("TLS observer did not capture ClientHello")}

		return
	}

	hello, err := parseClientHello(recorded.Bytes())
	if err != nil {
		o.result <- conformanceResult{err: fmt.Errorf("parse ClientHello: %w", err)}

		return
	}
	observation := observedConformance{
		hello:      hello,
		serverName: helloInfo.ServerName,
		protocol:   tlsConn.ConnectionState().NegotiatedProtocol,
	}

	if observation.protocol == http2.NextProtoTLS {
		err = o.observeHTTP2(tlsConn, &observation)
	} else {
		err = observeHTTP1(tlsConn)
	}
	o.result <- conformanceResult{observation: observation, err: err}
}

func (o *conformanceObserver) observeHTTP2(conn net.Conn, observation *observedConformance) error {
	reader := bufio.NewReader(conn)
	preface := make([]byte, len(http2.ClientPreface))
	_, err := io.ReadFull(reader, preface)
	if err != nil {
		return fmt.Errorf("read client preface: %w", err)
	}
	if string(preface) != http2.ClientPreface {
		return fmt.Errorf("client preface = %q", preface)
	}

	framer := http2.NewFramer(conn, reader)
	framer.ReadMetaHeaders = hpack.NewDecoder(64<<10, nil)
	err = framer.WriteSettings()
	if err != nil {
		return fmt.Errorf("write server settings: %w", err)
	}

	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			return fmt.Errorf("read HTTP/2 frame: %w", err)
		}
		switch frame := frame.(type) {
		case *http2.SettingsFrame:
			if !frame.IsAck() {
				for index := 0; index < frame.NumSettings(); index++ {
					setting := frame.Setting(index)
					observation.settings = append(observation.settings, goldenSetting{ID: setting.ID.String(), Value: setting.Val})
				}
				err = framer.WriteSettingsAck()
				if err != nil {
					return fmt.Errorf("write settings ACK: %w", err)
				}
			}
		case *http2.WindowUpdateFrame:
			if frame.StreamID == 0 {
				observation.connectionWindow = frame.Increment
			}
		case *http2.PriorityFrame:
			observation.prioritySeen = true
		case *http2.MetaHeadersFrame:
			for _, field := range frame.Fields {
				if strings.HasPrefix(field.Name, ":") {
					observation.pseudoHeaderOrder = append(observation.pseudoHeaderOrder, field.Name)
				} else {
					observation.applicationHeaderOrder = append(observation.applicationHeaderOrder, field.Name)
				}
			}

			return writeHTTP2Response(framer, frame.StreamID)
		}
	}
}

func writeHTTP2Response(framer *http2.Framer, streamID uint32) error {
	var block bytes.Buffer
	encoder := hpack.NewEncoder(&block)
	for _, field := range []hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: headerContentLength, Value: "2"},
	} {
		err := encoder.WriteField(field)
		if err != nil {
			return err
		}
	}
	err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block.Bytes(),
		EndHeaders:    true,
	})
	if err != nil {
		return fmt.Errorf("write response headers: %w", err)
	}
	err = framer.WriteData(streamID, true, []byte("ok"))
	if err != nil {
		return fmt.Errorf("write response body: %w", err)
	}

	return nil
}

func observeHTTP1(conn net.Conn) error {
	request, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return fmt.Errorf("read HTTP/1.1 request: %w", err)
	}
	_ = request.Body.Close()
	_, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")

	return err
}

type recordingConn struct {
	net.Conn
	mu   sync.Mutex
	read bytes.Buffer
}

func (c *recordingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.mu.Lock()
		_, _ = c.read.Write(p[:n])
		c.mu.Unlock()
	}

	return n, err
}

func (c *recordingConn) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.read.Bytes())
}

func cloneClientHelloInfo(info *tls.ClientHelloInfo) *tls.ClientHelloInfo {
	if info == nil {
		return nil
	}
	clone := *info
	clone.CipherSuites = slices.Clone(info.CipherSuites)
	clone.SupportedCurves = slices.Clone(info.SupportedCurves)
	clone.SupportedPoints = slices.Clone(info.SupportedPoints)
	clone.SignatureSchemes = slices.Clone(info.SignatureSchemes)
	clone.SupportedProtos = slices.Clone(info.SupportedProtos)
	clone.SupportedVersions = slices.Clone(info.SupportedVersions)
	clone.Extensions = slices.Clone(info.Extensions)

	return &clone
}

func parseClientHello(records []byte) (observedClientHello, error) {
	body, err := clientHelloBody(records)
	if err != nil {
		return observedClientHello{}, err
	}
	reader := byteReader{data: body}
	_, err = reader.u16()
	if err != nil {
		return observedClientHello{}, err
	}
	err = reader.skip(32)
	if err != nil {
		return observedClientHello{}, err
	}
	sessionLength, err := reader.u8()
	if err != nil {
		return observedClientHello{}, err
	}
	err = reader.skip(int(sessionLength))
	if err != nil {
		return observedClientHello{}, err
	}
	cipherLength, err := reader.u16()
	if err != nil || cipherLength%2 != 0 {
		return observedClientHello{}, errors.New("invalid ClientHello cipher suites")
	}
	observed := observedClientHello{}
	for range cipherLength / 2 {
		cipher, err := reader.u16()
		if err != nil {
			return observedClientHello{}, err
		}
		observed.CipherSuites = append(observed.CipherSuites, normalizeCipherSuite(cipher))
	}
	compressionLength, err := reader.u8()
	if err != nil {
		return observedClientHello{}, err
	}
	err = reader.skip(int(compressionLength))
	if err != nil {
		return observedClientHello{}, err
	}
	extensionsLength, err := reader.u16()
	if err != nil {
		return observedClientHello{}, err
	}
	extensions, err := reader.bytes(int(extensionsLength))
	if err != nil {
		return observedClientHello{}, err
	}
	extensionReader := byteReader{data: extensions}
	for extensionReader.remaining() > 0 {
		id, err := extensionReader.u16()
		if err != nil {
			return observedClientHello{}, err
		}
		length, err := extensionReader.u16()
		if err != nil {
			return observedClientHello{}, err
		}
		value, err := extensionReader.bytes(int(length))
		if err != nil {
			return observedClientHello{}, err
		}
		observed.Extensions = append(observed.Extensions, normalizeTLSExtension(id))
		switch id {
		case 10:
			observed.SupportedGroups, err = parseU16List(value, normalizeCurve)
		case 13:
			observed.SignatureAlgorithms, err = parseU16List(value, normalizeSignature)
		case 16:
			observed.ALPN, err = parseALPN(value)
		case 43:
			observed.SupportedVersions, err = parseSupportedVersions(value)
		case 51:
			observed.KeyShares, err = parseKeyShares(value)
		}
		if err != nil {
			return observedClientHello{}, fmt.Errorf("parse TLS extension %d: %w", id, err)
		}
	}

	return observed, nil
}

func clientHelloBody(records []byte) ([]byte, error) {
	for offset := 0; offset+5 <= len(records); {
		if records[offset] != 22 {
			length := int(binary.BigEndian.Uint16(records[offset+3 : offset+5]))
			offset += 5 + length

			continue
		}
		length := int(binary.BigEndian.Uint16(records[offset+3 : offset+5]))
		if offset+5+length > len(records) {
			return nil, errors.New("truncated TLS handshake record")
		}
		handshake := records[offset+5 : offset+5+length]
		for inner := 0; inner+4 <= len(handshake); {
			typ := handshake[inner]
			messageLength := int(handshake[inner+1])<<16 | int(handshake[inner+2])<<8 | int(handshake[inner+3])
			if inner+4+messageLength > len(handshake) {
				break
			}
			if typ == 1 {
				return handshake[inner+4 : inner+4+messageLength], nil
			}
			inner += 4 + messageLength
		}
		offset += 5 + length
	}

	return nil, errors.New("ClientHello not found")
}

type byteReader struct {
	data []byte
	off  int
}

func (r *byteReader) remaining() int { return len(r.data) - r.off }

func (r *byteReader) u8() (uint8, error) {
	if r.remaining() < 1 {
		return 0, io.ErrUnexpectedEOF
	}
	value := r.data[r.off]
	r.off++

	return value, nil
}

func (r *byteReader) u16() (uint16, error) {
	if r.remaining() < 2 {
		return 0, io.ErrUnexpectedEOF
	}
	value := binary.BigEndian.Uint16(r.data[r.off : r.off+2])
	r.off += 2

	return value, nil
}

func (r *byteReader) bytes(length int) ([]byte, error) {
	if length < 0 || r.remaining() < length {
		return nil, io.ErrUnexpectedEOF
	}
	value := r.data[r.off : r.off+length]
	r.off += length

	return value, nil
}

func (r *byteReader) skip(length int) error {
	_, err := r.bytes(length)

	return err
}

func parseU16List(value []byte, normalize func(uint16) string) ([]string, error) {
	reader := byteReader{data: value}
	length, err := reader.u16()
	if err != nil || length%2 != 0 {
		return nil, errors.New("invalid uint16 list")
	}
	items := make([]string, 0, length/2)
	for range length / 2 {
		item, err := reader.u16()
		if err != nil {
			return nil, err
		}
		items = append(items, normalize(item))
	}

	return items, nil
}

func parseSupportedVersions(value []byte) ([]string, error) {
	reader := byteReader{data: value}
	length, err := reader.u8()
	if err != nil || length%2 != 0 {
		return nil, errors.New("invalid supported versions")
	}
	items := make([]string, 0, length/2)
	for range length / 2 {
		version, err := reader.u16()
		if err != nil {
			return nil, err
		}
		items = append(items, normalizeTLSVersion(version))
	}

	return items, nil
}

func parseALPN(value []byte) ([]string, error) {
	reader := byteReader{data: value}
	length, err := reader.u16()
	if err != nil {
		return nil, err
	}
	protocols, err := reader.bytes(int(length))
	if err != nil {
		return nil, err
	}
	items := []string{}
	reader = byteReader{data: protocols}
	for reader.remaining() > 0 {
		length, err := reader.u8()
		if err != nil {
			return nil, err
		}
		protocol, err := reader.bytes(int(length))
		if err != nil {
			return nil, err
		}
		items = append(items, string(protocol))
	}

	return items, nil
}

func parseKeyShares(value []byte) ([]goldenKeyShare, error) {
	reader := byteReader{data: value}
	length, err := reader.u16()
	if err != nil {
		return nil, err
	}
	shares, err := reader.bytes(int(length))
	if err != nil {
		return nil, err
	}
	reader = byteReader{data: shares}
	items := []goldenKeyShare{}
	for reader.remaining() > 0 {
		group, err := reader.u16()
		if err != nil {
			return nil, err
		}
		length, err := reader.u16()
		if err != nil {
			return nil, err
		}
		_, err = reader.bytes(int(length))
		if err != nil {
			return nil, err
		}
		items = append(items, goldenKeyShare{Group: normalizeCurve(group), KeyExchangeLength: int(length)})
	}

	return items, nil
}

func normalizeCipherSuite(id uint16) string {
	if isGREASE(id) {
		return profileGoldenGREASE
	}

	return tls.CipherSuiteName(id)
}

func normalizeTLSExtension(id uint16) string {
	if isGREASE(id) {
		return profileGoldenGREASE
	}
	switch id {
	case 0:
		return "server_name"
	case 5:
		return "status_request"
	case 10:
		return "supported_groups"
	case 11:
		return "ec_point_formats"
	case 13:
		return "signature_algorithms"
	case 16:
		return "application_layer_protocol_negotiation"
	case 18:
		return "signed_certificate_timestamp"
	case 21:
		return "padding"
	case 23:
		return "extended_master_secret"
	case 27:
		return "compress_certificate"
	case 35:
		return "session_ticket"
	case 41:
		return "pre_shared_key"
	case 43:
		return "supported_versions"
	case 45:
		return "psk_key_exchange_modes"
	case 51:
		return "key_share"
	case 17513, 17613:
		return "application_settings"
	case 65281:
		return "renegotiation_info"
	case 0xfe0d:
		return "encrypted_client_hello"
	default:
		return fmt.Sprintf("extension_%d", id)
	}
}

func normalizeTLSVersion(version uint16) string {
	if isGREASE(version) {
		return profileGoldenGREASE
	}
	switch version {
	case tls.VersionTLS13:
		return "TLS1.3"
	case tls.VersionTLS12:
		return "TLS1.2"
	default:
		return fmt.Sprintf("TLS_%04x", version)
	}
}

func normalizeCurve(curve uint16) string {
	if isGREASE(curve) {
		return profileGoldenGREASE
	}
	switch tls.CurveID(curve) {
	case tls.X25519:
		return "X25519"
	case tls.CurveP256:
		return "P-256"
	case tls.CurveP384:
		return "P-384"
	case tls.CurveP521, tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024:
		return fmt.Sprintf("curve_%d", curve)
	default:
		return fmt.Sprintf("curve_%d", curve)
	}
}

func normalizeSignature(signature uint16) string {
	switch tls.SignatureScheme(signature) {
	case tls.ECDSAWithP256AndSHA256:
		return "ECDSA_SECP256R1_SHA256"
	case tls.PSSWithSHA256:
		return "RSA_PSS_RSAE_SHA256"
	case tls.PKCS1WithSHA256:
		return "RSA_PKCS1_SHA256"
	case tls.ECDSAWithP384AndSHA384:
		return "ECDSA_SECP384R1_SHA384"
	case tls.PSSWithSHA384:
		return "RSA_PSS_RSAE_SHA384"
	case tls.PKCS1WithSHA384:
		return "RSA_PKCS1_SHA384"
	case tls.PSSWithSHA512:
		return "RSA_PSS_RSAE_SHA512"
	case tls.PKCS1WithSHA512:
		return "RSA_PKCS1_SHA512"
	case tls.ECDSAWithP521AndSHA512, tls.Ed25519, tls.PKCS1WithSHA1, tls.ECDSAWithSHA1:
		return fmt.Sprintf("signature_%d", signature)
	default:
		return fmt.Sprintf("signature_%d", signature)
	}
}

func isGREASE(value uint16) bool {
	return value&0x0f0f == 0x0a0a && value>>8 == value&0xff
}

func sortedStrings(values []string) []string {
	clone := slices.Clone(values)
	sort.Strings(clone)

	return clone
}
