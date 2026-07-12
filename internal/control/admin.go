package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/beremaran/straw-oss/internal/config"
)

const (
	// RuntimeSnapshotSubject distributes validated deployment snapshots to workers.
	RuntimeSnapshotSubject = "straw.v1.config.snapshot"
	// RuntimeSnapshotAckSubject receives worker rollout acknowledgements.
	RuntimeSnapshotAckSubject = "straw.v1.config.ack"

	rolloutApplied    = "applied"
	republishInterval = 5 * time.Second
)

var (
	errAdminDependencies     = errors.New("runtime admin store, cache, and worker registry are required")
	errConfigVersionNotFound = errors.New("configuration version not found")
	errWorkerIDRequired      = errors.New("worker id is required")
	errWorkerAction          = errors.New("unsupported worker action")
	errAdminTokenRequired    = errors.New("runtime administration requires a non-empty admin token")
)

type snapshotPublisher interface {
	Publish(subject string, data []byte) error
}

type rolloutSubscriber interface {
	Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error)
	Flush() error
}

// RolloutStatus reports which runtime components have applied the current version.
type RolloutStatus struct {
	ConfigVersion uint64          `json:"config_version"`
	Control       string          `json:"control"`
	Workers       []WorkerRollout `json:"workers"`
}

// WorkerRollout reports one worker's progress toward the current version.
type WorkerRollout struct {
	WorkerID string `json:"worker_id"`
	Status   string `json:"status"`
}

// AdminService owns validated activation, audit, rollback, lifecycle and cancellation.
type AdminService struct {
	store     RuntimeConfigStore
	cache     *ConfigCache
	workers   *WorkerRegistry
	inflight  *InFlightRegistry
	publisher snapshotPublisher
	mu        sync.Mutex
	acked     map[string]uint64
}

// NewAdminService loads durable configuration and activates it before serving requests.
func NewAdminService(store RuntimeConfigStore, cache *ConfigCache, workers *WorkerRegistry, inflight *InFlightRegistry, publisher snapshotPublisher) (*AdminService, error) {
	if store == nil || cache == nil || workers == nil {
		return nil, errAdminDependencies
	}

	service := &AdminService{
		store: store, cache: cache, workers: workers, inflight: inflight,
		publisher: publisher, acked: make(map[string]uint64),
	}

	record, err := store.Current()
	if err != nil {
		return nil, fmt.Errorf("load runtime configuration: %w", err)
	}

	err = config.ValidateSnapshot(record.Snapshot)
	if err != nil {
		return nil, fmt.Errorf("validate stored runtime configuration: %w", err)
	}

	service.activate(record.Snapshot)

	return service, nil
}

// Current returns the active durable configuration record.
func (s *AdminService) Current() (ConfigRecord, error) {
	record, err := s.store.Current()
	if err != nil {
		return ConfigRecord{}, fmt.Errorf("read current runtime configuration: %w", err)
	}

	return record, nil
}

// History returns newest-first retained activation history.
func (s *AdminService) History() ([]ConfigRecord, error) {
	records, err := s.store.History()
	if err != nil {
		return nil, fmt.Errorf("read runtime configuration history: %w", err)
	}

	return records, nil
}

// Update validates, persists with compare-and-swap, and activates a snapshot.
func (s *AdminService) Update(expectedRevision uint64, snapshot config.Snapshot, actor, action string) (ConfigRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.store.Current()
	if err != nil {
		return ConfigRecord{}, fmt.Errorf("read current configuration for update: %w", err)
	}

	if current.Revision != expectedRevision {
		return ConfigRecord{}, ErrConfigConflict
	}

	snapshot.ConfigVersion = current.Snapshot.ConfigVersion + 1

	err = config.ValidateSnapshot(snapshot)
	if err != nil {
		return ConfigRecord{}, fmt.Errorf("validate runtime configuration update: %w", err)
	}

	if actor == "" {
		actor = "admin"
	}

	if action == "" {
		action = "update"
	}

	record, err := s.store.Save(expectedRevision, ConfigRecord{Snapshot: snapshot, Actor: actor, Action: action})
	if err != nil {
		return ConfigRecord{}, fmt.Errorf("save runtime configuration update: %w", err)
	}

	s.activate(record.Snapshot)

	return record, nil
}

// Rollback activates a retained snapshot as a new monotonically increasing version.
func (s *AdminService) Rollback(expectedRevision, targetVersion uint64, actor string) (ConfigRecord, error) {
	history, err := s.History()
	if err != nil {
		return ConfigRecord{}, err
	}

	for _, record := range history {
		if record.Snapshot.ConfigVersion == targetVersion {
			return s.Update(expectedRevision, record.Snapshot, actor, "rollback_from_"+strconv.FormatUint(targetVersion, 10))
		}
	}

	return ConfigRecord{}, fmt.Errorf("%w: %d", errConfigVersionNotFound, targetVersion)
}

// SetWorker persists one worker lifecycle transition in the deployment snapshot.
func (s *AdminService) SetWorker(expectedRevision uint64, workerID, action, actor string) (ConfigRecord, error) {
	if workerID == "" {
		return ConfigRecord{}, errWorkerIDRequired
	}

	current, err := s.Current()
	if err != nil {
		return ConfigRecord{}, err
	}

	setting := config.WorkerSetting{WorkerID: workerID, Enabled: true}
	settings := current.Snapshot.WorkerSettings
	found := false

	for i := range settings {
		if settings[i].WorkerID == workerID {
			setting, found = settings[i], true

			break
		}
	}

	err = applyWorkerAction(&setting, action)
	if err != nil {
		return ConfigRecord{}, err
	}

	if found {
		for i := range settings {
			if settings[i].WorkerID == workerID {
				settings[i] = setting
			}
		}
	} else {
		settings = append(settings, setting)
	}

	current.Snapshot.WorkerSettings = settings

	return s.Update(expectedRevision, current.Snapshot, actor, "worker_"+action+":"+workerID)
}

