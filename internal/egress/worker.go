package egress

import (
	"fmt"

	strawpb "github.com/beremaran/straw-oss/v2/api/proto/straw/v1"
	"github.com/beremaran/straw-oss/v2/internal/natsx"
	sdkegress "github.com/beremaran/straw-oss/v2/sdk/egress"
)

// Worker is the SDK-owned assignment runtime used by the official worker.
type Worker = sdkegress.Worker

// NewWorker builds the official worker assignment runtime on sdk/egress.
func NewWorker(conn *natsx.Connection, id Identity, executor *Executor, sessionID string, maxConcurrency uint32) (*Worker, error) {
	worker, err := sdkegress.NewWorker(sdkegress.WorkerOptions{
		Conn:           conn,
		Identity:       sdkegress.Identity(id),
		Executor:       executor,
		BodyRefs:       bodyRefAdapter{executor: executor},
		Tunnels:        tunnelAdapter{executor: executor},
		SessionID:      sessionID,
		MaxConcurrency: maxConcurrency,
		SupportedModes: []strawpb.RequestMode{strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP, strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL},
	})
	if err != nil {
		return nil, fmt.Errorf("sdk new worker: %w", err)
	}

	return worker, nil
}
