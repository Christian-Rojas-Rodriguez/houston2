package server

import (
	"encoding/json"
	"net/http"
)

// stubHandler returns a placeholder response for routes not yet implemented.
// Tasks 0007–0009 will replace each stub with a real handler.
func stubHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "stub": true})
}
