// Package handlers_test is the failing-first test suite for Task 0009 (provider-credentials).
//
// TDD red phase: these tests reference handlers.CredentialStore, handlers.RegisterCredential,
// handlers.RotateCredential, handlers.RevokeCredential, and handlers.GetCredentialStatus —
// none of which exist until internal/handlers/credentials.go is implemented.
//
// Run with:
//   go test ./tests/unit/handlers/... -v
//
// No network calls, no Supabase credentials, no environment variables required.
//
// AC mapping
// ----------
// AC1 — TestCredentials_Register_ValidKey_Returns201
// AC2 — TestCredentials_Register_InvalidKeyFormat_Returns400
// AC3 — TestCredentials_Rotate_ValidKey_Returns200
// AC4 — TestCredentials_Rotate_InvalidKeyFormat_Returns400
// AC5 — TestCredentials_Revoke_Returns204
// AC6 — TestCredentials_Status_KeyExists_Returns200True
// AC7 — TestCredentials_Status_NoKey_Returns200False
// AC8 — TestCredentials_OrgIDMismatch_Returns403
// AC9 — TestCredentials_Register_ResponseNeverContainsKey

package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/handlers"
	"github.com/nomenclator/houston2/internal/middleware"
)

// ---------------------------------------------------------------------------
// Stub CredentialStore
// ---------------------------------------------------------------------------

type stubCredentialStore struct {
	upsertErr   error
	deleteErr   error
	hasKey      bool
	hasErr      error
	capturedKey string
	upsertCalls int
	deleteCalls int
}

func (s *stubCredentialStore) UpsertCredential(_ context.Context, _ string, _ uuid.UUID, key string) error {
	s.upsertCalls++
	s.capturedKey = key
	return s.upsertErr
}

func (s *stubCredentialStore) DeleteCredential(_ context.Context, _ string, _ uuid.UUID) error {
	s.deleteCalls++
	return s.deleteErr
}

func (s *stubCredentialStore) HasCredential(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	return s.hasKey, s.hasErr
}

// ---------------------------------------------------------------------------
// Stub TenantQuerier
// ---------------------------------------------------------------------------

type stubCredQuerier struct {
	orgID   uuid.UUID
	groupID uuid.UUID
	role    middleware.Role
	err     error
}

func (s *stubCredQuerier) CurrentTenant(_ context.Context, _ uuid.UUID) (middleware.TenantContext, error) {
	if s.err != nil {
		return middleware.TenantContext{}, s.err
	}
	return middleware.TenantContext{
		OrgID:   s.orgID,
		GroupID: s.groupID,
		Role:    s.role,
	}, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const credTestJWTSecret = "test-secret-credentials-0009"

func makeCredJWT(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"exp": jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		"iat": jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(credTestJWTSecret))
	if err != nil {
		t.Fatalf("makeCredJWT: %v", err)
	}
	return signed
}

// credMux builds an http.ServeMux with all four credential endpoints wired
// through the full auth → tenant → role middleware chain. Routing via a mux
// is required so that r.PathValue("id") works inside the handlers.
func credMux(t *testing.T, querier middleware.TenantQuerier, store handlers.CredentialStore) http.Handler {
	t.Helper()
	authCfg := auth.Config{
		SupabaseURL: "https://fake.supabase.co",
		AnonKey:     "test-anon-key",
		JWTSecret:   credTestJWTSecret,
		Port:        "0",
	}
	wrap := func(h http.Handler) http.Handler {
		return auth.AuthMiddleware(authCfg)(
			middleware.TenantMiddleware(querier)(
				middleware.RequireRole(middleware.RoleOwner)(h),
			),
		)
	}
	mux := http.NewServeMux()
	mux.Handle("POST /v1/orgs/{id}/credentials", wrap(handlers.RegisterCredential(store)))
	mux.Handle("PUT /v1/orgs/{id}/credentials", wrap(handlers.RotateCredential(store)))
	mux.Handle("DELETE /v1/orgs/{id}/credentials", wrap(handlers.RevokeCredential(store)))
	mux.Handle("GET /v1/orgs/{id}/credentials", wrap(handlers.GetCredentialStatus(store)))
	return mux
}

