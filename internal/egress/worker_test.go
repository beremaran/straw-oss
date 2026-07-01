package egress

import (
	"context"
	"testing"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/protocol/wirepb"
)

func TestNewWorkerAppliesCustomExecutor(t *testing.T) {
	executor := &dummyExecutor{}
	cfg := &config.EgressConfig{ID: "egress-1"}

	w := NewWorker(cfg, executor)

	if w.executor != executor {
		t.Fatal("custom executor was not applied")
	}
}

type dummyExecutor struct{}

func (d *dummyExecutor) Do(_ context.Context, req *wirepb.Request) (*wirepb.Response, error) {
	return &wirepb.Response{
		RequestId:  req.GetId(),
		StatusCode: 200,
	}, nil
}
