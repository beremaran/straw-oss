package config

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const validUpstreamProxySnapshotCase = "valid"

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

func TestValidateSnapshotUpstreamProxy(t *testing.T) {
	t.Parallel()

	for name, upstreamProxy := range map[string]*ExecutorPoolUpstreamProxy{
		validUpstreamProxySnapshotCase: {ID: "brightdata-resi", TrustedRemoteResolution: true},
		"untrusted":                    {ID: "brightdata-resi"},
		"empty id":                     {TrustedRemoteResolution: true},
		"invalid id":                   {ID: "bad proxy", TrustedRemoteResolution: true},
		"oversized id":                 {ID: strings.Repeat("x", MaxUpstreamProxyIDBytes+1), TrustedRemoteResolution: true},
	} {
		t.Run(name, func(t *testing.T) {
			s := validRuntimeSnapshot()
			s.ExecutorPools[0].UpstreamProxy = upstreamProxy

			err := ValidateSnapshot(s)
			if name == validUpstreamProxySnapshotCase && err != nil {
				t.Fatalf("ValidateSnapshot() error = %v", err)
			}
			if name != validUpstreamProxySnapshotCase && !errors.Is(err, ErrInvalidSnapshot) {
				t.Fatalf("ValidateSnapshot() error = %v, want ErrInvalidSnapshot", err)
			}
		})
	}

	s := validRuntimeSnapshot()
	s.ExecutorPools[0].UpstreamProxy = &ExecutorPoolUpstreamProxy{ID: "shared", TrustedRemoteResolution: true}
	s.ExecutorPools = append(s.ExecutorPools, ExecutorPool{ID: "secondary", ExecutorType: "egress", Enabled: true, UpstreamProxy: &ExecutorPoolUpstreamProxy{ID: "shared", TrustedRemoteResolution: true}})
	err := ValidateSnapshot(s)
	if err != nil {
		t.Fatalf("ValidateSnapshot() with shared profile error = %v", err)
	}
}

func TestExecutorPoolUpstreamProxyStrictJSON(t *testing.T) {
	t.Parallel()

	decoder := json.NewDecoder(strings.NewReader(`{"id":"proxy-pool","executor_type":"egress","upstream_proxy":{"id":"profile","trusted_remote_resolution":true,"endpoint":"http://not-allowed:8080"}}`))
	decoder.DisallowUnknownFields()

	var pool ExecutorPool
	err := decoder.Decode(&pool)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Decode() error = %v, want unknown field", err)
	}
}

func TestSnapshotCloneIsolatesExecutorPoolUpstreamProxy(t *testing.T) {
	t.Parallel()

	s := validRuntimeSnapshot()
	s.ExecutorPools[0].UpstreamProxy = &ExecutorPoolUpstreamProxy{ID: "profile", TrustedRemoteResolution: true}
	cloned := s.Clone()
	cloned.ExecutorPools[0].UpstreamProxy.ID = "changed"
	cloned.ExecutorPools[0].UpstreamProxy.TrustedRemoteResolution = false

	if s.ExecutorPools[0].UpstreamProxy.ID != "profile" || !s.ExecutorPools[0].UpstreamProxy.TrustedRemoteResolution {
		t.Fatalf("Clone() mutated source upstream proxy: %+v", s.ExecutorPools[0].UpstreamProxy)
	}
	if cloned.ExecutorPools[0].UpstreamProxy == s.ExecutorPools[0].UpstreamProxy {
		t.Fatal("Clone() retained upstream proxy pointer")
	}
}

