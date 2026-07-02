package config

// TenantSnapshot is the immutable tenant config view consumed by control-plane
// admission and routing decisions.
type TenantSnapshot struct {
	TenantID         string
	ConfigVersion    uint64
	RevokedAPIKeyIDs []string
}

// NewTenantSnapshot copies slice fields so callers can keep their input buffers
// mutable without affecting cached snapshots.
func NewTenantSnapshot(tenantID string, configVersion uint64, revokedAPIKeyIDs []string) TenantSnapshot {
	return TenantSnapshot{
		TenantID:         tenantID,
		ConfigVersion:    configVersion,
		RevokedAPIKeyIDs: append([]string(nil), revokedAPIKeyIDs...),
	}
}

// Clone returns a deep copy of the snapshot.
func (s TenantSnapshot) Clone() TenantSnapshot {
	return NewTenantSnapshot(s.TenantID, s.ConfigVersion, s.RevokedAPIKeyIDs)
}
