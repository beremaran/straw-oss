package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/beremaran/straw/internal/server/helper"
)

func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				bytes := make([]byte, 16)
				_, err := rand.Read(bytes)
				if err == nil {
					reqID = hex.EncodeToString(bytes)
				} else {
					reqID = fmt.Sprintf("req_%d", r.Context().Done())
				}
			}
			r.Header.Set("X-Request-ID", reqID)
			w.Header().Set("X-Request-ID", reqID)
			next.ServeHTTP(w, r)
		})
	}
}

func Recover() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			defer func(ctx context.Context) {
				if err := recover(); err != nil {
					slog.ErrorContext(ctx, "panic recovered",
						"error", err,
						"stack", string(debug.Stack()),
					)
					helper.WriteError(w, http.StatusInternalServerError, "Internal Server Error")
				}
			}(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

func CORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Session-ID, X-Session-End")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes > 0 {
				if r.ContentLength > maxBytes {
					helper.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")

					return
				}
				if r.Body != nil {
					r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
