// Package integration provides testcontainer-based infrastructure for integration testing.
package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// MockTargetConfig configures a mock target server response.
type MockTargetConfig struct {
	// StatusCode to return (default: 200)
	StatusCode int
	// Headers to include in response
	Headers map[string]string
	// Body to return
	Body []byte
	// Delay before responding (for latency simulation)
	Delay time.Duration
	// Error causes the handler to close the connection abruptly
	Error bool
}

// RecordedRequest stores information about a received request.
type RecordedRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	Time    time.Time
}

// MockTargetServer is a configurable HTTP server for testing.
type MockTargetServer struct {
	server *httptest.Server

	mu         sync.RWMutex
	config     MockTargetConfig
	requests   []RecordedRequest
	urlConfigs map[string]MockTargetConfig
}

// NewMockTargetServer creates and starts a new mock target server.
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

// URL returns the base URL of the mock server.
func (m *MockTargetServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server.
func (m *MockTargetServer) Close() {
	m.server.Close()
}

// SetDefaultResponse sets the default response for all requests.
func (m *MockTargetServer) SetDefaultResponse(config MockTargetConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// SetResponseForPath sets a specific response for a URL path.
func (m *MockTargetServer) SetResponseForPath(path string, config MockTargetConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.urlConfigs[path] = config
}

// ClearResponses resets all configured responses to defaults.
func (m *MockTargetServer) ClearResponses() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	}
	m.urlConfigs = make(map[string]MockTargetConfig)
}

// GetRequests returns all recorded requests.
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
	// Record the request
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

	// Get configuration for this path
	m.mu.RLock()
	config, ok := m.urlConfigs[r.URL.Path]
	if !ok {
		config = m.config
	}
	m.mu.RUnlock()

	// Simulate error (abrupt connection close)
	if config.Error {
		// Hijack and close connection
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				conn.Close()
				return
			}
		}
		// Fallback: just return 500
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Apply delay
	if config.Delay > 0 {
		time.Sleep(config.Delay)
	}

	// Set headers
	for k, v := range config.Headers {
		w.Header().Set(k, v)
	}

	// Set status code
	statusCode := config.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)

	// Write body
	if len(config.Body) > 0 {
		_, _ = w.Write(config.Body)
	}
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()

	// Limit body size for safety
	const maxBodySize = 1024 * 1024 // 1MB
	limited := http.MaxBytesReader(nil, r.Body, maxBodySize)

	buf := make([]byte, 0, 1024)
	for {
		tmp := make([]byte, 1024)
		n, err := limited.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

// JSONResponse is a helper to create a JSON response body.
func JSONResponse(data interface{}) []byte {
	b, _ := json.Marshal(data)
	return b
}

// MockTargetWithJSON creates a mock target that returns JSON.
func MockTargetWithJSON(data interface{}) MockTargetConfig {
	return MockTargetConfig{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: JSONResponse(data),
	}
}

// MockTargetWithStatus creates a mock target with specific status code.
func MockTargetWithStatus(status int, body string) MockTargetConfig {
	return MockTargetConfig{
		StatusCode: status,
		Body:       []byte(body),
	}
}

// MockTargetWithDelay creates a mock target with simulated latency.
func MockTargetWithDelay(delay time.Duration) MockTargetConfig {
	return MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
		Delay:      delay,
	}
}
