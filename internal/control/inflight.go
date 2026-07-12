package control

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const leaseRefreshDivisor = 3

// InFlightRegistry owns local cancellation functions and optionally publishes
// cross-instance cancellations using shared ownership records.
type InFlightRegistry struct {
	mu         sync.Mutex
	entries    map[string]inFlightEntry
	shared     RuntimeState
	instanceID string
	ttl        time.Duration
	publisher  snapshotPublisher
}

type inFlightEntry struct {
	deploymentID string
	cancel       func()
}

// NewInFlightRegistry creates an empty in-process request registry.
func NewInFlightRegistry() *InFlightRegistry {
	return &InFlightRegistry{entries: make(map[string]inFlightEntry)}
}

// NewSharedInFlightRegistry coordinates ownership and remote cancellation.
func NewSharedInFlightRegistry(state RuntimeState, instanceID string, ttl time.Duration, publisher snapshotPublisher) *InFlightRegistry {
	return &InFlightRegistry{entries: make(map[string]inFlightEntry), shared: state, instanceID: instanceID, ttl: ttl, publisher: publisher}
}

// SetupRemoteCancellation subscribes this instance before it becomes ready.
func (r *InFlightRegistry) SetupRemoteCancellation(conn rolloutSubscriber) error {
	if r == nil || r.shared == nil {
		return nil
	}

	_, err := conn.Subscribe(remoteCancelSubject(r.instanceID), func(msg *nats.Msg) { cancelLocal(r, string(msg.Data)) })
	if err != nil {
		return fmt.Errorf("subscribe remote cancellation: %w", err)
	}

	err = conn.Flush()
	if err != nil {
		return fmt.Errorf("flush remote cancellation subscription: %w", err)
	}

	return nil
}

func remoteCancelSubject(instanceID string) string {
	return "straw.v1.control." + instanceID + ".cancel"
}

// InFlightRequest is the safe administrative view of an active request.
type InFlightRequest struct {
	RequestID    string `json:"request_id"`
	DeploymentID string `json:"deployment_id"`
}

// Register records one running request.
func (r *InFlightRegistry) Register(ctx context.Context, requestID, deploymentID string, cancel func()) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.entries[requestID] = inFlightEntry{deploymentID: deploymentID, cancel: cancel}
	r.mu.Unlock()

	if r.shared != nil {
		ok, err := r.shared.claimRequest(ctx, requestID, deploymentID, r.instanceID, r.ttl)
		if err != nil || !ok {
			cancelLocal(r, requestID)

			return
		}

		go renewInFlight(ctx, r, requestID)
	}
}

// Cancel safely propagates cancellation through the existing request stream.
func (r *InFlightRegistry) Cancel(ctx context.Context, requestID string) bool {
	if r == nil {
		return false
	}

	if cancelLocal(r, requestID) {
		return true
	}

	if r.shared == nil {
		return false
	}

	owner, ok, err := r.shared.requestOwner(ctx, requestID)
	if err != nil || !ok || owner == "" || r.publisher == nil {
		return false
	}

	return r.publisher.Publish(remoteCancelSubject(owner), []byte(requestID)) == nil
}

// Requests lists active request identifiers without exposing request contents.
func (r *InFlightRegistry) Requests(ctx context.Context) []InFlightRequest {
	if r == nil {
		return nil
	}

	if r.shared != nil {
		out, err := r.shared.requests(ctx)
		if err == nil {
			return out
		}

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
func (r *InFlightRegistry) Deregister(ctx context.Context, requestID string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	delete(r.entries, requestID)
	r.mu.Unlock()

	if r.shared != nil {
		_ = r.shared.releaseRequest(ctx, requestID, r.instanceID)
	}
}

func renewInFlight(ctx context.Context, registry *InFlightRegistry, requestID string) {
	interval := registry.ttl / leaseRefreshDivisor
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok, err := registry.shared.renewRequest(ctx, requestID, registry.instanceID, registry.ttl)
			if err != nil || !ok {
				cancelLocal(registry, requestID)

				return
			}
		}
	}
}

func cancelLocal(registry *InFlightRegistry, requestID string) bool {
	registry.mu.Lock()
	entry, ok := registry.entries[requestID]
	registry.mu.Unlock()

	if ok && entry.cancel != nil {
		entry.cancel()
	}

	return ok
}