// Rollout returns Control and registered-worker activation status.
func (s *AdminService) Rollout() RolloutStatus {
	snapshot := s.cache.Snapshot()
	workers := s.workers.Workers()

	s.mu.Lock()

	acked := make(map[string]uint64, len(s.acked))
	maps.Copy(acked, s.acked)
	s.mu.Unlock()

	out := RolloutStatus{ConfigVersion: snapshot.ConfigVersion, Control: rolloutApplied, Workers: make([]WorkerRollout, 0, len(workers))}
	for _, worker := range workers {
		status := "pending"
		if acked[worker.WorkerID] == snapshot.ConfigVersion {
			status = rolloutApplied
		}

		out.Workers = append(out.Workers, WorkerRollout{WorkerID: worker.WorkerID, Status: status})
	}

	return out
}

// SetupRolloutAcks subscribes to worker acknowledgements for published snapshots.
func (s *AdminService) SetupRolloutAcks(conn rolloutSubscriber) error {
	_, err := conn.Subscribe(RuntimeSnapshotAckSubject, func(msg *nats.Msg) {
		var ack struct {
			WorkerID      string `json:"worker_id"`
			ConfigVersion uint64 `json:"config_version"`
			Status        string `json:"status"`
		}

		decodeErr := json.Unmarshal(msg.Data, &ack)
		if decodeErr != nil || ack.WorkerID == "" || ack.Status != rolloutApplied {
			return
		}

		s.mu.Lock()
		s.acked[ack.WorkerID] = ack.ConfigVersion
		s.mu.Unlock()
	})
	if err != nil {
		return fmt.Errorf("subscribe runtime rollout acknowledgements: %w", err)
	}

	err = conn.Flush()
	if err != nil {
		return fmt.Errorf("flush runtime rollout acknowledgement subscription: %w", err)
	}

	return nil
}

// SetupConfigInvalidation applies newer snapshots on every Control. The
// durable store remains authoritative; this subject is only fast invalidation.
func (s *AdminService) SetupConfigInvalidation(conn rolloutSubscriber) error {
	_, err := conn.Subscribe(RuntimeSnapshotSubject, func(msg *nats.Msg) {
		var snapshot config.Snapshot
		if json.Unmarshal(msg.Data, &snapshot) != nil || config.ValidateSnapshot(snapshot) != nil {
			return
		}

		if snapshot.ConfigVersion <= s.cache.Snapshot().ConfigVersion {
			return
		}

		s.activate(snapshot)
	})
	if err != nil {
		return fmt.Errorf("subscribe runtime configuration invalidation: %w", err)
	}

	err = conn.Flush()
	if err != nil {
		return fmt.Errorf("flush runtime configuration invalidation subscription: %w", err)
	}

	return nil
}

// Workers returns the administrative worker view.
func (s *AdminService) Workers() []WorkerInfo { return s.workers.Workers() }

// Requests returns active request identifiers.
func (s *AdminService) Requests(ctx context.Context) []InFlightRequest {
	return s.inflight.Requests(ctx)
}

// CancelRequest safely cancels an active request through its normal stream path.
func (s *AdminService) CancelRequest(ctx context.Context, requestID string) bool {
	return s.inflight.Cancel(ctx, requestID)
}

// RunRepublisher makes rollout delivery resilient to worker startup and reconnects.
func (s *AdminService) RunRepublisher(ctx context.Context) {
	ticker := time.NewTicker(republishInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			record, err := s.store.Current()
			if err == nil && record.Snapshot.ConfigVersion > s.cache.Snapshot().ConfigVersion {
				s.activate(record.Snapshot)
			} else {
				s.publishCurrent()
			}
		}
	}
}

func applyWorkerAction(setting *config.WorkerSetting, action string) error {
	switch action {
	case "drain":
		setting.Draining = true
	case "undrain":
		setting.Draining = false
	case "disable":
		setting.Enabled = false
	case "enable":
		setting.Enabled = true
	default:
		return fmt.Errorf("%w: %q", errWorkerAction, action)
	}

	return nil
}

func (s *AdminService) activate(snapshot config.Snapshot) {
	s.cache.Replace(snapshot)
	s.workers.ApplySnapshot(snapshot)

	s.publishCurrent()
}

func (s *AdminService) publishCurrent() {
	if s.publisher == nil {
		return
	}

	raw, err := json.Marshal(s.cache.Snapshot())
	if err == nil {
		_ = s.publisher.Publish(RuntimeSnapshotSubject, raw)
	}
}

// AdminAuthenticator always requires a dedicated deployment-wide token.
type AdminAuthenticator struct{ token string }

// NewAdminAuthenticator rejects an empty administrative token.
func NewAdminAuthenticator(token string) (*AdminAuthenticator, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errAdminTokenRequired
	}

	return &AdminAuthenticator{token: token}, nil
}

// Authorize validates the administrative bearer token in constant time.
func (a *AdminAuthenticator) Authorize(ctx context.Context, header string) bool {
	if a == nil {
		return false
	}

	_, err := NewDeploymentAuthenticator(a.token).Authenticate(ctx, header)

	return err == nil
}
