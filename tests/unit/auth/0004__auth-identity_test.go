// Package auth_test is the failing-first test suite for Task 0004 (auth-identity).
//
// TDD red phase: these tests reference github.com/nomenclator/houston2/internal/auth,
// which does not exist yet. The file compiles only after the Coder creates the
// implementation. Every test is expected to FAIL (or fail to compile) until then.
//
// AC mapping
// ----------
// AC1  — .env.example — MANUAL CHECK (not a Go unit test; noted below)
// AC2  — TestLoginHandler_RedirectsWithPKCEAndCookie
// AC3  — TestCallbackHandler_ValidCodeAndState_Returns200WithToken
// AC4  — TestCallbackHandler_MismatchedState_Returns400_NoSupabaseCall
// AC5  — TestAuthMiddleware_ValidJWT_CallsNext_AndContextHasUUID
// AC6  — TestAuthMiddleware_TamperedJWT_Returns401
// AC7  — TestAuthMiddleware_ExpiredJWT_Returns401
// AC8  — TestUserIDFromContext_EmptyContext_ReturnsNilFalse
// AC9  — TestNewUserClient_InjectsAuthorizationHeader
// AC10 — covered by compilation: every test references typed symbols from internal/auth

package auth_test

// AC1 NOTE: .env.example must exist at the repo root with the four documented
// variables (SUPABASE_URL, SUPABASE_ANON_KEY, SUPABASE_JWT_SECRET, SERVER_PORT),
// each with a placeholder value and an inline comment. This is a filesystem
// presence check performed during acceptance review, not a Go unit test.

import (
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
)

// ---------------------------------------------------------------------------
// Shared test fixtures
// ---------------------------------------------------------------------------

const testJWTSecret = "super-secret-for-testing-only"

// testCfg returns an auth.Config wired to a fake Supabase URL.
// The URL is replaced per-test when a real httptest.Server is used.
func testCfg(supabaseURL string) auth.Config {
	return auth.Config{
		SupabaseURL: supabaseURL,
		AnonKey:     "test-anon-key",
		JWTSecret:   testJWTSecret,
		Port:        "8080",
	}
}

// makeJWT creates an HS256-signed JWT with the given sub (UUID string) and
// expiry, signed with testJWTSecret.
func makeJWT(t *testing.T, sub string, expiry time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": sub,
		"exp": jwt.NewNumericDate(expiry),
		"iat": jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("makeJWT: %v", err)
	}
	return signed
}

// makeExpiredJWT creates a JWT whose exp is 1 hour in the past.
func makeExpiredJWT(t *testing.T, sub string) string {
	t.Helper()
	return makeJWT(t, sub, time.Now().Add(-1*time.Hour))
}

// makeTamperedJWT creates a valid JWT then corrupts the signature segment.
func makeTamperedJWT(t *testing.T, sub string) string {
	t.Helper()
	good := makeJWT(t, sub, time.Now().Add(1*time.Hour))
	parts := strings.Split(good, ".")
	if len(parts) != 3 {
		t.Fatal("makeTamperedJWT: unexpected JWT structure")
	}
	parts[2] = parts[2] + "XXXXXX"
	return strings.Join(parts, ".")
}

// ---------------------------------------------------------------------------
// AC2 — LoginHandler: 302, PKCE params, pkce_state cookie
// ---------------------------------------------------------------------------

// TestLoginHandler_RedirectsWithPKCEAndCookie verifies acceptance criterion 2:
//   - HTTP status is 302
//   - Location header contains response_type=code, code_challenge,
//     code_challenge_method=S256, and state
//   - Response sets a pkce_state cookie with HttpOnly and SameSite=Lax
func TestLoginHandler_RedirectsWithPKCEAndCookie(t *testing.T) {
	cfg := testCfg("https://fake.supabase.co")
	handler := auth.LoginHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Errorf("AC2: expected 302, got %d", res.StatusCode)
	}

	loc := res.Header.Get("Location")
	if loc == "" {
		t.Fatal("AC2: Location header is empty")
	}

	for _, param := range []string{"response_type=code", "code_challenge=", "code_challenge_method=S256", "state="} {
		if !strings.Contains(loc, param) {
			t.Errorf("AC2: Location header missing %q — got %s", param, loc)
		}
	}

	var pkceStateCookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "pkce_state" {
			pkceStateCookie = c
			break
		}
	}
	if pkceStateCookie == nil {
		t.Fatal("AC2: pkce_state cookie not set")
	}
	if !pkceStateCookie.HttpOnly {
		t.Error("AC2: pkce_state cookie must be HttpOnly")
	}
	if pkceStateCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("AC2: pkce_state cookie SameSite must be Lax, got %v", pkceStateCookie.SameSite)
	}
	if pkceStateCookie.Value == "" {
		t.Error("AC2: pkce_state cookie value is empty")
	}
}

