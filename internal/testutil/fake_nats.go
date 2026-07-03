package testutil

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
)

const (
	fakeNATSFieldsSUB   = 3
	fakeNATSFieldsUNSUB = 2
	fakeNATSFieldsPUB   = 3
	fakeNATSMaxPayload  = 2
)

// FakeNATSServer is a tiny Core NATS broker for tests that need real request/
// reply and subscription routing without an external process.
type FakeNATSServer struct {
	ln         net.Listener
	maxPayload int

	mu        sync.Mutex
	clients   map[*fakeNATSClient]struct{}
	bySubject map[string]map[*fakeNATSSubscription]struct{}
}

type fakeNATSClient struct {
	server *FakeNATSServer
	conn   net.Conn

	wmu  sync.Mutex
	mu   sync.Mutex
	subs map[string]*fakeNATSSubscription
}

type fakeNATSSubscription struct {
	client  *fakeNATSClient
	subject string
	queue   string
	sid     string
}

// ErrInvalidSUB, ErrInvalidUNSUB, ErrInvalidPUB, ErrInvalidPUBSize are sentinel errors for malformed NATS protocol lines.
var (
	ErrInvalidSUB   = errors.New("invalid SUB line")
	ErrInvalidUNSUB = errors.New("invalid UNSUB line")
	ErrInvalidPUB   = errors.New("invalid PUB line")
	ErrInvalidPUBSz = errors.New("invalid PUB size")
)

// NewFakeNATSServer starts a broker on 127.0.0.1 and registers cleanup with t.
func NewFakeNATSServer(t testing.TB, maxPayload int) *FakeNATSServer {
	t.Helper()

	lc := net.ListenConfig{}

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake NATS: %v", err)
	}

	srv := &FakeNATSServer{
		ln:         ln,
		maxPayload: maxPayload,
		clients:    make(map[*fakeNATSClient]struct{}),
		bySubject:  make(map[string]map[*fakeNATSSubscription]struct{}),
	}
	go srv.acceptLoop()

	t.Cleanup(srv.Close)

	return srv
}

// URL returns the broker URL for NATS clients.
func (s *FakeNATSServer) URL() string {
	return "nats://" + s.ln.Addr().String()
}

// Close shuts down the listener and all active clients.
func (s *FakeNATSServer) Close() {
	_ = s.ln.Close()

	s.mu.Lock()

	clients := make([]*fakeNATSClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()

	for _, c := range clients {
		_ = c.conn.Close()
	}
}

func (s *FakeNATSServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}

		client := &fakeNATSClient{
			server: s,
			conn:   conn,
			subs:   make(map[string]*fakeNATSSubscription),
		}
		s.mu.Lock()
		s.clients[client] = struct{}{}
		s.mu.Unlock()

		go client.serve()
	}
}

func (c *fakeNATSClient) serve() {
	defer c.close()

	err := c.writeInfo()
	if err != nil {
		return
	}

	reader := bufio.NewReader(c.conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		err = c.handleLine(line, reader)
		if err != nil {
			return
		}
	}
}

func (c *fakeNATSClient) handleLine(line string, reader *bufio.Reader) error {
	switch {
	case strings.HasPrefix(line, "PING"):
		return c.writeString("PONG\r\n")
	case strings.HasPrefix(line, "PONG"):
		return nil
	case strings.HasPrefix(line, "CONNECT"):
		return nil
	case strings.HasPrefix(line, "SUB "):
		return c.handleSub(line)
	case strings.HasPrefix(line, "UNSUB "):
		return c.handleUnsub(line)
	case strings.HasPrefix(line, "PUB "):
		return c.handlePub(line, reader)
	default:
		return nil
	}
}

func (c *fakeNATSClient) writeInfo() error {
	info := fmt.Sprintf(
		`INFO {"server_id":"straw-test","version":"1.0.0","proto":1,"max_payload":%d}`+"\r\n",
		c.server.maxPayload,
	)

	return c.writeString(info)
}

func (c *fakeNATSClient) handleSub(line string) error {
	fields := strings.Fields(line)
	if len(fields) != fakeNATSFieldsSUB && len(fields) != fakeNATSFieldsSUB+1 {
		return fmt.Errorf("%w: %q", ErrInvalidSUB, line)
	}

	subject := fields[1]
	queue := ""

	sid := fields[len(fields)-1]
	if len(fields) == fakeNATSFieldsSUB+1 {
		queue = fields[2]
	}

	sub := &fakeNATSSubscription{client: c, subject: subject, queue: queue, sid: sid}
	c.mu.Lock()
	c.subs[sid] = sub
	c.mu.Unlock()

	c.server.mu.Lock()
	defer c.server.mu.Unlock()

	if c.server.bySubject[subject] == nil {
		c.server.bySubject[subject] = make(map[*fakeNATSSubscription]struct{})
	}

	c.server.bySubject[subject][sub] = struct{}{}

	return nil
}

