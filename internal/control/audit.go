package control

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/beremaran/straw/v2/internal/config"
)

// auditFieldPathAll is the documented whole-object field_path sentinel
// (docs/tasks/p0/50): call sites pass it for whole-object mutations, and
// recordAudit refines it into the dotted paths of the fields that actually
// differ when both old and new values are present. It stays "*" when the
// change is not diffable (create, delete, or non-object payloads).
const auditFieldPathAll = "*"

var auditFieldNameAliases = map[string]string{
	"Match": "match_conditions",
}

// AuditRecord mirrors the `config_audit_source` table. actor_id is always
// the API key ID in P0 (docs/planning/26). Secret fields must be redacted
// before a record is written; this task never writes plaintext key
// material or hashes into audit records.
// AuditRecord mirrors the config audit table.
type AuditRecord struct {
	ID            int64
	TenantID      string // empty for platform-scoped actions
	ActorType     string
	ActorID       string
	ResourceType  string
	ResourceID    string
	Action        string
	CreatedAt     time.Time
	ConfigVersion uint64
	FieldPath     string
	OldValueJSON  string
	NewValueJSON  string
	SkipPostgres  bool
}

// AuditStore persists config audit records.
// AuditStore persists config audit records.
type AuditStore interface {
	Record(ctx context.Context, record AuditRecord) error
	ListTenant(ctx context.Context, tenantID string) ([]AuditRecord, error)
	// ListTenantPage returns a tenant's audit history ordered created_at
	// descending then id ascending, per the shared list contract
	// (docs/planning/26). Callers pass an already-clamped limit.
	ListTenantPage(ctx context.Context, tenantID string, limit, offset int) ([]AuditRecord, error)
}

// InMemoryAuditStore is the P0 store implementation.
// InMemoryAuditStore is the P0 audit store implementation.
type InMemoryAuditStore struct {
	mu      sync.RWMutex
	records []AuditRecord
	nextID  int64
}

// NewInMemoryAuditStore builds an empty in-memory audit store.
func NewInMemoryAuditStore() *InMemoryAuditStore {
	return &InMemoryAuditStore{}
}

// Record appends an audit record.
func (s *InMemoryAuditStore) Record(_ context.Context, record AuditRecord) error {
	if record.SkipPostgres {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++

	record.ID = s.nextID
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	s.records = append(s.records, record)

	return nil
}

// ListTenant returns audit records for a tenant.
func (s *InMemoryAuditStore) ListTenant(_ context.Context, tenantID string) ([]AuditRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []AuditRecord

	for _, r := range s.records {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}

	return out, nil
}

// ListTenantPage returns a paginated, tenant-scoped view of the audit log,
// sorted created_at descending then id ascending.
func (s *InMemoryAuditStore) ListTenantPage(_ context.Context, tenantID string, limit, offset int) ([]AuditRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []AuditRecord

	for _, r := range s.records {
		if r.TenantID == tenantID {
			matched = append(matched, r)
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].CreatedAt.After(matched[j].CreatedAt)
		}

		return matched[i].ID < matched[j].ID
	})

	if offset >= len(matched) {
		return []AuditRecord{}, nil
	}

	end := min(offset+limit, len(matched))

	return matched[offset:end], nil
}

// recordAudit is a small helper used by admin handlers so every mutation
// site records actor, resource, and action consistently.
func recordAudit(
	ctx context.Context,
	store AuditStore,
	identity Identity,
	resourceType, resourceID, action string,
	configVersion uint64,
	fieldPath string,
	oldVal, newVal any,
	skipPostgres bool,
) {
	if store == nil {
		return
	}

	oldJSON, _ := redactAndMarshal(oldVal)
	newJSON, _ := redactAndMarshal(newVal)
	fieldPath = deriveFieldPath(fieldPath, oldJSON, newJSON)

	_ = store.Record(ctx, AuditRecord{
		TenantID:      identity.TenantID,
		ActorType:     configActorTypeAPIKey,
		ActorID:       identity.APIKeyID,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Action:        action,
		ConfigVersion: configVersion,
		FieldPath:     fieldPath,
		OldValueJSON:  oldJSON,
		NewValueJSON:  newJSON,
		SkipPostgres:  skipPostgres,
	})
}

// deriveFieldPath refines the whole-object sentinel into a comma-joined,
// sorted list of dotted paths for the fields that differ between the
// redacted old and new JSON (docs/planning/26 "field_path"). Explicit
// per-field paths pass through unchanged; the sentinel is kept when either
// side is absent, either side is not a JSON object, or nothing differs.
func deriveFieldPath(fieldPath, oldJSON, newJSON string) string {
	if fieldPath != auditFieldPathAll || oldJSON == "" || newJSON == "" {
		return fieldPath
	}

	var oldObj, newObj map[string]any
	if json.Unmarshal([]byte(oldJSON), &oldObj) != nil || json.Unmarshal([]byte(newJSON), &newObj) != nil {
		return fieldPath
	}

	paths := diffFieldPaths("", oldObj, newObj)
	if len(paths) == 0 {
		return fieldPath
	}

	sort.Strings(paths)

	return strings.Join(paths, ",")
}

