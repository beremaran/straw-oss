package middleware

import (
	"fmt"
	"net/http"
)

// StatusResponseWriter wraps an http.ResponseWriter to capture the status code.
type StatusResponseWriter struct {
	http.ResponseWriter
	Status int
}

// NewStatusResponseWriter creates a new StatusResponseWriter wrapping the given writer.
func NewStatusResponseWriter(w http.ResponseWriter) *StatusResponseWriter {
	return &StatusResponseWriter{ResponseWriter: w, Status: http.StatusOK}
}

// WriteHeader captures the status code before writing to the underlying response writer.
func (w *StatusResponseWriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *StatusResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	if err != nil {
		return n, fmt.Errorf("write: %w", err)
	}

	return n, nil
}
