package egress

import (
	"crypto/ed25519"
	"testing"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

const (
	testWorker1 = "worker-1"
	testWcred1  = "wcred_1"
	testTenantA = "ten_a"
	testPool1   = "pool_1"
	testEgress  = "egress"
)

func TestBuildRegisterRequestSignsVerifiably(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	id := Identity{WorkerID: testWorker1, CredentialID: testWcred1, ExecutorType: testEgress, PrivateKey: priv}
	caps := Capabilities{
		AllowedPools:   []*strawpb.RegisterRequest_PoolRef{{TenantId: testTenantA, PoolId: testPool1}},
		Tags:           []string{"datacenter"},
		MaxConcurrency: 4,
	}

	req, err := BuildRegisterRequest(id, caps)
	if err != nil {
		t.Fatalf("BuildRegisterRequest: %v", err)
	}
	if req.GetProtocolMajor() != ProtocolMajor {
		t.Fatalf("protocol major = %d, want %d", req.GetProtocolMajor(), ProtocolMajor)
	}
	if !strawpb.VerifyRegistrationSignature(pub, req, req.GetSignedToken()) {
		t.Fatal("signature produced by BuildRegisterRequest did not verify")
	}

	// A wrong public key must not verify.
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if strawpb.VerifyRegistrationSignature(otherPub, req, req.GetSignedToken()) {
		t.Fatal("signature verified under the wrong public key")
	}
}

func TestIdentityInboxPrefix(t *testing.T) {
	t.Parallel()
	id := Identity{WorkerID: testWorker1}
	got, err := id.InboxPrefix()
	if err != nil {
		t.Fatalf("InboxPrefix error: %v", err)
	}
	if got != "_INBOX.wrk.worker-1" {
		t.Fatalf("InboxPrefix = %q, want _INBOX.wrk.worker-1", got)
	}
}

func TestBuildHeartbeat(t *testing.T) {
	t.Parallel()
	id := Identity{WorkerID: testWorker1}
	hb := BuildHeartbeat(id, "sess_1", strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED, 2, 6, 8, true)
	if hb.GetSessionId() != "sess_1" || hb.GetHealth() != strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED {
		t.Fatalf("heartbeat = %+v, unexpected session/health", hb)
	}
	if !hb.GetDraining() {
		t.Fatal("draining flag not set")
	}
}
