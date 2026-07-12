package main

import (
	"context"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
)

const (
	responseStartSeq = 1
	responseDataSeq  = 2
	responseEndSeq   = 3
)

// staticExecutor answers every decoded-HTTP assignment with the same fixed
// status and body. It never resolves a hostname or opens an upstream
// connection, so it has no destination-policy obligation of its own; see
// README.md for what a delegating (network-reaching) custom executor must
// enforce instead.
type staticExecutor struct {
	status uint32
	body   []byte
}

// Execute implements sdkegress.Executor.
func (e *staticExecutor) Execute(_ context.Context, _ *strawpb.RequestStart, _ []byte, attempt uint32, _ func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	return []*strawpb.StreamFrame{
		{
			StreamSeq: responseStartSeq,
			Attempt:   attempt,
			Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{
				Status:  e.status,
				Headers: []*strawpb.Header{{Name: "Content-Type", Value: []byte("text/plain; charset=utf-8")}},
			}},
		},
		{
			StreamSeq: responseDataSeq,
			Attempt:   attempt,
			Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 0, Data: e.body}},
		},
		{
			StreamSeq: responseEndSeq,
			Attempt:   attempt,
			Payload:   &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}},
		},
	}
}
