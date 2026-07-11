package control

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/egress"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/testutil"
)

const (
	workerNATSTestTenant  = "ten_a"
	workerRegTestExecutor = "egress"
	workerTestVersion     = "test"
)

func TestWorkerDiscoveryOverNATSDuplicateSessionAndTimeouts(t *testing.T) {
	t.Parallel()

	h := newTestAdmin(t)
	clock := &wireClock{t: time.Now()}
	reg := NewWorkerRegistry(h.workerCreds, DefaultWorkerTimings(), clock.Now)
	h.h.Workers = reg

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	cred := WorkerCredential{
		ID:                     "wcred_wire",
		Status:                 WorkerCredentialStatusActive,
		ExecutorType:           workerRegTestExecutor,
		TenantScope:            []string{workerNATSTestTenant},
		AllowedPools:           []AllowedPool{{TenantID: workerNATSTestTenant, PoolID: routingTestPool1}},
		PublicKeyEd25519Base64: base64.StdEncoding.EncodeToString(pub),
	}

	err = h.workerCreds.Create(context.Background(), cred)
	if err != nil {
		t.Fatalf("create worker cred: %v", err)
	}

	srv := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := mustConnectNATS(t, srv.URL())
	t.Cleanup(controlConn.Close)

	err = SetupWorkerDiscoverySubscriptions(controlConn, reg)
	if err != nil {
		t.Fatalf("setup worker discovery: %v", err)
	}

	workerConn := mustConnectNATS(t, srv.URL())
	t.Cleanup(workerConn.Close)

	id := egress.Identity{
		WorkerID:     workerRegTestWorker1,
		CredentialID: cred.ID,
		ExecutorType: workerRegTestExecutor,
		PrivateKey:   priv,
	}
	caps := egress.Capabilities{
		AllowedPools:          []*strawpb.RegisterRequest_PoolRef{{TenantId: workerNATSTestTenant, PoolId: routingTestPool1}},
		SoftwareVersion:       workerTestVersion,
		MaxConcurrency:        4,
		SupportedIngressModes: []string{IngressTypeREST},
	}

	sess1, err := egress.Register(context.Background(), workerConn, id, caps)
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	err = egress.Heartbeat(context.Background(), workerConn, id, sess1, strawpb.WorkerHealth_WORKER_HEALTH_READY, 0, 4, 4, false)
	if err != nil {
		t.Fatalf("first heartbeat: %v", err)
	}

	platformToken := h.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	workers := mustListWorkers(t, h.h, platformToken)
	if len(workers) != 1 {
		t.Fatalf("workers = %+v, want 1 row", workers)
	}
	if workers[0].SessionID != sess1 || workers[0].RuntimeState != string(RuntimeReady) {
		t.Fatalf("first worker view = %+v, want ready session %q", workers[0], sess1)
	}

	sess2, err := egress.Register(context.Background(), workerConn, id, caps)
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if sess2 == sess1 {
		t.Fatalf("duplicate registration reused session %q", sess1)
	}

	err = egress.Heartbeat(context.Background(), workerConn, id, sess2, strawpb.WorkerHealth_WORKER_HEALTH_READY, 0, 4, 4, false)
	if err != nil {
		t.Fatalf("second heartbeat: %v", err)
	}

	workers = mustListWorkers(t, h.h, platformToken)
	if workers[0].SessionID != sess2 || workers[0].RuntimeState != string(RuntimeReady) {
		t.Fatalf("second worker view = %+v, want ready session %q", workers[0], sess2)
	}

	err = egress.Heartbeat(context.Background(), workerConn, id, sess1, strawpb.WorkerHealth_WORKER_HEALTH_READY, 0, 4, 4, false)
	if err != nil {
		t.Fatalf("stale heartbeat request: %v", err)
	}
	workers = mustListWorkers(t, h.h, platformToken)
	if workers[0].SessionID != sess2 || workers[0].RuntimeState != string(RuntimeReady) {
		t.Fatalf("stale heartbeat changed worker view = %+v", workers[0])
	}

	clock.Advance(16 * time.Second)
	workers = mustListWorkers(t, h.h, platformToken)
	if workers[0].RuntimeState != string(RuntimeUnavailable) {
		t.Fatalf("after 16s worker state = %s, want unavailable", workers[0].RuntimeState)
	}

	clock.Advance(15 * time.Second)
	workers = mustListWorkers(t, h.h, platformToken)
	if workers[0].RuntimeState != string(RuntimeDead) {
		t.Fatalf("after 31s worker state = %s, want dead", workers[0].RuntimeState)
	}
}

