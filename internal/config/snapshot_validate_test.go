package config

import (
	"errors"
	"testing"
)

func validRuntimeSnapshot() Snapshot {
	s := NewSnapshot(1)
	s.ExecutorPools = []ExecutorPool{{ID: DefaultPoolID, ExecutorType: "egress", Enabled: true}}
	s.RoutingRules = []RoutingRule{{ID: "default", Enabled: true, TargetPoolID: DefaultPoolID}}

	return s
}

func TestValidateSnapshotRejectsUnknownRoutePool(t *testing.T) {
	t.Parallel()
	s := validRuntimeSnapshot()
	s.RoutingRules[0].TargetPoolID = "missing"
	err := ValidateSnapshot(s)
	if !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("ValidateSnapshot() error = %v, want ErrInvalidSnapshot", err)
	}
}

func TestValidateSnapshotAcceptsDeploymentPolicy(t *testing.T) {
	t.Parallel()
	err := ValidateSnapshot(validRuntimeSnapshot())
	if err != nil {
		t.Fatalf("ValidateSnapshot() error = %v", err)
	}
}