// ---------------------------------------------------------------------------
// AC3 — CallbackHandler: valid code+state → 200 { "token": "..." }
// ---------------------------------------------------------------------------

// fakeSupabaseTokenServer returns an httptest.Server that mimics the Supabase
// token endpoint by returning a minimal JWT-like token string.
func fakeSupabaseTokenServer(t *testing.T, responseToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]string{"access_token": responseToken})
		w.Write(body)
	}))
}

// TestCallbackHandler_ValidCodeAndState_Returns200WithToken verifies AC3:
// valid code + matching state cookie → 200 { "token": "<non-empty>" }.
func TestCallbackHandler_ValidCodeAndState_Returns200WithToken(t *testing.T) {
	fakeToken := makeJWT(t, uuid.New().String(), time.Now().Add(1*time.Hour))
	srv := fakeSupabaseTokenServer(t, fakeToken)
	defer srv.Close()

	cfg := testCfg(srv.URL)
	handler := auth.CallbackHandler(cfg)

	// We need a valid state. Get one by calling LoginHandler first.
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	loginRec := httptest.NewRecorder()
	auth.LoginHandler(cfg).ServeHTTP(loginRec, loginReq)
	loginRes := loginRec.Result()
	defer loginRes.Body.Close()

	// Extract state from Location and the pkce_state cookie value.
	loc := loginRes.Header.Get("Location")
	stateVal := extractQueryParam(t, loc, "state")

	var pkceStateCookie *http.Cookie
	for _, c := range loginRes.Cookies() {
		if c.Name == "pkce_state" {
			pkceStateCookie = c
		}
	}
	if pkceStateCookie == nil {
		t.Fatal("AC3 setup: pkce_state cookie missing from LoginHandler response")
	}

	// Build callback request with matching cookie and query params.
	callbackReq := httptest.NewRequest(http.MethodGet,
		"/auth/callback?code=testcode&state="+stateVal, nil)
	callbackReq.AddCookie(pkceStateCookie)

	callbackRec := httptest.NewRecorder()
	handler.ServeHTTP(callbackRec, callbackReq)

	callbackRes := callbackRec.Result()
	defer callbackRes.Body.Close()

	if callbackRes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(callbackRes.Body)
		t.Fatalf("AC3: expected 200, got %d — body: %s", callbackRes.StatusCode, body)
	}

	var payload map[string]string
	if err := json.NewDecoder(callbackRes.Body).Decode(&payload); err != nil {
		t.Fatalf("AC3: response body is not valid JSON: %v", err)
	}
	tok, ok := payload["token"]
	if !ok || tok == "" {
		t.Errorf("AC3: response JSON missing non-empty 'token' field, got %v", payload)
	}
}

// ---------------------------------------------------------------------------
// AC4 — CallbackHandler: mismatched state → 400, no Supabase call
// ---------------------------------------------------------------------------

// TestCallbackHandler_MismatchedState_Returns400_NoSupabaseCall verifies AC4:
// mismatched state → HTTP 400 and the mock Supabase server is never contacted.
func TestCallbackHandler_MismatchedState_Returns400_NoSupabaseCall(t *testing.T) {
	called := false
	fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fakeSrv.Close()

	cfg := testCfg(fakeSrv.URL)
	handler := auth.CallbackHandler(cfg)

	// Set a pkce_state cookie whose value does NOT match the state query param.
	req := httptest.NewRequest(http.MethodGet,
		"/auth/callback?code=anycode&state=wrong-state-value", nil)
	req.AddCookie(&http.Cookie{
		Name:  "pkce_state",
		Value: "correct-state-value-that-does-not-match",
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("AC4: expected 400, got %d", res.StatusCode)
	}
	if called {
		t.Error("AC4: Supabase token endpoint was called despite state mismatch — must not be")
	}
}

// ---------------------------------------------------------------------------
// AC5 — AuthMiddleware: valid JWT → next called, UserIDFromContext returns UUID
// ---------------------------------------------------------------------------

// TestAuthMiddleware_ValidJWT_CallsNext_AndContextHasUUID verifies AC5.
func TestAuthMiddleware_ValidJWT_CallsNext_AndContextHasUUID(t *testing.T) {
	userID := uuid.New()
	token := makeJWT(t, userID.String(), time.Now().Add(1*time.Hour))

	cfg := testCfg("https://fake.supabase.co")
	middleware := auth.AuthMiddleware(cfg)

	nextCalled := false
	var ctxUserID uuid.UUID
	var ctxOK bool

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		ctxUserID, ctxOK = auth.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(nextHandler)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("AC5: next handler was not called for a valid JWT")
	}
	if !ctxOK {
		t.Error("AC5: UserIDFromContext returned ok=false for a valid JWT")
	}
	if ctxUserID == uuid.Nil {
		t.Error("AC5: UserIDFromContext returned uuid.Nil for a valid JWT")
	}
	if ctxUserID != userID {
		t.Errorf("AC5: context user_id %v != expected %v", ctxUserID, userID)
	}
}

