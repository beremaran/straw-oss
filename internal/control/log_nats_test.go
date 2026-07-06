package control

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/testutil"
)

const (
	logNATSTestPoolID   = "pool-1"
	logNATSTestRequest  = "req-1"
	logNATSTestTenant   = "tenant-1"
	logNATSTestReqEnv   = "req-env"
	logNATSTestTenantEn = "tenant-env"
	logNATSTestTraceEnv = "trace-env"
)

func TestHandleLogEventEnqueuesClickHouseRow(t *testing.T) {
	sink := &recordingLogEventSink{}
	writer := NewLogEventWriter(sink, 10, 10, time.Hour)

	raw, err := natsx.MarshalEnvelope(&strawpb.Envelope{
		RequestId: logNATSTestReqEnv,
		TenantId:  logNATSTestTenantEn,
		TraceId:   logNATSTestTraceEnv,
		Payload: &strawpb.Envelope_LogEvent{LogEvent: &strawpb.LogEvent{
			TimestampUnixMs: time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC).UnixMilli(),
			Service:         errorCategoryEgress,
			Level:           "INFO",
			Message:         "assignment accepted",
			WorkerId:        routingTestWorker1,
			ErrorCode:       errorCodeRouteNoMatch,
			Extra:           map[string]string{"pool_id": logNATSTestPoolID},
		}},
	})
	if err != nil {
		t.Fatalf("MarshalEnvelope() error = %v", err)
	}

	handleLogEvent(writer, &nats.Msg{Data: raw})

	err = writer.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	events := sink.events()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}

	event := events[0]
	if event.Service != errorCategoryEgress || event.RequestID != logNATSTestReqEnv || event.TenantID != logNATSTestTenantEn || event.TraceID != logNATSTestTraceEnv || event.WorkerID != routingTestWorker1 {
		t.Fatalf("event context = %+v", event)
	}
	if event.Extra["pool_id"] != logNATSTestPoolID {
		t.Fatalf("pool_id = %q, want %q", event.Extra["pool_id"], logNATSTestPoolID)
	}
}

func TestLogEventSubscriptionReceivesNATSTelemetry(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := mustConnectNATS(t, srv.URL())
	t.Cleanup(controlConn.Close)

	sink := &recordingLogEventSink{}
	writer := NewLogEventWriter(sink, 10, 10, time.Hour)

	err := SetupLogEventSubscription(controlConn, writer)
	if err != nil {
		t.Fatalf("SetupLogEventSubscription() error = %v", err)
	}

	workerConn := mustConnectNATS(t, srv.URL())
	t.Cleanup(workerConn.Close)

	raw, err := natsx.MarshalEnvelope(&strawpb.Envelope{
		RequestId: logNATSTestRequest,
		TenantId:  logNATSTestTenant,
		Payload: &strawpb.Envelope_LogEvent{LogEvent: &strawpb.LogEvent{
			Service:  errorCategoryEgress,
			Level:    "WARN",
			Message:  "upstream failed",
			WorkerId: routingTestWorker1,
		}},
	})
	if err != nil {
		t.Fatalf("MarshalEnvelope() error = %v", err)
	}

	err = workerConn.Publish(natsx.LogTelemetrySubject(), raw)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		err = writer.Flush(context.Background())
		if err != nil {
			t.Fatalf("Flush() error = %v", err)
		}

		if events := sink.events(); len(events) == 1 {
			if events[0].Message != "upstream failed" || events[0].RequestID != logNATSTestRequest || events[0].TenantID != logNATSTestTenant {
				t.Fatalf("event = %+v", events[0])
			}

			return
		}

		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for log event")
		}

		time.Sleep(10 * time.Millisecond)
	}
}
