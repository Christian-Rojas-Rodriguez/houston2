package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// writeErrorJSON writes the RFC §4.11 error envelope.
func writeErrorJSON(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":       code,
			"message":    message,
			"request_id": uuid.New().String(),
		},
	})
}