// ---------------------------------------------------------------------------
// AC6 — AuthMiddleware: tampered JWT → 401 unauthenticated envelope
// ---------------------------------------------------------------------------

// TestAuthMiddleware_TamperedJWT_Returns401 verifies AC6.
func TestAuthMiddleware_TamperedJWT_Returns401(t *testing.T) {
	cfg := testCfg("https://fake.supabase.co")
	middleware := auth.AuthMiddleware(cfg)

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	tampered := makeTamperedJWT(t, uuid.New().String())
	handler := middleware(nextHandler)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tampered)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("AC6: expected 401, got %d", res.StatusCode)
	}
	if nextCalled {
		t.Error("AC6: next handler must NOT be called for a tampered JWT")
	}
	assertUnauthenticatedEnvelope(t, "AC6", res.Body)
}

// ---------------------------------------------------------------------------
// AC7 — AuthMiddleware: expired JWT → 401 unauthenticated envelope
// ---------------------------------------------------------------------------

// TestAuthMiddleware_ExpiredJWT_Returns401 verifies AC7.
func TestAuthMiddleware_ExpiredJWT_Returns401(t *testing.T) {
	cfg := testCfg("https://fake.supabase.co")
	middleware := auth.AuthMiddleware(cfg)

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	expired := makeExpiredJWT(t, uuid.New().String())
	handler := middleware(nextHandler)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+expired)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("AC7: expected 401, got %d", res.StatusCode)
	}
	if nextCalled {
		t.Error("AC7: next handler must NOT be called for an expired JWT")
	}
	assertUnauthenticatedEnvelope(t, "AC7", res.Body)
}

// ---------------------------------------------------------------------------
// AC8 — UserIDFromContext: empty context → uuid.Nil, false
// ---------------------------------------------------------------------------

// TestUserIDFromContext_EmptyContext_ReturnsNilFalse verifies AC8.
func TestUserIDFromContext_EmptyContext_ReturnsNilFalse(t *testing.T) {
	id, ok := auth.UserIDFromContext(context.Background())
	if ok {
		t.Error("AC8: expected ok=false for empty context, got true")
	}
	if id != uuid.Nil {
		t.Errorf("AC8: expected uuid.Nil for empty context, got %v", id)
	}
}

// ---------------------------------------------------------------------------
// AC9 — NewUserClient: transport injects Authorization: Bearer header
// ---------------------------------------------------------------------------

// TestNewUserClient_InjectsAuthorizationHeader verifies AC9.
func TestNewUserClient_InjectsAuthorizationHeader(t *testing.T) {
	const testToken = "my.test.jwt"
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := auth.NewUserClient(testToken)
	resp, err := client.Get(srv.URL + "/anything")
	if err != nil {
		t.Fatalf("AC9: request failed: %v", err)
	}
	resp.Body.Close()

	expected := "Bearer " + testToken
	if receivedAuth != expected {
		t.Errorf("AC9: Authorization header = %q, want %q", receivedAuth, expected)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// assertUnauthenticatedEnvelope checks that the response body is a JSON object
// with the shape { "error": { "code": "unauthenticated", ... } }.
func assertUnauthenticatedEnvelope(t *testing.T, prefix string, body io.Reader) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	raw, _ := io.ReadAll(body)
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Errorf("%s: response body is not valid JSON: %v — body: %s", prefix, err, raw)
		return
	}
	if envelope.Error.Code != "unauthenticated" {
		t.Errorf("%s: error.code = %q, want \"unauthenticated\"", prefix, envelope.Error.Code)
	}
	if envelope.Error.Message == "" {
		t.Errorf("%s: error.message must not be empty", prefix)
	}
	// request_id is expected but not strictly validated for content — just presence.
	if envelope.Error.RequestID == "" {
		t.Errorf("%s: error.request_id must not be empty", prefix)
	}
}

// extractQueryParam parses a raw URL string and returns the value of the named
// query parameter. Fails the test if the parameter is absent.
func extractQueryParam(t *testing.T, rawURL, name string) string {
	t.Helper()
	// Find the query string portion.
	idx := strings.Index(rawURL, "?")
	if idx == -1 {
		t.Fatalf("extractQueryParam: no query string in URL %q", rawURL)
	}
	query := rawURL[idx+1:]
	for _, pair := range strings.Split(query, "&") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 && kv[0] == name {
			return kv[1]
		}
	}
	t.Fatalf("extractQueryParam: param %q not found in %q", name, rawURL)
	return ""
}
