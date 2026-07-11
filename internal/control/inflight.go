package control

import (
	"context"
	"sync"
)

// InFlightRegistry owns cancellation functions for requests running in this
// Control process. Straw intentionally supports a single Control instance.
type InFlightRegistry struct {
	mu      sync.Mutex
	entries map[string]func()
}

// NewInFlightRegistry creates an empty in-process request registry.
func NewInFlightRegistry() *InFlightRegistry {
	return &InFlightRegistry{entries: make(map[string]func())}
}

// Register records one running request.
func (r *InFlightRegistry) Register(_ context.Context, requestID, _ string, cancel func()) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.entries[requestID] = cancel
	r.mu.Unlock()
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
