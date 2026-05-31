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

// CredentialStore abstracts all I/O for the credential CRUD handlers.
// The concrete implementation handles encryption; handlers receive plaintext.
type CredentialStore interface {
	// UpsertCredential inserts or updates the org's Anthropic key.
	UpsertCredential(ctx context.Context, jwt string, orgID uuid.UUID, plaintextKey string) error

	// DeleteCredential removes the org's key row.
	DeleteCredential(ctx context.Context, jwt string, orgID uuid.UUID) error

	// HasCredential reports whether the org has a key configured, without
	// exposing the key value.
	HasCredential(ctx context.Context, jwt string, orgID uuid.UUID) (bool, error)
}

type credentialStatusResponse struct {
	OrgID  string `json:"org_id"`
	HasKey bool   `json:"has_key"`
}

// resolveCredentialOrgID extracts and validates the {id} path parameter and
// verifies it matches the authenticated tenant's org ID. Returns (orgID, true)
// on success; writes an error response and returns (uuid.Nil, false) on failure.
func resolveCredentialOrgID(w http.ResponseWriter, r *http.Request, tc middleware.TenantContext) (uuid.UUID, bool) {
	rawID := r.PathValue("id")
	orgID, err := uuid.Parse(rawID)
	if err != nil {
		writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "invalid org ID in path")
		return uuid.UUID{}, false
	}
	if orgID != tc.OrgID {
		writeHandlerError(w, r, http.StatusForbidden, "forbidden", "org ID in path does not match authenticated org")
		return uuid.UUID{}, false
	}
	return orgID, true
}

// validateAnthropicKey returns true when key has the required sk-ant- prefix.
func validateAnthropicKey(key string) bool {
	return strings.HasPrefix(key, "sk-ant-")
}

// decodeKeyRequest decodes `{"key": "..."}` from r.Body. Writes an error
// response and returns ("", false) on failure.
func decodeKeyRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return "", false
	}
	if !validateAnthropicKey(req.Key) {
		writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "key must start with sk-ant-")
		return "", false
	}
	return req.Key, true
}

// RegisterCredential handles POST /v1/orgs/{id}/credentials.
// Requires RoleOwner in the middleware chain.
func RegisterCredential(store CredentialStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tc, ok := middleware.TenantFromContext(r.Context())
		if !ok {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "tenant context missing")
			return
		}
		orgID, ok := resolveCredentialOrgID(w, r, tc)
		if !ok {
			return
		}
		key, ok := decodeKeyRequest(w, r)
		if !ok {
			return
		}
		jwt, _ := auth.JWTFromContext(r.Context())
		if err := store.UpsertCredential(r.Context(), jwt, orgID, key); err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to register credential")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(credentialStatusResponse{OrgID: orgID.String(), HasKey: true})
	}
}

// RotateCredential handles PUT /v1/orgs/{id}/credentials.
// Requires RoleOwner in the middleware chain.
func RotateCredential(store CredentialStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tc, ok := middleware.TenantFromContext(r.Context())
		if !ok {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "tenant context missing")
			return
		}
		orgID, ok := resolveCredentialOrgID(w, r, tc)
		if !ok {
			return
		}
		key, ok := decodeKeyRequest(w, r)
		if !ok {
			return
		}
		jwt, _ := auth.JWTFromContext(r.Context())
		if err := store.UpsertCredential(r.Context(), jwt, orgID, key); err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to rotate credential")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(credentialStatusResponse{OrgID: orgID.String(), HasKey: true})
	}
}

// RevokeCredential handles DELETE /v1/orgs/{id}/credentials.
// Requires RoleOwner in the middleware chain.
func RevokeCredential(store CredentialStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tc, ok := middleware.TenantFromContext(r.Context())
		if !ok {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "tenant context missing")
			return
		}
		orgID, ok := resolveCredentialOrgID(w, r, tc)
		if !ok {
			return
		}
		jwt, _ := auth.JWTFromContext(r.Context())
		if err := store.DeleteCredential(r.Context(), jwt, orgID); err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to revoke credential")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetCredentialStatus handles GET /v1/orgs/{id}/credentials.
// Returns {"org_id": "...", "has_key": bool} — never the plaintext key.
// Requires RoleOwner in the middleware chain.
func GetCredentialStatus(store CredentialStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tc, ok := middleware.TenantFromContext(r.Context())
		if !ok {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "tenant context missing")
			return
		}
		orgID, ok := resolveCredentialOrgID(w, r, tc)
		if !ok {
			return
		}
		jwt, _ := auth.JWTFromContext(r.Context())
		hasKey, err := store.HasCredential(r.Context(), jwt, orgID)
		if err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to check credential status")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(credentialStatusResponse{OrgID: orgID.String(), HasKey: hasKey})
	}
}