// diffFieldPaths walks two JSON objects and returns the dotted paths whose
// values differ. Arrays and scalars are compared as leaves, so a changed
// list element reports the list's path (e.g. "operations"), matching the
// whole-object diff granularity task 44 established.
func diffFieldPaths(prefix string, oldObj, newObj map[string]any) []string {
	keys := make(map[string]struct{}, len(oldObj)+len(newObj))
	for k := range oldObj {
		keys[k] = struct{}{}
	}

	for k := range newObj {
		keys[k] = struct{}{}
	}

	var out []string

	for k := range keys {
		path := auditFieldPath(prefix, k)

		if auditFieldPathIgnored(path) {
			continue
		}

		oldVal, inOld := oldObj[k]
		newVal, inNew := newObj[k]
		out = append(out, diffFieldValuePaths(path, oldVal, newVal, inOld, inNew)...)
	}

	return out
}

func auditFieldPath(prefix, name string) string {
	path := auditFieldPathSegment(name)
	if prefix != "" {
		path = prefix + "." + path
	}

	return path
}

func diffFieldValuePaths(path string, oldVal, newVal any, inOld, inNew bool) []string {
	if !inOld || !inNew {
		return []string{path}
	}

	oldMap, oldIsMap := oldVal.(map[string]any)

	newMap, newIsMap := newVal.(map[string]any)
	if oldIsMap && newIsMap {
		return diffFieldPaths(path, oldMap, newMap)
	}

	if reflect.DeepEqual(oldVal, newVal) {
		return nil
	}

	return []string{path}
}

func auditFieldPathSegment(name string) string {
	if alias, ok := auditFieldNameAliases[name]; ok {
		return alias
	}

	if name == "" || strings.Contains(name, "_") {
		return name
	}

	var b strings.Builder

	for i, r := range name {
		if auditSnakeBoundary(name, i, r) {
			b.WriteByte('_')
		}

		b.WriteRune(unicode.ToLower(r))
	}

	return b.String()
}

func auditSnakeBoundary(name string, i int, r rune) bool {
	if i == 0 || !unicode.IsUpper(r) {
		return false
	}

	prev := rune(name[i-1])

	next := rune(0)
	if i+1 < len(name) {
		next = rune(name[i+1])
	}

	return unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && next != 0 && unicode.IsLower(next))
}

func auditFieldPathIgnored(path string) bool {
	switch path {
	case "config_version", "updated_at":
		return true
	default:
		return false
	}
}

// redactAndMarshal converts the object to its redacted JSON representation.
// It classifies and redacts secret fields like value_base64 in injection policies.
func redactAndMarshal(v any) (string, error) {
	if v == nil {
		return "", nil
	}

	switch val := v.(type) {
	case config.InjectionPolicy:
		val = redactInjectionPolicy(val)
		v = val
	case *config.InjectionPolicy:
		if val != nil {
			clone := redactInjectionPolicy(*val)
			v = &clone
		}
	case APIKeyRecord:
		val.SecretHash = requestMetadataRedacted
		v = val
	case *APIKeyRecord:
		if val != nil {
			clone := *val
			clone.SecretHash = requestMetadataRedacted
			v = &clone
		}
	}

	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("json marshal: %w", err)
	}

	return string(b), nil
}

// auditStoreWithEvents wraps an AuditStore so every successful Record also
// mirrors into the config_audit_events ClickHouse sink (docs/tasks/p0/32),
// covering every recordAudit call site (tenant, API key, worker credential,
// routing/deny/injection/pool config, worker admin, request cancel) from one
// choke point. AuditRecord carries config_version and redacted old/new value
// JSON (see recordAudit / redactAndMarshal), so the mirrored ClickHouse rows
// are populated with the same enriched, redacted fields as the Postgres
// config_audit_source records.
type auditStoreWithEvents struct {
	AuditStore
	events ConfigAuditRecorder
}

// NewAuditStoreWithEvents wraps store so every Record call also enqueues a
// config_audit_events row. If events is nil, store is returned unwrapped.
func NewAuditStoreWithEvents(store AuditStore, events ConfigAuditRecorder) AuditStore {
	if events == nil {
		return store
	}

	return &auditStoreWithEvents{AuditStore: store, events: events}
}

func (s *auditStoreWithEvents) Record(ctx context.Context, record AuditRecord) error {
	err := s.AuditStore.Record(ctx, record)
	if err != nil {
		return fmt.Errorf("record audit: %w", err)
	}

	s.events.Enqueue(ConfigAuditEvent{
		Timestamp:     time.Now().UTC(),
		TenantID:      record.TenantID,
		ActorType:     record.ActorType,
		ActorID:       record.ActorID,
		ConfigType:    record.ResourceType,
		ResourceID:    record.ResourceID,
		Action:        record.Action,
		ConfigVersion: record.ConfigVersion,
		FieldPath:     record.FieldPath,
		OldValueJSON:  record.OldValueJSON,
		NewValueJSON:  record.NewValueJSON,
	})

	return nil
}