func TestWorkerRunLoopAppearsInAdminWorkersAndDrainsOnCancel(t *testing.T) {
	t.Parallel()

	h := newTestAdmin(t)
	clock := &wireClock{t: time.Now()}
	reg := NewWorkerRegistry(h.workerCreds, DefaultWorkerTimings(), clock.Now)
	h.h.Workers = reg

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	cred := WorkerCredential{
		ID:                     "wcred_run",
		Status:                 WorkerCredentialStatusActive,
		ExecutorType:           workerRegTestExecutor,
		TenantScope:            []string{workerNATSTestTenant},
		AllowedPools:           []AllowedPool{{TenantID: workerNATSTestTenant, PoolID: routingTestPool1}},
		PublicKeyEd25519Base64: base64.StdEncoding.EncodeToString(pub),
	}

	err = h.workerCreds.Create(context.Background(), cred)
	if err != nil {
		t.Fatalf("create worker cred: %v", err)
	}

	srv := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := mustConnectNATS(t, srv.URL())
	t.Cleanup(controlConn.Close)

	err = SetupWorkerDiscoverySubscriptions(controlConn, reg)
	if err != nil {
		t.Fatalf("setup worker discovery: %v", err)
	}

	workerConn := mustConnectNATS(t, srv.URL())
	t.Cleanup(workerConn.Close)

	id := egress.Identity{
		WorkerID:     workerRegTestWorker2,
		CredentialID: cred.ID,
		ExecutorType: workerRegTestExecutor,
		PrivateKey:   priv,
	}
	caps := egress.Capabilities{
		AllowedPools:          []*strawpb.RegisterRequest_PoolRef{{TenantId: workerNATSTestTenant, PoolId: routingTestPool1}},
		SoftwareVersion:       workerTestVersion,
		MaxConcurrency:        2,
		SupportedIngressModes: []string{"rest"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := &atomic.Bool{}
	var runErr error
	var runMu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		runErr = egress.Run(ctx, workerConn, id, caps, egress.NewExecutor(egress.ExecutorOptions{}), 25*time.Millisecond, ready)
		runMu.Lock()
		defer runMu.Unlock()
	}()

	platformToken := h.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	waitForWorkerState(t, h.h, platformToken, workerRegTestWorker2, string(RuntimeReady))

	if !ready.Load() {
		t.Fatalf("ready flag = false after registration, want true (docs/implementation-history.md#p0-38)")
	}

	cancel()
	<-done
	if runErr != nil {
		t.Fatalf("worker run loop error: %v", runErr)
	}

	if ready.Load() {
		t.Fatalf("ready flag = true after drain, want false (docs/implementation-history.md#p0-38)")
	}

	waitForWorkerState(t, h.h, platformToken, workerRegTestWorker2, string(RuntimeDraining))
}

func mustConnectNATS(t *testing.T, url string) *natsx.Connection {
	t.Helper()

	conn, err := natsx.Connect(natsx.ConnectOptions{
		Servers:         []string{url},
		ReconnectWait:   10 * time.Millisecond,
		PingInterval:    100 * time.Millisecond,
		MaxPingFailures: 1,
	})
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}

	return conn
}

func mustListWorkers(t *testing.T, h *AdminHandlers, token string) []workerAdminView {
	t.Helper()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/workers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ListWorkers(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list workers status = %d body=%s", w.Code, w.Body.String())
	}

	var views []workerAdminView

	err := json.Unmarshal(w.Body.Bytes(), &views)
	if err != nil {
		t.Fatalf("unmarshal workers: %v", err)
	}

	return views
}

func waitForWorkerState(t *testing.T, h *AdminHandlers, token, workerID, want string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, row := range mustListWorkers(t, h, token) {
			if row.WorkerID == workerID && row.RuntimeState == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("worker %s did not reach state %s", workerID, want)
}

type wireClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *wireClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *wireClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = c.t.Add(d)
}
