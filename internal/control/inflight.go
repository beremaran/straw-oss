package control

import (
	"context"
	"sync"
)

// InFlightRegistry owns cancellation functions for requests running in this
// Control process. Straw intentionally supports a single Control instance.
type InFlightRegistry struct {
	mu      sync.Mutex
	entries map[string]inFlightEntry
}

type inFlightEntry struct {
	deploymentID string
	cancel       func()
}

// InFlightRequest is the safe administrative view of an active request.
type InFlightRequest struct {
	RequestID    string `json:"request_id"`
	DeploymentID string `json:"deployment_id"`
}

// NewInFlightRegistry creates an empty in-process request registry.
func NewInFlightRegistry() *InFlightRegistry {
	return &InFlightRegistry{entries: make(map[string]inFlightEntry)}
}

// Register records one running request.
func (r *InFlightRegistry) Register(_ context.Context, requestID, deploymentID string, cancel func()) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.entries[requestID] = inFlightEntry{deploymentID: deploymentID, cancel: cancel}
	r.mu.Unlock()
}

// Cancel safely propagates cancellation through the existing request stream.
func (r *InFlightRegistry) Cancel(requestID string) bool {
	if r == nil {
		return false
	}

	r.mu.Lock()
	entry, ok := r.entries[requestID]
	r.mu.Unlock()

	if ok && entry.cancel != nil {
		entry.cancel()
	}

	return ok
}

// Requests lists active request identifiers without exposing request contents.
func (r *InFlightRegistry) Requests() []InFlightRequest {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]InFlightRequest, 0, len(r.entries))
	for id, entry := range r.entries {
		out = append(out, InFlightRequest{RequestID: id, DeploymentID: entry.deploymentID})
	}

	return out
}

// Deregister removes a completed request.
func (r *InFlightRegistry) Deregister(_ context.Context, requestID string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	delete(r.entries, requestID)
	r.mu.Unlock()
}
