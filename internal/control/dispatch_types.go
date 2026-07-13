package control

import (
	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/receipt"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

type dispatchResult struct {
	status                     uint32
	headers                    []*strawpb.Header
	body                       []byte
	size                       uint64
	egressMs                   int64
	selectedFingerprintProfile string
	executedFingerprintProfile string
	responseUpload             *receipt.ResponseUpload
	responseReceiptID          string
	responseReceiptSHA256      string
}

type snapshotRules struct {
	deploymentID string
	rules        []config.RoutingRule
}

func (s snapshotRules) RulesForDeployment(deploymentID string) []RoutingRule {
	if deploymentID != s.deploymentID {
		return nil
	}

	out := make([]RoutingRule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, RoutingRule{
			ID:                      r.ID,
			DeploymentID:            deploymentID,
			Priority:                r.Priority,
			Enabled:                 r.Enabled,
			Match:                   matchFromSnapshot(r.Match),
			TargetPoolID:            r.TargetPoolID,
			StickySessionTTLSeconds: r.StickySessionTTLSeconds,
			AllowStickyFallback:     r.AllowStickyFallback,
		})
	}

	return out
}

func matchFromSnapshot(m config.MatchConditions) MatchConditions {
	return MatchConditions{
		Tags:        append([]string(nil), m.Tags...),
		Country:     m.Country,
		Region:      m.Region,
		IPType:      m.IPType,
		IngressType: m.IngressType,
		TargetHost:  m.TargetHost,
	}
}
