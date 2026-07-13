package egress

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

const profiledLateTrailerValue = "done"

func TestProfiledSendControlAppliesPeerSettingsAndAcknowledges(t *testing.T) {
	t.Parallel()

	var encoded bytes.Buffer

	writer := http2.NewFramer(&encoded, nil)
	err := writer.WriteSettings(
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: 1 << 20},
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: 32 << 10},
	)
	if err != nil {
		t.Fatalf("write fixture settings: %v", err)
	}

	var acknowledgements bytes.Buffer

	framer := http2.NewFramer(&acknowledgements, bytes.NewReader(encoded.Bytes()))
	flow := profiledSendFlow{
		connection:    profiledDefaultFlowWindow,
		stream:        profiledDefaultFlowWindow,
		initialStream: profiledDefaultFlowWindow,
		maxFrame:      profiledHTTP2MaxFrameSize,
	}

	early, err := readProfiledSendControl(framer, profiledDefaultHTTP2StreamID, &flow)
	if err != nil {
		t.Fatalf("read control settings: %v", err)
	}
	if early != nil {
		t.Fatal("settings produced an early response")
	}
	if !flow.seenSettings || flow.stream != 1<<20 || flow.initialStream != 1<<20 || flow.maxFrame != 32<<10 {
		t.Fatalf("flow = %+v, want peer settings applied", flow)
	}

	ackReader := http2.NewFramer(nil, bytes.NewReader(acknowledgements.Bytes()))
	frame, err := ackReader.ReadFrame()
	if err != nil {
		t.Fatalf("read settings acknowledgement: %v", err)
	}

	settings, ok := frame.(*http2.SettingsFrame)
	if !ok || !settings.IsAck() {
		t.Fatalf("acknowledgement = %#v, want SETTINGS ACK", frame)
	}
}

func TestProfiledSendControlHandlesInformationalFinalAndResetFrames(t *testing.T) {
	t.Parallel()

	informational := profiledMetaHeaders(http.StatusEarlyHints, profiledDefaultHTTP2StreamID, false)
	early, err := finalProfiledResponseHeaders(profiledDefaultHTTP2StreamID, informational)
	if err != nil || early != nil {
		t.Fatalf("informational response = (%#v, %v), want ignored", early, err)
	}

	final := profiledMetaHeaders(http.StatusTooManyRequests, profiledDefaultHTTP2StreamID, false)
	early, err = finalProfiledResponseHeaders(profiledDefaultHTTP2StreamID, final)
	if err != nil || early != final {
		t.Fatalf("final response = (%#v, %v), want returned", early, err)
	}

	invalid := profiledMetaHeaders(0, profiledDefaultHTTP2StreamID, false)
	_, err = finalProfiledResponseHeaders(profiledDefaultHTTP2StreamID, invalid)
	if !errors.Is(err, errProfiledHTTP2InvalidStatus) {
		t.Fatalf("invalid status error = %v", err)
	}

	reset := &http2.RSTStreamFrame{
		FrameHeader: http2.FrameHeader{StreamID: profiledDefaultHTTP2StreamID},
		ErrCode:     http2.ErrCodeRefusedStream,
	}
	err = profiledResetError(profiledDefaultHTTP2StreamID, reset)

	var streamError http2.StreamError
	if !errors.As(err, &streamError) || streamError.Code != http2.ErrCodeRefusedStream {
		t.Fatalf("reset error = %#v, want refused-stream error", err)
	}
}

func TestProfiledHTTP2BodyAcceptsLateTrailers(t *testing.T) {
	t.Parallel()

	trailers := make(http.Header)
	body := &profiledHTTP2Body{streamID: profiledDefaultHTTP2StreamID, trailers: trailers}
	frame := &http2.MetaHeadersFrame{
		HeadersFrame: &http2.HeadersFrame{FrameHeader: http2.FrameHeader{
			StreamID: profiledDefaultHTTP2StreamID,
			Flags:    http2.FlagHeadersEndStream,
		}},
		Fields: []hpack.HeaderField{{Name: "x-late", Value: profiledLateTrailerValue}},
	}

	body.acceptTrailers(frame)

	if !body.done || trailers.Get("X-Late") != profiledLateTrailerValue {
		t.Fatalf("body = %#v, trailers = %#v", body, trailers)
	}
}

func profiledMetaHeaders(status int, streamID uint32, end bool) *http2.MetaHeadersFrame {
	flags := http2.Flags(0)
	if end {
		flags = http2.FlagHeadersEndStream
	}

	value := "invalid"
	if status != 0 {
		value = strconv.Itoa(status)
	}

	return &http2.MetaHeadersFrame{
		HeadersFrame: &http2.HeadersFrame{FrameHeader: http2.FrameHeader{StreamID: streamID, Flags: flags}},
		Fields:       []hpack.HeaderField{{Name: ":status", Value: value}},
	}
}
