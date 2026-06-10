// Package handlers_test is the failing-first test suite for Task 0009
// (provider-credentials).
//
// TDD red phase: these tests reference types in internal/handlers that do not
// exist yet (CredentialStore, RegisterCredential, RotateCredential,
// RevokeCredential, GetCredentialStatus). The file will not compile until the
// Coder creates internal/handlers/credentials.go.
//
// Run with:
//
//	go test ./tests/unit/handlers/... -run TestCredential -v
//
// No network calls, no Supabase credentials, no environment variables required.
//
// AC mapping
// ----------
// AC1  — TestCredential_POST_ValidKey_Returns201
// AC2  — TestCredential_POST_InvalidKey_Returns400
// AC3  — TestCredential_PUT_ValidKey_Returns200
// AC4  — TestCredential_PUT_InvalidKey_Returns400
// AC5  — TestCredential_DELETE_Returns204
// AC6  — TestCredential_GET_HasKey_Returns200True
// AC7  — TestCredential_GET_NoKey_Returns200False
// AC8  — TestCredential_OrgIDMismatch_Returns403
// AC9  — TestCredential_ResponseNeverContainsPlaintextKey
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

type mockCredentialStore struct {
	// UpsertCredential behavior
	upsertErr        error
	upsertCalls      int
	capturedKey      string
	capturedUpsertID uuid.UUID

	// DeleteCredential behavior
	deleteErr   error
	deleteCalls int

	// HasCredential behavior
	hasResult    bool
	hasErr       error
	hasCalls     int
}

func (m *mockCredentialStore) UpsertCredential(_ context.Context, _ string, orgID uuid.UUID, plaintextKey string) error {
	m.upsertCalls++
	m.capturedUpsertID = orgID
	m.capturedKey = plaintextKey
	return m.upsertErr
}

func (m *mockCredentialStore) DeleteCredential(_ context.Context, _ string, _ uuid.UUID) error {
	m.deleteCalls++
	return m.deleteErr
}

func (m *mockCredentialStore) HasCredential(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	m.hasCalls++
	return m.hasResult, m.hasErr
}

// Compile-time interface check.
var _ handlers.CredentialStore = (*mockCredentialStore)(nil)

// ---------------------------------------------------------------------------
// Shared test fixtures
// ---------------------------------------------------------------------------

const credTestJWTSecret = "super-secret-for-0009-credential-tests"

// makeCredJWT returns a signed HS256 JWT for the given userID.
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

// credAuthCfg returns an auth.Config wired to the test JWT secret.
func credAuthCfg() auth.Config {
	return auth.Config{
		SupabaseURL: "https://fake.supabase.co",
		AnonKey:     "test-anon-key",
		JWTSecret:   credTestJWTSecret,
		Port:        "0",
	}
}

// stubOwnerQuerier returns a stub TenantQuerier that grants RoleOwner for
// the given org/group IDs.
type stubOwnerQuerier struct {
	orgID   uuid.UUID
	groupID uuid.UUID
}

func (s *stubOwnerQuerier) CurrentTenant(_ context.Context, _ uuid.UUID) (middleware.TenantContext, error) {
	return middleware.TenantContext{
		OrgID:   s.orgID,
		GroupID: s.groupID,
		Role:    middleware.RoleOwner,
	}, nil
}

