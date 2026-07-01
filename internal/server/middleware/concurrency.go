// Package middleware provides HTTP middleware for the control server.
package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// ConcurrencyLimiter limits the number of concurrent requests, rejecting excess immediately.
func ConcurrencyLimiter(maxConcurrent int) func(http.Handler) http.Handler {
	if maxConcurrent <= 0 {
		maxConcurrent = 50
	}

	semaphore := make(chan struct{}, maxConcurrent)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()

				next.ServeHTTP(w, r)
			default:
				writeError(w, http.StatusServiceUnavailable, "server at capacity, please retry")
			}
		})
	}
}

// BodyLimit limits request body size.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes > 0 && r.ContentLength > maxBytes {
				writeError(w, http.StatusRequestEntityTooLarge, "request body too large")

				return
			}

			if maxBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequestID copies or creates an X-Request-ID response header.
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = "req_" + strconv.FormatInt(time.Now().UnixNano(), 10)
				r.Header.Set("X-Request-ID", requestID)
			}

			w.Header().Set("X-Request-ID", requestID)
			next.ServeHTTP(w, r)
		})
	}
}

// LoggerMiddleware logs each request after handling.
func LoggerMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			slog.InfoContext(r.Context(), "request handled", "method", r.Method, "path", r.URL.Path)
		})
	}
}

// CORS sets basic CORS headers.
func CORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Recover converts panics into JSON 500 responses.
func Recover() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			defer func() {
				if v := recover(); v != nil {
					slog.ErrorContext(ctx, "panic recovered", "panic", v)
					writeError(w, http.StatusInternalServerError, "internal server error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(struct {
		Error map[string]string `json:"error"`
	}{
		Error: map[string]string{"message": message},
	})
	if err != nil {
		slog.Error("failed to encode error response", "error", err)
	}
}
