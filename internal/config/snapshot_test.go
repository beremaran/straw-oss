package config

import (
	"reflect"
	"testing"
)

func TestTenantSnapshotCloneDeepCopiesPolicySlices(t *testing.T) {
	original := TenantSnapshot{
		TenantID:         "tenant_a",
		ConfigVersion:    7,
		RevokedAPIKeyIDs: []string{"key_a"},
		RoutingRules: []RoutingRule{{
			ID:    "route_a",
			Match: MatchConditions{Tags: []string{"fast"}},
		}},
		ExecutorPools: []ExecutorPool{{
			ID:   "pool_a",
			Tags: []string{"au"},
		}},
		InjectionPolicies: []InjectionPolicy{{
			ID: "inject_a",
			Operations: []InjectionOperation{{
				Op:          "set",
				HeaderName:  "X-Test",
				ValueBase64: "dmFsdWU=",
			}},
		}},
	}

	clone := original.Clone()
	clone.RevokedAPIKeyIDs[0] = "key_b"
	clone.RoutingRules[0].Match.Tags[0] = "slow"
	clone.ExecutorPools[0].Tags[0] = "us"
	clone.InjectionPolicies[0].Operations[0].ValueBase64 = "b3RoZXI="

	if original.RevokedAPIKeyIDs[0] != "key_a" {
		t.Fatalf("original revoked key mutated to %q", original.RevokedAPIKeyIDs[0])
	}
	if original.RoutingRules[0].Match.Tags[0] != "fast" {
		t.Fatalf("original route tag mutated to %q", original.RoutingRules[0].Match.Tags[0])
	}
	if original.ExecutorPools[0].Tags[0] != "au" {
		t.Fatalf("original pool tag mutated to %q", original.ExecutorPools[0].Tags[0])
	}
	if original.InjectionPolicies[0].Operations[0].ValueBase64 != "dmFsdWU=" {
		t.Fatalf("original injection value mutated to %q", original.InjectionPolicies[0].Operations[0].ValueBase64)
	}
}

func TestTenantSnapshotClonePreservesImmutableFingerprintProfileMetadata(t *testing.T) {
	profile := FingerprintProfile{Name: "chrome_120", ScopeType: "global", Enabled: true, SupportedByWorker: true}
	metadata := map[string]string{
		"ExecutorType":     "egress",
		"ProfileRef":       "tls-client/v1.15.1:profiles.Chrome_120",
		"ContractRevision": "chrome_120_v1_15_1",
	}
	profileValue := reflect.ValueOf(&profile).Elem()
	for field, value := range metadata {
		fieldValue := profileValue.FieldByName(field)
		if !fieldValue.IsValid() || !fieldValue.CanSet() || fieldValue.Kind() != reflect.String {
			t.Fatalf("fingerprint profile missing immutable %s metadata", field)
		}
		fieldValue.SetString(value)
	}
	original := TenantSnapshot{FingerprintProfiles: []FingerprintProfile{profile}}

	clone := original.Clone()
	if len(clone.FingerprintProfiles) != 1 || clone.FingerprintProfiles[0].Name != "chrome_120" {
		t.Fatalf("cloned profiles = %+v, want chrome_120 descriptor", clone.FingerprintProfiles)
	}

	for field, want := range metadata {
		fieldValue := reflect.ValueOf(clone.FingerprintProfiles[0]).FieldByName(field)
		if !fieldValue.IsValid() || fieldValue.Kind() != reflect.String {
			t.Fatalf("cloned fingerprint profile missing immutable %s metadata", field)
		}
		if fieldValue.String() != want {
			t.Fatalf("cloned fingerprint profile %s = %q, want %q", field, fieldValue.String(), want)
		}
	}
}
