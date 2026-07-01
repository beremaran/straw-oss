package endpoint

import (
	"context"
	"testing"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/protocol"
)

func TestNewWorkerAppliesCustomExecutor(t *testing.T) {
	executor := &dummyExecutor{}
	cfg := &config.EndpointConfig{ID: "endpoint-1"}

	w := NewWorker(cfg, WithRequestExecutor(executor))

	if w.executor != executor {
		t.Fatal("custom executor was not applied")
	}
}

type dummyExecutor struct{}

func (d *dummyExecutor) Do(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
	return &protocol.Response{
		RequestID:  req.ID,
		StatusCode: 200,
	}, nil
}
