package natsx

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConnectAndVerifyMaxPayload(t *testing.T) {
	t.Parallel()

	server := newFakeNATSServer(t, 2_000_000)

	conn, err := Connect(ConnectOptions{
		Servers:         []string{server.URL()},
		ReconnectWait:   10 * time.Millisecond,
		PingInterval:    100 * time.Millisecond,
		MaxPingFailures: 1,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	err = waitForCondition(2*time.Second, func() bool { return conn.IsConnected() })
	if err != nil {
		t.Fatal(err)
	}

	if got := conn.ConnectedUrl(); got != server.URL() {
		t.Fatalf("ConnectedUrl() = %q, want %q", got, server.URL())
	}

	err = ValidateConnectedMaxPayload(conn, 1_048_576, 1_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("ValidateConnectedMaxPayload() error = %v", err)
	}
}

func TestConnectReconnects(t *testing.T) {
	t.Parallel()

	first := newFakeNATSServer(t, 2_000_000)
	second := newFakeNATSServer(t, 2_000_000)

	conn, err := Connect(ConnectOptions{
		Servers:         []string{first.URL(), second.URL()},
		ReconnectWait:   25 * time.Millisecond,
		PingInterval:    100 * time.Millisecond,
		MaxPingFailures: 1,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	err = waitForCondition(2*time.Second, func() bool { return conn.ConnectedUrl() == first.URL() })
	if err != nil {
		t.Fatal(err)
	}

	first.Close()

	err = waitForCondition(5*time.Second, func() bool { return conn.ConnectedUrl() == second.URL() })
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateConnectedMaxPayloadRejectsSmallServerLimit(t *testing.T) {
	t.Parallel()

	server := newFakeNATSServer(t, 1_100_000)

	conn, err := Connect(ConnectOptions{
		Servers:         []string{server.URL()},
		ReconnectWait:   10 * time.Millisecond,
		PingInterval:    100 * time.Millisecond,
		MaxPingFailures: 1,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	err = ValidateConnectedMaxPayload(conn, 1_048_576, 1_000_000, 1_000_000)
	if err == nil {
		t.Fatal("ValidateConnectedMaxPayload() = nil, want error")
	}
}

func TestConnectionDrain(t *testing.T) {
	t.Parallel()

	server := newFakeNATSServer(t, 2_000_000)

	conn, err := Connect(ConnectOptions{
		Servers:         []string{server.URL()},
		ReconnectWait:   10 * time.Millisecond,
		PingInterval:    100 * time.Millisecond,
		MaxPingFailures: 1,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	err = conn.Drain()
	if err != nil {
		t.Fatalf("Drain() error = %v", err)
	}

	err = waitForCondition(2*time.Second, func() bool { return conn.IsClosed() })
	if err != nil {
		t.Fatal(err)
	}
}

type fakeNATSServer struct {
	ln         net.Listener
	maxPayload int

	mu     sync.Mutex
	active net.Conn
}

func newFakeNATSServer(t *testing.T, maxPayload int) *fakeNATSServer {
	t.Helper()

	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	srv := &fakeNATSServer{ln: ln, maxPayload: maxPayload}
	go srv.acceptLoop()
	t.Cleanup(func() {
		srv.Close()
	})

	return srv
}

func (s *fakeNATSServer) URL() string {
	return "nats://" + s.ln.Addr().String()
}

func (s *fakeNATSServer) Close() {
	_ = s.ln.Close()
	s.CloseActive()
}

func (s *fakeNATSServer) CloseActive() {
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()

	if active != nil {
		_ = active.Close()
	}
}

func (s *fakeNATSServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}

		s.mu.Lock()
		s.active = conn
		s.mu.Unlock()

		go s.serveConn(conn)
	}
}

func (s *fakeNATSServer) serveConn(conn net.Conn) {
	defer s.clearActive(conn)

	info := struct {
		ServerID    string   `json:"server_id"`
		Version     string   `json:"version"`
		Proto       int      `json:"proto"`
		MaxPayload  int      `json:"max_payload"`
		ConnectURLs []string `json:"connect_urls,omitempty"`
	}{
		ServerID:   "straw-test",
		Version:    "1.0.0",
		Proto:      1,
		MaxPayload: s.maxPayload,
	}

	raw, err := json.Marshal(info)
	if err != nil {
		_ = conn.Close()

		return
	}

	_, err = fmt.Fprintf(conn, "INFO %s\r\n", raw)
	if err != nil {
		_ = conn.Close()

		return
	}

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PING") {
			_, err = conn.Write([]byte("PONG\r\n"))
			if err != nil {
				return
			}
		}
	}
}

func (s *fakeNATSServer) clearActive(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active == conn {
		s.active = nil
	}
}

func waitForCondition(timeout time.Duration, fn func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}

	return fmt.Errorf("condition not met within %s", timeout)
}
