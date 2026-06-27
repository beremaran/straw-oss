package middleware

import "net/http"

type StatusResponseWriter struct {
	http.ResponseWriter
	Status int
}

func NewStatusResponseWriter(w http.ResponseWriter) *StatusResponseWriter {
	return &StatusResponseWriter{ResponseWriter: w, Status: http.StatusOK}
}

func (w *StatusResponseWriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *StatusResponseWriter) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}