func TestPrepareSnapshotNormalizesDestinationRulesAndClearsDerivedFields(t *testing.T) {
	t.Parallel()

	s := validRuntimeSnapshot()
	s.DenyRules = []DenyRule{
		{ID: "host-rule", RuleType: denyRuleTypeHost, Action: denyRuleActionDeny, Enabled: true, RawPattern: "BÜCHER.Example.", NormalizedCIDR: "stale"},
		{ID: "suffix-rule", RuleType: denyRuleTypeHostSuffix, Action: denyRuleActionDeny, Enabled: true, RawPattern: "*.Example.COM"},
		{ID: "cidr-rule", RuleType: denyRuleTypeCIDR, Action: denyRuleActionDeny, Enabled: true, RawPattern: "192.168.1.42/24"},
		{ID: "ip-rule", RuleType: denyRuleTypeIP, Action: denyRuleActionDeny, Enabled: true, RawPattern: "192.0.2.1"},
		{ID: "cname-rule", RuleType: denyRuleTypeCNAMESuffix, Action: denyRuleActionDeny, Enabled: true, RawPattern: ".Cdn.Example"},
		{ID: "private-rule", RuleType: denyRuleTypePrivateRange, Action: denyRuleActionDeny, Enabled: true, RawPattern: "10.0.0.0/8"},
		{ID: "metadata-rule", RuleType: denyRuleTypeMetadataIP, Action: denyRuleActionDeny, Enabled: true, RawPattern: metadataIP169254169254},
	}

	prepared, err := PrepareSnapshot(s)
	if err != nil {
		t.Fatalf("PrepareSnapshot() error = %v", err)
	}
	want := []DenyRule{
		{ID: "host-rule", NormalizedHost: "xn--bcher-kva.example"},
		{ID: "suffix-rule", NormalizedHost: "example.com"},
		{ID: "cidr-rule", NormalizedCIDR: "192.168.1.0/24"},
		{ID: "ip-rule", NormalizedIP: "192.0.2.1"},
		{ID: "cname-rule", NormalizedName: "cdn.example"},
		{ID: "private-rule", NormalizedCIDR: "10.0.0.0/8"},
		{ID: "metadata-rule", NormalizedIP: metadataIP169254169254},
	}
	for i, rule := range prepared.DenyRules {
		if rule.ID != want[i].ID || rule.NormalizedHost != want[i].NormalizedHost || rule.NormalizedCIDR != want[i].NormalizedCIDR || rule.NormalizedIP != want[i].NormalizedIP || rule.NormalizedName != want[i].NormalizedName {
			t.Fatalf("rule %q = %+v, want derived fields from %+v", rule.ID, rule, want[i])
		}
	}
}

func TestValidateSnapshotRejectsMalformedRuntimePolicyRecords(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*Snapshot){
		"route conditions": func(s *Snapshot) { s.RoutingRules[0].Match.IngressType = "unsupported" },
		"sticky coherence": func(s *Snapshot) { s.RoutingRules[0].AllowStickyFallback = true },
		"pool semantics":   func(s *Snapshot) { s.ExecutorPools[0].AllowedCountries = []string{"AU", "AU"} },
		"destination host": func(s *Snapshot) {
			s.DenyRules = []DenyRule{{ID: "d", RuleType: denyRuleTypeHost, Action: denyRuleActionDeny, RawPattern: "https://bad.example"}}
		},
		"destination cidr": func(s *Snapshot) {
			s.DenyRules = []DenyRule{{ID: "d", RuleType: "cidr", Action: "deny", RawPattern: "not-a-cidr"}}
		},
		"destination metadata": func(s *Snapshot) {
			s.DenyRules = []DenyRule{{ID: "d", RuleType: "metadata_ip", Action: "deny", RawPattern: "not-an-ip"}}
		},
		"injection operation": func(s *Snapshot) {
			s.InjectionPolicies = []InjectionPolicy{{ID: "p", Enabled: true, Operations: []InjectionOperation{{Op: "replace", HeaderName: "X-Test"}}}}
		},
		"fingerprint record": func(s *Snapshot) {
			s.FingerprintProfiles = []FingerprintProfile{{Name: "", Enabled: true}}
		},
		"worker setting": func(s *Snapshot) {
			s.WorkerSettings = []WorkerSetting{{WorkerID: "bad worker", Enabled: true}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s := validRuntimeSnapshot()
			mutate(&s)
			err := ValidateSnapshot(s)
			if !errors.Is(err, ErrInvalidSnapshot) {
				t.Fatalf("ValidateSnapshot() error = %v, want ErrInvalidSnapshot", err)
			}
		})
	}
}
