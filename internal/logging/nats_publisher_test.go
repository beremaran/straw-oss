package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/testutil"
)

func TestNATSLogPublisherPublishesRedactedProtobuf(t *testing.T) {
	conn := &recordingNATSConn{published: make(chan []byte, 1)}
	publisher := NewNATSLogPublisher(conn, natsx.LogTelemetrySubject(), 1)
	defer publisher.Close()

	var stdout bytes.Buffer
	logger := slog.New(NewTeeHandler(NewHandler(&stdout), publisher)).With("service", "egress", "worker_id", loggingTestWorkerID)
	logger.Info("assignment accepted", "request_id", loggingTestRequest, "tenant_id", loggingTestTenant, "api_key_secret", "secret-value")

	raw := <-conn.published
	env, err := natsx.UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope() error = %v", err)
	}

	event := env.GetLogEvent()
	if event == nil {
		t.Fatal("log_event payload missing")
	}

	if conn.subject != natsx.LogTelemetrySubject() {
		t.Fatalf("subject = %q, want %q", conn.subject, natsx.LogTelemetrySubject())
	}
	if event.GetService() != "egress" || event.GetWorkerId() != loggingTestWorkerID || event.GetRequestId() != loggingTestRequest || event.GetTenantId() != loggingTestTenant {
		t.Fatalf("event context = %+v", event)
	}
	if event.GetExtra()["api_key_secret"] != redactedValue {
		t.Fatalf("api_key_secret = %q, want redacted", event.GetExtra()["api_key_secret"])
	}
}

func TestNATSLogPublisherOverflowDoesNotBlock(t *testing.T) {
	conn := &blockingNATSConn{release: make(chan struct{})}
	publisher := NewNATSLogPublisher(conn, natsx.LogTelemetrySubject(), 1)

	publisher.Enqueue(LogEvent{Message: "first"})
	publisher.Enqueue(LogEvent{Message: "second"})

	start := time.Now()
	publisher.Enqueue(LogEvent{Message: "third"})
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Enqueue blocked for %s", elapsed)
	}
	if publisher.Dropped() == 0 {
		t.Fatal("Dropped() = 0, want overflow to be observable")
	}

	close(conn.release)
	publisher.Close()
}

func TestNATSLogPublisherPublishErrorIsObservableDrop(t *testing.T) {
	publisher := NewNATSLogPublisher(failingNATSConn{}, natsx.LogTelemetrySubject(), 1)
	defer publisher.Close()

	publisher.Enqueue(LogEvent{Message: "outage"})

	deadline := time.Now().Add(time.Second)
	for publisher.Dropped() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for publish error drop")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestNATSLogPublisherNoSubscriberDoesNotBlockOrDrop(t *testing.T) {
	srv := testutil.NewFakeNATSServer(t, 2_000_000)
	conn, err := natsx.Connect(natsx.ConnectOptions{Servers: []string{srv.URL()}})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(conn.Close)

	publisher := NewNATSLogPublisher(conn, natsx.LogTelemetrySubject(), 1)

	start := time.Now()
	publisher.Enqueue(LogEvent{Message: "no subscriber"})
	publisher.Close()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("publish without subscriber blocked for %s", elapsed)
	}
	if publisher.Dropped() != 0 {
		t.Fatalf("Dropped() = %d, want 0", publisher.Dropped())
	}
}

type recordingNATSConn struct {
	subject   string
	published chan []byte
}

func (c *recordingNATSConn) Publish(subject string, data []byte) error {
	c.subject = subject
	c.published <- append([]byte(nil), data...)

	return nil
}

type blockingNATSConn struct {
	release chan struct{}
}

func (c *blockingNATSConn) Publish(string, []byte) error {
	<-c.release

	return nil
}

type failingNATSConn struct{}

func (failingNATSConn) Publish(string, []byte) error {
	return errors.New("nats down")
}
