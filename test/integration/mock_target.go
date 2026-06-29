// Package integration provides test helpers for integration tests.
package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// MockTargetConfig holds the response configuration for a mock target server.
type MockTargetConfig struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	Delay      time.Duration
	Error      bool
}

// RecordedRequest captures an incoming HTTP request.
type RecordedRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	Time    time.Time
}

// MockTargetServer is a test double that records requests and returns configurable responses.
type MockTargetServer struct {
	server     *httptest.Server
	mu         sync.RWMutex
	config     MockTargetConfig
	requests   []RecordedRequest
	urlConfigs map[string]MockTargetConfig
}

// NewMockTargetServer creates a new mock target HTTP server.
func NewMockTargetServer() *MockTargetServer {
	m := &MockTargetServer{
		config: MockTargetConfig{
			StatusCode: http.StatusOK,
			Body:       []byte("OK"),
		},
		urlConfigs: make(map[string]MockTargetConfig),
	}

	m.server = httptest.NewServer(http.HandlerFunc(m.handler))

	return m
}

// URL returns the server URL.
func (m *MockTargetServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server.
func (m *MockTargetServer) Close() {
	m.server.Close()
}

// SetDefaultResponse sets the response returned for all unmatched requests.
func (m *MockTargetServer) SetDefaultResponse(config MockTargetConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = config
}

// SetResponseForPath configures a custom response for requests to the given path.
func (m *MockTargetServer) SetResponseForPath(path string, config MockTargetConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.urlConfigs[path] = config
}

// ClearResponses resets the default response and removes all path-specific configurations.
func (m *MockTargetServer) ClearResponses() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	}
	m.urlConfigs = make(map[string]MockTargetConfig)
}

// GetRequests returns a copy of all recorded requests.
func (m *MockTargetServer) GetRequests() []RecordedRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]RecordedRequest, len(m.requests))
	copy(result, m.requests)

	return result
}

// ClearRequests clears all recorded requests.
func (m *MockTargetServer) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests = nil
}

// RequestCount returns the number of recorded requests.
func (m *MockTargetServer) RequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.requests)
}

func (m *MockTargetServer) handler(w http.ResponseWriter, r *http.Request) {
	body := make([]byte, 0)
	if r.Body != nil {
		body, _ = readBody(r)
	}

	m.mu.Lock()
	m.requests = append(m.requests, RecordedRequest{
		Method:  r.Method,
		URL:     r.URL.String(),
		Headers: r.Header.Clone(),
		Body:    body,
		Time:    time.Now(),
	})
	m.mu.Unlock()

	m.mu.RLock()

	config, ok := m.urlConfigs[r.URL.Path]
	if !ok {
		config = m.config
	}

	m.mu.RUnlock()

	if config.Error {
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()

				return
			}
		}

		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	if config.Delay > 0 {
		time.Sleep(config.Delay)
	}

	for k, v := range config.Headers {
		w.Header().Set(k, v)
	}

	statusCode := config.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	w.WriteHeader(statusCode)

	if len(config.Body) > 0 {
		_, _ = w.Write(config.Body)
	}
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer func() { _ = r.Body.Close() }()

	const maxBodySize = 1024 * 1024

	const readBufSize = 1024

	limited := http.MaxBytesReader(nil, r.Body, maxBodySize)

	buf := make([]byte, 0, readBufSize)
	for {
		tmp := make([]byte, readBufSize)
		n, err := limited.Read(tmp)
		buf = append(buf, tmp[:n]...)

		if err != nil {
			break
		}
	}

	return buf, nil
}

// JSONResponse marshals data to JSON bytes, panicking on error.
func JSONResponse(data any) []byte {
	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	return b
}

// MockTargetWithJSON returns a config that responds with JSON data.
func MockTargetWithJSON(data any) MockTargetConfig {
	return MockTargetConfig{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": httpContentTypeJSON,
		},
		Body: JSONResponse(data),
	}
}

// MockTargetWithStatus returns a config that responds with the given status code and body.
func MockTargetWithStatus(status int, body string) MockTargetConfig {
	return MockTargetConfig{
		StatusCode: status,
		Body:       []byte(body),
	}
}

// MockTargetWithDelay returns a config that adds a delay before responding.
func MockTargetWithDelay(delay time.Duration) MockTargetConfig {
	return MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
		Delay:      delay,
	}
}