func (c *fakeNATSClient) handleUnsub(line string) error {
	fields := strings.Fields(line)
	if len(fields) != fakeNATSFieldsUNSUB && len(fields) != fakeNATSFieldsUNSUB+1 {
		return fmt.Errorf("%w: %q", ErrInvalidUNSUB, line)
	}

	sid := fields[1]

	c.mu.Lock()
	sub := c.subs[sid]
	delete(c.subs, sid)
	c.mu.Unlock()

	if sub == nil {
		return nil
	}

	c.server.mu.Lock()
	defer c.server.mu.Unlock()

	if subs := c.server.bySubject[sub.subject]; subs != nil {
		delete(subs, sub)

		if len(subs) == 0 {
			delete(c.server.bySubject, sub.subject)
		}
	}

	return nil
}

func (c *fakeNATSClient) handlePub(line string, reader *bufio.Reader) error {
	fields := strings.Fields(line)
	if len(fields) != fakeNATSFieldsPUB && len(fields) != fakeNATSFieldsPUB+1 {
		return fmt.Errorf("%w: %q", ErrInvalidPUB, line)
	}

	subject := fields[1]
	replyTo := ""

	sizeField := fields[len(fields)-1]
	if len(fields) == fakeNATSFieldsPUB+1 {
		replyTo = fields[2]
	}

	var size int

	_, err := fmt.Sscanf(sizeField, "%d", &size)
	if err != nil || size < 0 {
		return fmt.Errorf("%w: %q", ErrInvalidPUBSz, line)
	}

	payload := make([]byte, size)

	_, err = io.ReadFull(reader, payload)
	if err != nil {
		return fmt.Errorf("read payload: %w", err)
	}

	trailer := make([]byte, fakeNATSMaxPayload)

	_, err = io.ReadFull(reader, trailer)
	if err != nil {
		return fmt.Errorf("read trailer: %w", err)
	}

	c.server.publish(subject, replyTo, payload)

	return nil
}

func (s *FakeNATSServer) publish(subject, replyTo string, payload []byte) {
	s.mu.Lock()
	direct := make([]*fakeNATSSubscription, 0)
	queueTargets := make(map[string]*fakeNATSSubscription)

	for pattern, subs := range s.bySubject {
		if !subjectMatches(pattern, subject) {
			continue
		}

		for sub := range subs {
			if sub.queue == "" {
				direct = append(direct, sub)

				continue
			}

			key := pattern + "\x00" + sub.queue
			if _, ok := queueTargets[key]; !ok {
				queueTargets[key] = sub
			}
		}
	}

	targets := direct
	for _, sub := range queueTargets {
		targets = append(targets, sub)
	}
	s.mu.Unlock()

	for _, sub := range targets {
		err := sub.client.writeMsg(subject, sub.sid, replyTo, payload)
		if err != nil {
			_ = sub.client.conn.Close()
		}
	}
}

func subjectMatches(pattern, subject string) bool {
	if pattern == subject {
		return true
	}

	pp := strings.Split(pattern, ".")
	sp := strings.Split(subject, ".")

	for i := range pp {
		if pp[i] == ">" {
			return i == len(pp)-1
		}

		if i >= len(sp) {
			return false
		}

		if pp[i] != "*" && pp[i] != sp[i] {
			return false
		}
	}

	return len(pp) == len(sp)
}

func (c *fakeNATSClient) writeMsg(subject, sid, replyTo string, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	var err error

	if replyTo == "" {
		_, err = fmt.Fprintf(c.conn, "MSG %s %s %d\r\n", subject, sid, len(payload))
		if err != nil {
			return fmt.Errorf("write msg: %w", err)
		}
	} else {
		_, err = fmt.Fprintf(c.conn, "MSG %s %s %s %d\r\n", subject, sid, replyTo, len(payload))
		if err != nil {
			return fmt.Errorf("write msg: %w", err)
		}
	}

	_, err = c.conn.Write(payload)
	if err != nil {
		return fmt.Errorf("write payload: %w", err)
	}

	_, err = c.conn.Write([]byte("\r\n"))
	if err != nil {
		return fmt.Errorf("write trailing newline: %w", err)
	}

	return nil
}

func (c *fakeNATSClient) writeString(s string) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	_, err := io.WriteString(c.conn, s)
	if err != nil {
		return fmt.Errorf("write string: %w", err)
	}

	return nil
}

func (c *fakeNATSClient) close() {
	c.server.mu.Lock()
	delete(c.server.clients, c)

	subs := make([]*fakeNATSSubscription, 0, len(c.subs))
	for _, sub := range c.subs {
		subs = append(subs, sub)
	}
	c.server.mu.Unlock()

	for _, sub := range subs {
		c.server.mu.Lock()
		if subjectSubs := c.server.bySubject[sub.subject]; subjectSubs != nil {
			delete(subjectSubs, sub)

			if len(subjectSubs) == 0 {
				delete(c.server.bySubject, sub.subject)
			}
		}
		c.server.mu.Unlock()
	}

	_ = c.conn.Close()
}