// buildCredMux returns a ServeMux with the four credential routes mounted,
// each wrapped in the full middleware chain (Auth → Tenant → RequireRole(owner)).
// Using a real ServeMux ensures r.PathValue("id") resolves correctly on Go 1.22.
func buildCredMux(
	t *testing.T,
	orgID uuid.UUID,
	store handlers.CredentialStore,
) http.Handler {
	t.Helper()

	querier := &stubOwnerQuerier{orgID: orgID, groupID: uuid.New()}
	authCfg := credAuthCfg()

	ownerChain := func(h http.Handler) http.Handler {
		return auth.AuthMiddleware(authCfg)(
			middleware.TenantMiddleware(querier)(
				middleware.RequireRole(middleware.RoleOwner)(h),
			),
		)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /v1/orgs/{id}/credentials", ownerChain(handlers.RegisterCredential(store)))
	mux.Handle("PUT /v1/orgs/{id}/credentials", ownerChain(handlers.RotateCredential(store)))
	mux.Handle("DELETE /v1/orgs/{id}/credentials", ownerChain(handlers.RevokeCredential(store)))
	mux.Handle("GET /v1/orgs/{id}/credentials", ownerChain(handlers.GetCredentialStatus(store)))
	return mux
}

// doRequest is a small helper that fires a request at the mux and returns the response.
func doRequest(t *testing.T, mux http.Handler, method, url, token string, body any) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("doRequest: marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, url, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Result()
}

// assertCredOKResponse asserts status and decodes the success body.
// Returns (org_id, has_key) from the response.
func assertCredOKResponse(t *testing.T, res *http.Response, wantStatus int) (string, bool) {
	t.Helper()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != wantStatus {
		t.Errorf("expected HTTP %d, got %d — body: %s", wantStatus, res.StatusCode, body)
		return "", false
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var m struct {
		OrgID  string `json:"org_id"`
		HasKey bool   `json:"has_key"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("response body is not valid JSON: %v — body: %s", err, body)
	}
	return m.OrgID, m.HasKey
}

// assertCredErrorCode asserts a RFC §4.11 error envelope with the expected
// HTTP status and error code.
func assertCredErrorCode(t *testing.T, res *http.Response, wantStatus int, wantCode string) {
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
		t.Fatalf("body is not valid JSON: %v — body: %s", err, body)
	}
	if env.Error.Code != wantCode {
		t.Errorf("error.code = %q, want %q — body: %s", env.Error.Code, wantCode, body)
	}
	if env.Error.Message == "" {
		t.Error("error.message must not be empty")
	}
}

// ---------------------------------------------------------------------------
// AC1 — POST with valid key (sk-ant-*) → 201 Created, has_key: true
// ---------------------------------------------------------------------------

// TestCredential_POST_ValidKey_Returns201 verifies AC-1:
// POST /v1/orgs/{orgID}/credentials with a valid sk-ant- key returns 201,
// body {"org_id":"<uuid>","has_key":true}, Content-Type: application/json,
// and UpsertCredential is called exactly once with the plaintext key.
func TestCredential_POST_ValidKey_Returns201(t *testing.T) {
	orgID := uuid.New()
	store := &mockCredentialStore{}
	userID := uuid.New()
	tok := makeCredJWT(t, userID)
	mux := buildCredMux(t, orgID, store)

	const validKey = "sk-ant-api03-validkey123"
	res := doRequest(t, mux, http.MethodPost,
		"/v1/orgs/"+orgID.String()+"/credentials",
		tok, map[string]string{"key": validKey})
	defer res.Body.Close()

	gotOrgID, hasKey := assertCredOKResponse(t, res, http.StatusCreated)

	if gotOrgID != orgID.String() {
		t.Errorf("AC1: org_id = %q, want %q", gotOrgID, orgID.String())
	}
	if !hasKey {
		t.Error("AC1: has_key must be true")
	}
	if store.upsertCalls != 1 {
		t.Errorf("AC1: UpsertCredential called %d times, want 1", store.upsertCalls)
	}
	if store.capturedKey != validKey {
		t.Errorf("AC1: captured key = %q, want %q", store.capturedKey, validKey)
	}
}

// ---------------------------------------------------------------------------
// AC2 — POST with invalid key (no sk-ant- prefix) → 400 invalid_request
// ---------------------------------------------------------------------------

// TestCredential_POST_InvalidKey_Returns400 verifies AC-2:
// POST with body {"key":"invalid-format"} → 400, error.code == "invalid_request".
// UpsertCredential must not be called.
func TestCredential_POST_InvalidKey_Returns400(t *testing.T) {
	orgID := uuid.New()
	store := &mockCredentialStore{}
	tok := makeCredJWT(t, uuid.New())
	mux := buildCredMux(t, orgID, store)

	res := doRequest(t, mux, http.MethodPost,
		"/v1/orgs/"+orgID.String()+"/credentials",
		tok, map[string]string{"key": "invalid-format"})
	defer res.Body.Close()

	assertCredErrorCode(t, res, http.StatusBadRequest, "invalid_request")

	if store.upsertCalls != 0 {
		t.Errorf("AC2: UpsertCredential must not be called on validation error; got %d calls", store.upsertCalls)
	}
}

// ---------------------------------------------------------------------------
// AC3 — PUT with valid key → 200 OK, has_key: true
// ---------------------------------------------------------------------------

// TestCredential_PUT_ValidKey_Returns200 verifies AC-3:
// PUT /v1/orgs/{orgID}/credentials with a valid sk-ant- key returns 200,
// body {"org_id":"<uuid>","has_key":true}, UpsertCredential called once.
func TestCredential_PUT_ValidKey_Returns200(t *testing.T) {
	orgID := uuid.New()
	store := &mockCredentialStore{}
	tok := makeCredJWT(t, uuid.New())
	mux := buildCredMux(t, orgID, store)

	const validKey = "sk-ant-api03-rotated456"
	res := doRequest(t, mux, http.MethodPut,
		"/v1/orgs/"+orgID.String()+"/credentials",
		tok, map[string]string{"key": validKey})
	defer res.Body.Close()

	gotOrgID, hasKey := assertCredOKResponse(t, res, http.StatusOK)

	if gotOrgID != orgID.String() {
		t.Errorf("AC3: org_id = %q, want %q", gotOrgID, orgID.String())
	}
	if !hasKey {
		t.Error("AC3: has_key must be true")
	}
	if store.upsertCalls != 1 {
		t.Errorf("AC3: UpsertCredential called %d times, want 1", store.upsertCalls)
	}
}

// ---------------------------------------------------------------------------
// AC4 — PUT with invalid key → 400 invalid_request
// ---------------------------------------------------------------------------

// TestCredential_PUT_InvalidKey_Returns400 verifies AC-4:
// PUT with an invalid key returns 400 invalid_request. UpsertCredential not called.
func TestCredential_PUT_InvalidKey_Returns400(t *testing.T) {
	orgID := uuid.New()
	store := &mockCredentialStore{}
	tok := makeCredJWT(t, uuid.New())
	mux := buildCredMux(t, orgID, store)

	res := doRequest(t, mux, http.MethodPut,
		"/v1/orgs/"+orgID.String()+"/credentials",
		tok, map[string]string{"key": "bad-key"})
	defer res.Body.Close()

	assertCredErrorCode(t, res, http.StatusBadRequest, "invalid_request")

	if store.upsertCalls != 0 {
		t.Errorf("AC4: UpsertCredential must not be called; got %d calls", store.upsertCalls)
	}
}

// ---------------------------------------------------------------------------
// AC5 — DELETE → 204 No Content
// ---------------------------------------------------------------------------

// TestCredential_DELETE_Returns204 verifies AC-5:
// DELETE /v1/orgs/{orgID}/credentials returns 204 with empty body.
// DeleteCredential is called exactly once.
func TestCredential_DELETE_Returns204(t *testing.T) {
	orgID := uuid.New()
	store := &mockCredentialStore{}
	tok := makeCredJWT(t, uuid.New())
	mux := buildCredMux(t, orgID, store)

	res := doRequest(t, mux, http.MethodDelete,
		"/v1/orgs/"+orgID.String()+"/credentials",
		tok, nil)
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(res.Body)
		t.Errorf("AC5: expected 204, got %d — body: %s", res.StatusCode, body)
	}
	// Body must be empty.
	body, _ := io.ReadAll(res.Body)
	if len(body) != 0 {
		t.Errorf("AC5: expected empty body for 204, got: %s", body)
	}
	if store.deleteCalls != 1 {
		t.Errorf("AC5: DeleteCredential called %d times, want 1", store.deleteCalls)
	}
}

// ---------------------------------------------------------------------------
// AC6 — GET when key exists → 200 OK, has_key: true
// ---------------------------------------------------------------------------

// TestCredential_GET_HasKey_Returns200True verifies AC-6:
// GET /v1/orgs/{orgID}/credentials when HasCredential returns true.
// Response: 200, {"org_id":"<uuid>","has_key":true}.
func TestCredential_GET_HasKey_Returns200True(t *testing.T) {
	orgID := uuid.New()
	store := &mockCredentialStore{hasResult: true}
	tok := makeCredJWT(t, uuid.New())
	mux := buildCredMux(t, orgID, store)

	res := doRequest(t, mux, http.MethodGet,
		"/v1/orgs/"+orgID.String()+"/credentials",
		tok, nil)
	defer res.Body.Close()

	gotOrgID, hasKey := assertCredOKResponse(t, res, http.StatusOK)

	if gotOrgID != orgID.String() {
		t.Errorf("AC6: org_id = %q, want %q", gotOrgID, orgID.String())
	}
	if !hasKey {
		t.Error("AC6: has_key must be true when HasCredential returns true")
	}
	if store.hasCalls != 1 {
		t.Errorf("AC6: HasCredential called %d times, want 1", store.hasCalls)
	}
}

// ---------------------------------------------------------------------------
// AC7 — GET when key does not exist → 200 OK, has_key: false
// ---------------------------------------------------------------------------

// TestCredential_GET_NoKey_Returns200False verifies AC-7:
// GET when HasCredential returns false → 200, {"org_id":"<uuid>","has_key":false}.
func TestCredential_GET_NoKey_Returns200False(t *testing.T) {
	orgID := uuid.New()
	store := &mockCredentialStore{hasResult: false}
	tok := makeCredJWT(t, uuid.New())
	mux := buildCredMux(t, orgID, store)

	res := doRequest(t, mux, http.MethodGet,
		"/v1/orgs/"+orgID.String()+"/credentials",
		tok, nil)
	defer res.Body.Close()

	gotOrgID, hasKey := assertCredOKResponse(t, res, http.StatusOK)

	if gotOrgID != orgID.String() {
		t.Errorf("AC7: org_id = %q, want %q", gotOrgID, orgID.String())
	}
	if hasKey {
		t.Error("AC7: has_key must be false when HasCredential returns false")
	}
}

// ---------------------------------------------------------------------------
// AC8 — Path org ID != TenantContext org ID → 403 forbidden
// ---------------------------------------------------------------------------

// TestCredential_OrgIDMismatch_Returns403 verifies AC-8:
// When the {id} path param does not match tc.OrgID, all four handlers
// return 403 forbidden and store methods are not called.
func TestCredential_OrgIDMismatch_Returns403(t *testing.T) {
	realOrgID := uuid.New()
	differentOrgID := uuid.New() // this is in the path but tenant has realOrgID

	// buildCredMux seeds the TenantContext with realOrgID.
	// We send requests with differentOrgID in the path.
	store := &mockCredentialStore{hasResult: true}
	tok := makeCredJWT(t, uuid.New())
	mux := buildCredMux(t, realOrgID, store)

	pathURL := "/v1/orgs/" + differentOrgID.String() + "/credentials"

	cases := []struct {
		method string
		body   any
	}{
		{http.MethodPost, map[string]string{"key": "sk-ant-api03-xyz"}},
		{http.MethodPut, map[string]string{"key": "sk-ant-api03-xyz"}},
		{http.MethodDelete, nil},
		{http.MethodGet, nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method, func(t *testing.T) {
			res := doRequest(t, mux, tc.method, pathURL, tok, tc.body)
			defer res.Body.Close()
			assertCredErrorCode(t, res, http.StatusForbidden, "forbidden")
		})
	}

	// No store method should have been called.
	if store.upsertCalls != 0 {
		t.Errorf("AC8: UpsertCredential called %d times, want 0", store.upsertCalls)
	}
	if store.deleteCalls != 0 {
		t.Errorf("AC8: DeleteCredential called %d times, want 0", store.deleteCalls)
	}
	if store.hasCalls != 0 {
		t.Errorf("AC8: HasCredential called %d times, want 0", store.hasCalls)
	}
}

// ---------------------------------------------------------------------------
// AC9 — No response contains the plaintext key
// ---------------------------------------------------------------------------

// TestCredential_ResponseNeverContainsPlaintextKey verifies AC-9:
// The 201 and 200 response bodies do not contain any substring starting with
// "sk-ant-". The key is passed only to the store mock, never serialized.
func TestCredential_ResponseNeverContainsPlaintextKey(t *testing.T) {
	orgID := uuid.New()
	store := &mockCredentialStore{hasResult: true}
	tok := makeCredJWT(t, uuid.New())
	mux := buildCredMux(t, orgID, store)

	const validKey = "sk-ant-api03-supersecret789"
	orgPath := "/v1/orgs/" + orgID.String() + "/credentials"

	t.Run("POST 201 body", func(t *testing.T) {
		res := doRequest(t, mux, http.MethodPost, orgPath, tok,
			map[string]string{"key": validKey})
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if strings.Contains(string(body), "sk-ant-") {
			t.Errorf("AC9: POST response body contains plaintext key prefix — body: %s", body)
		}
	})

	t.Run("PUT 200 body", func(t *testing.T) {
		res := doRequest(t, mux, http.MethodPut, orgPath, tok,
			map[string]string{"key": validKey})
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if strings.Contains(string(body), "sk-ant-") {
			t.Errorf("AC9: PUT response body contains plaintext key prefix — body: %s", body)
		}
	})
}
