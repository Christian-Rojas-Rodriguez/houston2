package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/middleware"
)

// CredentialStore is the port for all credential storage operations.
// Defined in the consumer package (internal/handlers) following the hexagonal
// pattern established by AgentStore.
type CredentialStore interface {
	// UpsertCredential persists the plaintext key for the given org (insert or update).
	// Encryption is the store's responsibility — the handler passes plaintext.
	UpsertCredential(ctx context.Context, jwt string, orgID uuid.UUID, plaintextKey string) error

	// DeleteCredential removes the org_credentials row for the given org.
	DeleteCredential(ctx context.Context, jwt string, orgID uuid.UUID) error

	// HasCredential reports whether the org has a key configured, without exposing it.
	HasCredential(ctx context.Context, jwt string, orgID uuid.UUID) (bool, error)
}

// credentialRequest is the JSON body for POST and PUT.
type credentialRequest struct {
	Key string `json:"key"`
}

// credentialResponse is the success body for POST and PUT (and GET).
type credentialResponse struct {
	OrgID  string `json:"org_id"`
	HasKey bool   `json:"has_key"`
}

// validateCredentialWrite is shared validation logic for POST and PUT.
// It returns (orgID, jwt, ok). When ok is false the handler has already
// written the error response and the caller must return immediately.
func validateCredentialWrite(w http.ResponseWriter, r *http.Request, store CredentialStore) (orgID uuid.UUID, jwtStr string, key string, ok bool) {
	// Step 1: TenantContext must be present.
	tc, tenantOK := middleware.TenantFromContext(r.Context())
	if !tenantOK {
		writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "tenant context missing")
		return uuid.Nil, "", "", false
	}

	// Step 2: Parse {id} path param.
	rawID := r.PathValue("id")
	parsedID, err := uuid.Parse(rawID)
	if err != nil {
		writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "invalid org id in path")
		return uuid.Nil, "", "", false
	}

	// Step 3: {id} must match tc.OrgID (defense in depth).
	if parsedID != tc.OrgID {
		writeHandlerError(w, r, http.StatusForbidden, "forbidden", "path org id does not match authenticated org")
		return uuid.Nil, "", "", false
	}

	// Step 4: Decode JSON body.
	var req credentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return uuid.Nil, "", "", false
	}

	// Step 5: key must be present and non-empty.
	if req.Key == "" {
		writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "key is required")
		return uuid.Nil, "", "", false
	}

	// Step 6: key must have the sk-ant- prefix.
	if !strings.HasPrefix(req.Key, "sk-ant-") {
		writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "key must start with sk-ant-")
		return uuid.Nil, "", "", false
	}

	jwtStr, _ = auth.JWTFromContext(r.Context())
	return parsedID, jwtStr, req.Key, true
}

// validateCredentialRead validates path + tenant context for DELETE and GET.
// Returns (orgID, jwt, ok). When ok is false the error response has already been written.
func validateCredentialRead(w http.ResponseWriter, r *http.Request) (orgID uuid.UUID, jwtStr string, ok bool) {
	// Step 1: TenantContext must be present.
	tc, tenantOK := middleware.TenantFromContext(r.Context())
	if !tenantOK {
		writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "tenant context missing")
		return uuid.Nil, "", false
	}

	// Step 2: Parse {id} path param.
	rawID := r.PathValue("id")
	parsedID, err := uuid.Parse(rawID)
	if err != nil {
		writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "invalid org id in path")
		return uuid.Nil, "", false
	}

	// Step 3: {id} must match tc.OrgID.
	if parsedID != tc.OrgID {
		writeHandlerError(w, r, http.StatusForbidden, "forbidden", "path org id does not match authenticated org")
		return uuid.Nil, "", false
	}

	jwtStr, _ = auth.JWTFromContext(r.Context())
	return parsedID, jwtStr, true
}

// RegisterCredential returns an http.HandlerFunc for POST /v1/orgs/{id}/credentials.
// On success: 201 Created, {"org_id":"<uuid>","has_key":true}.
func RegisterCredential(store CredentialStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, jwtStr, key, ok := validateCredentialWrite(w, r, store)
		if !ok {
			return
		}

		// Step 7: Persist via store.
		if err := store.UpsertCredential(r.Context(), jwtStr, orgID, key); err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to store credential")
			return
		}

		// Step 8: 201 Created.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(credentialResponse{
			OrgID:  orgID.String(),
			HasKey: true,
		})
	}
}

// RotateCredential returns an http.HandlerFunc for PUT /v1/orgs/{id}/credentials.
// On success: 200 OK, {"org_id":"<uuid>","has_key":true}.
func RotateCredential(store CredentialStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, jwtStr, key, ok := validateCredentialWrite(w, r, store)
		if !ok {
			return
		}

		if err := store.UpsertCredential(r.Context(), jwtStr, orgID, key); err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to rotate credential")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(credentialResponse{
			OrgID:  orgID.String(),
			HasKey: true,
		})
	}
}

// RevokeCredential returns an http.HandlerFunc for DELETE /v1/orgs/{id}/credentials.
// On success: 204 No Content, empty body.
func RevokeCredential(store CredentialStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, jwtStr, ok := validateCredentialRead(w, r)
		if !ok {
			return
		}

		if err := store.DeleteCredential(r.Context(), jwtStr, orgID); err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to revoke credential")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// GetCredentialStatus returns an http.HandlerFunc for GET /v1/orgs/{id}/credentials.
// On success: 200 OK, {"org_id":"<uuid>","has_key":<bool>}.
// The plaintext key is never returned.
func GetCredentialStatus(store CredentialStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, jwtStr, ok := validateCredentialRead(w, r)
		if !ok {
			return
		}

		hasKey, err := store.HasCredential(r.Context(), jwtStr, orgID)
		if err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to check credential status")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(credentialResponse{
			OrgID:  orgID.String(),
			HasKey: hasKey,
		})
	}
}
