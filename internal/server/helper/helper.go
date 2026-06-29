// Package helper provides HTTP request and response helper functions.
package helper

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ReadJSON decodes the JSON body of the request into v.
func ReadJSON(r *http.Request, v any) error {
	err := json.NewDecoder(r.Body).Decode(v)
	if err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}

	return nil
}

// WriteJSON encodes v as JSON and writes it to the response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		return
	}
}

// WriteError writes a JSON error response with the given status and message.
func WriteError(w http.ResponseWriter, status int, errMsg string) {
	WriteJSON(w, status, map[string]string{"error": errMsg})
}
