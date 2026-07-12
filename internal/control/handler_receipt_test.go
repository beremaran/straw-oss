package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
)

type receiptPreparerStub struct {
	finished bool
	consumed bool
}

func (s *receiptPreparerStub) PrepareRequest(context.Context, string, string, string) (*strawpb.BodyRefFrame, error) {
	return &strawpb.BodyRefFrame{ExpectedSizeBytes: 9, Sha256Hex: strings.Repeat("a", 64)}, nil
}

func (s *receiptPreparerStub) FinishRequest(_ context.Context, _, _, _ string, consumed bool) error {
	s.finished = true
	s.consumed = consumed

	return nil
}

type receiptDispatcherStub struct{ sawRef bool }

func (s *receiptDispatcherStub) Dispatch(_ context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	s.sawRef = in.Request.BodyRef != nil && in.Request.BodySizeBytes == 9

	return SuccessResponse{RequestID: in.RequestID, Status: 200, Body: ResponseBody{Mode: "inline_base64"}}, nil
}

func TestRequestHandlerPreparesAndConsumesReceipt(t *testing.T) {
	t.Parallel()
	preparer := &receiptPreparerStub{}
	dispatcher := &receiptDispatcherStub{}
	handler := NewRequestHandler(4, 5000, NewDeploymentAuthenticator(""))
	handler.SetDispatcher(dispatcher)
	handler.SetReceiptPreparer(preparer)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(`{"method":"POST","url":"https://example.com","body":{"mode":"receipt","receipt_id":"rcpt_1"}}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !dispatcher.sawRef || !preparer.finished || !preparer.consumed {
		t.Fatalf("status=%d saw_ref=%v finished=%v consumed=%v body=%s", rec.Code, dispatcher.sawRef, preparer.finished, preparer.consumed, rec.Body.String())
	}
}
