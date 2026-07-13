package control

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

type requestBodyUpload struct {
	gate *tunnelUploadGate
	err  <-chan error
	c2e  *c2eStreamSender
}

type tunnelUploadGate struct {
	mu     sync.Mutex
	credit uint64
	wake   chan struct{}
	closed bool
}

func newTunnelUploadGate(initial uint64) *tunnelUploadGate {
	return &tunnelUploadGate{credit: initial, wake: make(chan struct{}, 1)}
}

func (g *tunnelUploadGate) grant(bytes uint64) {
	if bytes == 0 {
		return
	}

	g.mu.Lock()
	g.credit += bytes
	g.mu.Unlock()

	select {
	case g.wake <- struct{}{}:
	default:
	}
}

func (g *tunnelUploadGate) takeMax(maxBytes uint64) (uint64, bool) {
	for {
		g.mu.Lock()
		if g.closed {
			g.mu.Unlock()

			return 0, false
		}

		if g.credit > 0 {
			bytes := min(g.credit, maxBytes)
			g.credit -= bytes
			g.mu.Unlock()

			return bytes, true
		}
		g.mu.Unlock()

		<-g.wake
	}
}

func (g *tunnelUploadGate) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()

	select {
	case g.wake <- struct{}{}:
	default:
	}
}

type c2eStreamSender struct {
	mu         sync.Mutex
	dispatcher *DefaultRequestDispatcher
	subject    string
	in         DispatchInput
	deadline   time.Time
	seq        uint64
}

func (s *c2eStreamSender) data(offset uint64, data []byte) {
	s.publish(func(seq uint64) *strawpb.StreamFrame {
		return &strawpb.StreamFrame{
			StreamSeq: seq,
			Attempt:   defaultRequestAttempt,
			Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: offset, Data: data}},
		}
	})
}

func (s *c2eStreamSender) credit(bytes uint64) {
	if bytes == 0 {
		return
	}

	s.publish(func(seq uint64) *strawpb.StreamFrame {
		return &strawpb.StreamFrame{
			StreamSeq: seq,
			Attempt:   defaultRequestAttempt,
			Payload:   &strawpb.StreamFrame_Credit{Credit: &strawpb.CreditFrame{DownloadCreditBytes: bytes}},
		}
	})
}

func (s *c2eStreamSender) cancel(reason string) {
	s.publish(func(seq uint64) *strawpb.StreamFrame {
		return &strawpb.StreamFrame{
			StreamSeq: seq,
			Attempt:   defaultRequestAttempt,
			Payload:   &strawpb.StreamFrame_Cancel{Cancel: &strawpb.CancelFrame{Reason: reason}},
		}
	})
}

func (s *c2eStreamSender) publish(build func(uint64) *strawpb.StreamFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()

	frame := build(s.seq)
	_ = s.dispatcher.publishFrame(s.subject, s.in, s.deadline, frame)
	s.seq++
}

func (d *DefaultRequestDispatcher) startStreamingRequestBody(ctx context.Context, subject string, in DispatchInput, deadline time.Time, seq uint64) *requestBodyUpload {
	if in.Request == nil || in.Request.BodyReader == nil {
		return nil
	}

	gate := newTunnelUploadGate(d.opts.InitialUploadCreditBytes)
	errs := make(chan error, 1)
	upload := &requestBodyUpload{
		gate: gate,
		err:  errs,
		c2e:  &c2eStreamSender{dispatcher: d, subject: subject, in: in, deadline: deadline, seq: seq},
	}

	go func() {
		defer gate.close()
		defer func() { _ = in.Request.BodyReader.Close() }()

		errs <- d.streamRequestBody(ctx, in, upload)
	}()

	return upload
}

func (d *DefaultRequestDispatcher) streamRequestBody(ctx context.Context, in DispatchInput, upload *requestBodyUpload) error {
	buf := make([]byte, d.opts.MaxFrameDataBytes)
	offset := uint64(0)

	for {
		if in.Request.BodySizeBytes >= 0 && offset >= uint64(in.Request.BodySizeBytes) {
			return nil
		}

		credit, ok := upload.gate.takeMax(uint64(len(buf)))
		if !ok {
			return context.Canceled
		}

		n, err := in.Request.BodyReader.Read(buf[:credit])
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			upload.c2e.data(offset, chunk)
			offset += uint64FromInt(n)
		}

		if uint64FromInt(n) < credit {
			upload.gate.grant(credit - uint64FromInt(n))
		}

		perr := streamingRequestBodyReadError(err)
		if perr != nil {
			return perr
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("stream request body context: %w", ctx.Err())
		default:
		}
	}
}

func uploadErrorChan(upload *requestBodyUpload) <-chan error {
	if upload == nil {
		return nil
	}

	return upload.err
}

func closeUploadGate(upload *requestBodyUpload) {
	if upload == nil {
		return
	}

	upload.gate.close()
}

func streamingRequestBodyReadError(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}

	return &requestStreamError{perr: &PipelineError{Code: BodyTooLarge}, err: err}
}

func (d *DefaultRequestDispatcher) sendRequestBody(subject string, in DispatchInput, deadline time.Time, seq uint64) (uint64, error) {
	body := in.Request.BodyData
	if len(body) == 0 {
		return seq, nil
	}

	return d.sendRequestDataFrames(subject, in, deadline, seq, body)
}

func (d *DefaultRequestDispatcher) sendRequestDataFrames(subject string, in DispatchInput, deadline time.Time, seq uint64, body []byte) (uint64, error) {
	offset := uint64(0)

	for len(body) > 0 {
		seq++
		n := frameChunkSize(len(body), d.opts.MaxFrameDataBytes)
		chunk := body[:n]
		body = body[n:]

		err := d.publishFrame(subject, in, deadline, &strawpb.StreamFrame{
			StreamSeq: seq,
			Attempt:   defaultRequestAttempt,
			Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: offset, Data: chunk}},
		})
		if err != nil {
			return 0, err
		}

		offset += uint64(len(chunk))
	}

	return seq, nil
}

type requestStreamError struct {
	perr *PipelineError
	err  error
}

func requestStreamPipelineError(err error) *PipelineError {
	var streamErr *requestStreamError
	if errors.As(err, &streamErr) && streamErr.perr != nil {
		return streamErr.perr
	}

	return &PipelineError{Code: TransportUnavailable}
}

func requestStreamPipelineErrorOrNil(err error) *PipelineError {
	if err == nil {
		return nil
	}

	return requestStreamPipelineError(err)
}

func (e *requestStreamError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}

	return "request stream error"
}

func (d *DefaultRequestDispatcher) publishFrame(subject string, in DispatchInput, deadline time.Time, frame *strawpb.StreamFrame) error {
	env := &strawpb.Envelope{
		RequestId:      in.RequestID,
		DeploymentId:   in.Identity.DeploymentID,
		DeadlineUnixMs: deadline.UnixMilli(),
		ProtocolMajor:  ProtocolMajor,
		Attempt:        defaultRequestAttempt,
		Payload:        &strawpb.Envelope_StreamFrame{StreamFrame: frame},
	}

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		return fmt.Errorf("marshal stream frame: %w", err)
	}

	err = d.opts.NATS.Publish(subject, raw)
	if err != nil {
		return fmt.Errorf("publish stream frame: %w", err)
	}

	return nil
}