// doCredRequest fires a request against the given handler. body may be nil.
func doCredRequest(t *testing.T, h http.Handler, method, path, token string, body any) *http.Response {
	t.Helper()
	var r *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("doCredRequest: marshal body: %v", err)
		}
		r = bytes.NewReader(raw)
	} else {
		r = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func assertCredStatus(t *testing.T, res *http.Response, wantStatus int, wantHasKey bool) {
	t.Helper()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != wantStatus {
		t.Errorf("expected HTTP %d, got %d — body: %s", wantStatus, res.StatusCode, body)
		return
	}
	var m struct {
		OrgID  string `json:"org_id"`
		HasKey bool   `json:"has_key"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("response is not valid JSON: %v — body: %s", err, body)
	}
	if m.HasKey != wantHasKey {
		t.Errorf("has_key = %v, want %v — body: %s", m.HasKey, wantHasKey, body)
	}
	if m.OrgID == "" {
		t.Error("org_id must not be empty in response")
	}
}

func assertCredError(t *testing.T, res *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != wantStatus {
		t.Errorf("expected HTTP %d, got %d — body: %s", wantStatus, res.StatusCode, body)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("response is not valid JSON: %v — body: %s", err, body)
	}
	if env.Error.Code != wantCode {
		t.Errorf("error.code = %q, want %q — body: %s", env.Error.Code, wantCode, body)
	}
	if env.Error.Message == "" {
		t.Error("error.message must not be empty")
	}
}

// ---------------------------------------------------------------------------
// AC1 — POST with valid sk-ant-* key → 201, has_key: true, store called once
// ---------------------------------------------------------------------------

func TestCredentials_Register_ValidKey_Returns201(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &stubCredentialStore{}
	querier := &stubCredQuerier{orgID: orgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	tok := makeCredJWT(t, uuid.New())
	res := doCredRequest(t, h, http.MethodPost, "/v1/orgs/"+orgID.String()+"/credentials", tok,
		map[string]any{"key": "sk-ant-api03-test-key"})
	defer res.Body.Close()

	assertCredStatus(t, res, http.StatusCreated, true)

	if store.upsertCalls != 1 {
		t.Errorf("AC1: UpsertCredential called %d times, want 1", store.upsertCalls)
	}
	if store.capturedKey != "sk-ant-api03-test-key" {
		t.Errorf("AC1: captured key = %q, want %q", store.capturedKey, "sk-ant-api03-test-key")
	}
}

// ---------------------------------------------------------------------------
// AC2 — POST with invalid key format → 400 invalid_request, store not called
// ---------------------------------------------------------------------------

func TestCredentials_Register_InvalidKeyFormat_Returns400(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &stubCredentialStore{}
	querier := &stubCredQuerier{orgID: orgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	tok := makeCredJWT(t, uuid.New())
	res := doCredRequest(t, h, http.MethodPost, "/v1/orgs/"+orgID.String()+"/credentials", tok,
		map[string]any{"key": "invalid-key-format"})
	defer res.Body.Close()

	assertCredError(t, res, http.StatusBadRequest, "invalid_request")
	if store.upsertCalls != 0 {
		t.Errorf("AC2: UpsertCredential must not be called on validation failure; got %d calls", store.upsertCalls)
	}
}

// ---------------------------------------------------------------------------
// AC3 — PUT with valid key → 200 OK, has_key: true
// ---------------------------------------------------------------------------

func TestCredentials_Rotate_ValidKey_Returns200(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &stubCredentialStore{}
	querier := &stubCredQuerier{orgID: orgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	tok := makeCredJWT(t, uuid.New())
	res := doCredRequest(t, h, http.MethodPut, "/v1/orgs/"+orgID.String()+"/credentials", tok,
		map[string]any{"key": "sk-ant-api03-rotated-key"})
	defer res.Body.Close()

	assertCredStatus(t, res, http.StatusOK, true)
	if store.upsertCalls != 1 {
		t.Errorf("AC3: UpsertCredential called %d times, want 1", store.upsertCalls)
	}
}

// ---------------------------------------------------------------------------
// AC4 — PUT with invalid key → 400 invalid_request
// ---------------------------------------------------------------------------

func TestCredentials_Rotate_InvalidKeyFormat_Returns400(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &stubCredentialStore{}
	querier := &stubCredQuerier{orgID: orgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	tok := makeCredJWT(t, uuid.New())
	res := doCredRequest(t, h, http.MethodPut, "/v1/orgs/"+orgID.String()+"/credentials", tok,
		map[string]any{"key": "not-sk-ant-prefix"})
	defer res.Body.Close()

	assertCredError(t, res, http.StatusBadRequest, "invalid_request")
	if store.upsertCalls != 0 {
		t.Errorf("AC4: UpsertCredential must not be called on validation failure; got %d calls", store.upsertCalls)
	}
}

// ---------------------------------------------------------------------------
// AC5 — DELETE → 204 No Content, DeleteCredential called once
// ---------------------------------------------------------------------------

func TestCredentials_Revoke_Returns204(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &stubCredentialStore{}
	querier := &stubCredQuerier{orgID: orgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	tok := makeCredJWT(t, uuid.New())
	res := doCredRequest(t, h, http.MethodDelete, "/v1/orgs/"+orgID.String()+"/credentials", tok, nil)
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(res.Body)
		t.Errorf("AC5: expected 204 No Content, got %d — body: %s", res.StatusCode, body)
	}
	if store.deleteCalls != 1 {
		t.Errorf("AC5: DeleteCredential called %d times, want 1", store.deleteCalls)
	}
}

// ---------------------------------------------------------------------------
// AC6 — GET when key exists → 200, has_key: true
// ---------------------------------------------------------------------------

func TestCredentials_Status_KeyExists_Returns200True(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &stubCredentialStore{hasKey: true}
	querier := &stubCredQuerier{orgID: orgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	tok := makeCredJWT(t, uuid.New())
	res := doCredRequest(t, h, http.MethodGet, "/v1/orgs/"+orgID.String()+"/credentials", tok, nil)
	defer res.Body.Close()

	assertCredStatus(t, res, http.StatusOK, true)
}

// ---------------------------------------------------------------------------
// AC7 — GET when key doesn't exist → 200, has_key: false
// ---------------------------------------------------------------------------

func TestCredentials_Status_NoKey_Returns200False(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &stubCredentialStore{hasKey: false}
	querier := &stubCredQuerier{orgID: orgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	tok := makeCredJWT(t, uuid.New())
	res := doCredRequest(t, h, http.MethodGet, "/v1/orgs/"+orgID.String()+"/credentials", tok, nil)
	defer res.Body.Close()

	assertCredStatus(t, res, http.StatusOK, false)
}

// ---------------------------------------------------------------------------
// AC8 — Route org ID ≠ tenant org ID → 403 forbidden, store never called
// ---------------------------------------------------------------------------

func TestCredentials_OrgIDMismatch_Returns403(t *testing.T) {
	tenantOrgID := uuid.New()
	routeOrgID := uuid.New() // deliberately different
	groupID := uuid.New()
	store := &stubCredentialStore{}
	querier := &stubCredQuerier{orgID: tenantOrgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	tok := makeCredJWT(t, uuid.New())
	// Route path has routeOrgID but TenantContext has tenantOrgID.
	res := doCredRequest(t, h, http.MethodGet, "/v1/orgs/"+routeOrgID.String()+"/credentials", tok, nil)
	defer res.Body.Close()

	assertCredError(t, res, http.StatusForbidden, "forbidden")
	if store.upsertCalls != 0 || store.deleteCalls != 0 {
		t.Error("AC8: store must not be called when org ID mismatches")
	}
}

// ---------------------------------------------------------------------------
// AC9 — Response body never contains any fragment of the plaintext key
// ---------------------------------------------------------------------------

func TestCredentials_Register_ResponseNeverContainsKey(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &stubCredentialStore{}
	querier := &stubCredQuerier{orgID: orgID, groupID: groupID, role: middleware.RoleOwner}
	h := credMux(t, querier, store)

	secretKey := "sk-ant-api03-super-secret-that-must-not-appear-in-response"
	tok := makeCredJWT(t, uuid.New())
	res := doCredRequest(t, h, http.MethodPost, "/v1/orgs/"+orgID.String()+"/credentials", tok,
		map[string]any{"key": secretKey})
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	bodyStr := string(body)
	if strings.Contains(bodyStr, secretKey) {
		t.Errorf("AC9: response body contains plaintext key — must never happen; body: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "sk-ant-") {
		t.Errorf("AC9: response body contains key prefix — no key material allowed in responses; body: %s", bodyStr)
	}
}

// ---------------------------------------------------------------------------
// Compile check — every exported symbol from credentials.go is referenced
// ---------------------------------------------------------------------------

var _ handlers.CredentialStore = (*stubCredentialStore)(nil)
