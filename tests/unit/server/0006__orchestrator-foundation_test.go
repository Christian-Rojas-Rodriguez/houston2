// Package server_test is the failing-first test suite for Task 0006
// (orchestrator-foundation).
//
// TDD red phase: these tests reference packages that do not exist yet:
//   - github.com/nomenclator/houston2/internal/server
//   - github.com/nomenclator/houston2/internal/auth      (Task 0004)
//   - github.com/nomenclator/houston2/internal/middleware (Task 0005)
//
// The file will compile only after the Coder creates those implementations.
// Every test is expected to FAIL (or fail to compile) until then.
//
// AC mapping
// ----------
// AC1  — TestHealthHandler_Returns200WithoutAuth
// AC2  — TestPostAgents_NoAuth_Returns401Unauthenticated
// AC3  — TestPostAgents_ValidJWT_NoMembership_Returns403Forbidden
// AC4  — TestPostAgents_ValidJWT_RoleMember_RequireMember_Returns200Stub
// AC5  — TestPostAgents_ValidJWT_RoleMember_RequireManager_Returns403
// AC6  — TestServer_StartsOnEnvPort_HealthReturns200
// AC7  — TestV1Routes_ResponseHasXRequestID
// AC8  — TestLoggingMiddleware_JSONLogHasRequiredFields
// AC9  — TestSupabaseQuerier_CurrentTenant_SendsCorrectHeaders
// AC10 — covered by compilation: every test references typed symbols

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/middleware"
	"github.com/nomenclator/houston2/internal/server"
)

// ---------------------------------------------------------------------------
// Shared test fixtures
// ---------------------------------------------------------------------------

const testJWTSecret = "super-secret-for-0006-testing-only"

// makeJWT creates an HS256-signed JWT with the given sub UUID string and expiry,
// signed with testJWTSecret. Mirrors the helper in the 0004 test suite.
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

// makeValidJWT returns a non-expired JWT for a fresh UUID subject.
func makeValidJWT(t *testing.T) (string, uuid.UUID) {
	t.Helper()
	id := uuid.New()
	tok := makeJWT(t, id.String(), time.Now().Add(1*time.Hour))
	return tok, id
}

// testServerConfig returns a server.Config that points auth at a stub
// Supabase URL and uses testJWTSecret. supabaseURL is replaced per-test
// when a real httptest.Server is used.
func testServerConfig(supabaseURL string) server.Config {
	return server.Config{
		SupabaseURL: supabaseURL,
		AnonKey:     "test-anon-key",
		JWTSecret:   testJWTSecret,
		Port:        "0", // unused for handler-level tests
		LogLevel:    "info",
	}
}

// ---------------------------------------------------------------------------
// Stub TenantQuerier — used for AC1–AC8 handler-level tests so tests
// do not depend on the supabaseQuerier concrete implementation.
// ---------------------------------------------------------------------------

// stubQuerier implements middleware.TenantQuerier and returns a fixed result.
type stubQuerier struct {
	// returnErr is returned as-is from CurrentTenant when non-nil.
	returnErr error
	// returnCtx is returned when returnErr is nil.
	returnCtx middleware.TenantContext
}

func (s *stubQuerier) CurrentTenant(_ context.Context, _ uuid.UUID) (middleware.TenantContext, error) {
	if s.returnErr != nil {
		return middleware.TenantContext{}, s.returnErr
	}
	return s.returnCtx, nil
}

// errNoMembership aliases middleware.ErrNoMembership for readability.
var errNoMembership = middleware.ErrNoMembership

// ---------------------------------------------------------------------------
// assertForbiddenEnvelope asserts the body is a JSON error with code "forbidden".
// ---------------------------------------------------------------------------

func assertForbiddenEnvelope(t *testing.T, prefix string, body io.Reader) {
	t.Helper()
	assertErrorEnvelope(t, prefix, body, "forbidden")
}

// assertUnauthenticatedEnvelope asserts the body has code "unauthenticated".
func assertUnauthenticatedEnvelope(t *testing.T, prefix string, body io.Reader) {
	t.Helper()
	assertErrorEnvelope(t, prefix, body, "unauthenticated")
}

func assertErrorEnvelope(t *testing.T, prefix string, body io.Reader, wantCode string) {
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
	if envelope.Error.Code != wantCode {
		t.Errorf("%s: error.code = %q, want %q — body: %s", prefix, envelope.Error.Code, wantCode, raw)
	}
	if envelope.Error.Message == "" {
		t.Errorf("%s: error.message must not be empty", prefix)
	}
}

// ---------------------------------------------------------------------------
// AC1 — GET /health → 200 {"status":"ok"} without Authorization header
// ---------------------------------------------------------------------------

// TestHealthHandler_Returns200WithoutAuth verifies acceptance criterion 1:
// the /health route is publicly accessible, returns 200, and the body is
// {"status":"ok"}.
func TestHealthHandler_Returns200WithoutAuth(t *testing.T) {
	cfg := testServerConfig("https://fake.supabase.co")
	q := &stubQuerier{returnErr: errNoMembership}
	srv := server.New(cfg, q)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	// Deliberately no Authorization header.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("AC1: expected 200, got %d", res.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("AC1: body is not valid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("AC1: body[\"status\"] = %q, want \"ok\"", body["status"])
	}
}

// ---------------------------------------------------------------------------
// AC2 — POST /v1/agents without Authorization → 401 unauthenticated
// ---------------------------------------------------------------------------

// TestPostAgents_NoAuth_Returns401Unauthenticated verifies acceptance criterion 2.
func TestPostAgents_NoAuth_Returns401Unauthenticated(t *testing.T) {
	cfg := testServerConfig("https://fake.supabase.co")
	q := &stubQuerier{returnErr: errNoMembership}
	srv := server.New(cfg, q)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents", nil)
	// No Authorization header.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("AC2: expected 401, got %d", res.StatusCode)
	}
	assertUnauthenticatedEnvelope(t, "AC2", res.Body)
}

// ---------------------------------------------------------------------------
// AC3 — POST /v1/agents with valid JWT, querier returns ErrNoMembership → 403
// ---------------------------------------------------------------------------

// TestPostAgents_ValidJWT_NoMembership_Returns403Forbidden verifies AC3.
func TestPostAgents_ValidJWT_NoMembership_Returns403Forbidden(t *testing.T) {
	cfg := testServerConfig("https://fake.supabase.co")
	// Stub querier simulates user with no membership row.
	q := &stubQuerier{returnErr: errNoMembership}
	srv := server.New(cfg, q)

	token, _ := makeValidJWT(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Errorf("AC3: expected 403, got %d", res.StatusCode)
	}
	assertForbiddenEnvelope(t, "AC3", res.Body)
}

// ---------------------------------------------------------------------------
// AC4 — POST /v1/agents valid JWT + RoleMember querier + RequireRole(Member) → 200 stub
// ---------------------------------------------------------------------------

// TestPostAgents_ValidJWT_RoleMember_RequireMember_Returns200Stub verifies AC4.
func TestPostAgents_ValidJWT_RoleMember_RequireMember_Returns200Stub(t *testing.T) {
	cfg := testServerConfig("https://fake.supabase.co")
	// Stub querier grants RoleMember, which satisfies RequireRole(RoleMember).
	q := &stubQuerier{
		returnCtx: middleware.TenantContext{
			OrgID:   uuid.New(),
			GroupID: uuid.New(),
			Role:    middleware.RoleMember,
		},
	}
	srv := server.New(cfg, q)

	token, _ := makeValidJWT(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("AC4: expected 200, got %d — body: %s", res.StatusCode, body)
	}

	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("AC4: body is not valid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("AC4: body[\"status\"] = %v, want \"ok\"", body["status"])
	}
	if body["stub"] != true {
		t.Errorf("AC4: body[\"stub\"] = %v, want true", body["stub"])
	}
}

// ---------------------------------------------------------------------------
// AC5 — POST /v1/agents valid JWT + RoleMember querier + RequireRole(Manager) → 403
// ---------------------------------------------------------------------------

// TestPostAgents_ValidJWT_RoleMember_RequireManager_Returns403 verifies AC5:
// a RoleMember caller is rejected when the route requires RoleManager.
// This test requires the server to wire POST /v1/agents with RequireRole(RoleMember),
// not RequireRole(RoleManager). To exercise AC5 directly against the middleware
// we build a mini handler chain using the middleware package.
func TestPostAgents_ValidJWT_RoleMember_RequireManager_Returns403(t *testing.T) {
	// Wire a TenantMiddleware → RequireRole(RoleManager) chain manually,
	// using the auth.AuthMiddleware so the full stack is exercised.
	q := &stubQuerier{
		returnCtx: middleware.TenantContext{
			OrgID:   uuid.New(),
			GroupID: uuid.New(),
			Role:    middleware.RoleMember, // insufficient for Manager gate
		},
	}

	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Build: AuthMiddleware → TenantMiddleware → RequireRole(RoleManager) → stub
	authCfg := auth.Config{
		SupabaseURL: "https://fake.supabase.co",
		AnonKey:     "test-anon-key",
		JWTSecret:   testJWTSecret,
		Port:        "0",
	}
	chain := auth.AuthMiddleware(authCfg)(
		middleware.TenantMiddleware(q)(
			middleware.RequireRole(middleware.RoleManager)(stub),
		),
	)

	token, _ := makeValidJWT(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(res.Body)
		t.Errorf("AC5: expected 403, got %d — body: %s", res.StatusCode, body)
	}
}

// ---------------------------------------------------------------------------
// AC6 — Server starts on SERVER_PORT env var; GET /health returns 200
// ---------------------------------------------------------------------------

// TestServer_StartsOnEnvPort_HealthReturns200 verifies AC6: the server
// binds on the port specified by SERVER_PORT and accepts real HTTP connections.
// We pick a free OS-assigned port to avoid conflicts.
func TestServer_StartsOnEnvPort_HealthReturns200(t *testing.T) {
	// Ask the OS for a free port by binding to :0 briefly.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("AC6: could not acquire free port: %v", err)
	}
	port := fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
	ln.Close()

	t.Setenv("SERVER_PORT", port)
	t.Setenv("SUPABASE_URL", "https://fake.supabase.co")
	t.Setenv("SUPABASE_ANON_KEY", "test-anon-key")
	t.Setenv("SUPABASE_JWT_SECRET", testJWTSecret)
	t.Setenv("LOG_LEVEL", "info")

	cfg := server.LoadConfig() // reads env vars
	q := &stubQuerier{returnErr: errNoMembership}
	srv := server.New(cfg, q)

	// Start in background; cancel after test.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	// Poll for the server to be ready.
	addr := "http://127.0.0.1:" + port + "/health"
	var resp *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get(addr) //nolint:noctx
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("AC6: server never became reachable on port %s: %v", port, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("AC6: GET /health on port %s returned %d, want 200", port, resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// AC7 — Every /v1/* response has X-Request-ID with a non-empty UUID
// ---------------------------------------------------------------------------

// TestV1Routes_ResponseHasXRequestID verifies AC7 for POST /v1/agents.
// We send a valid JWT so the request gets past auth (otherwise it short-circuits
// with 401 before the request-ID header is written — but the spec requires it
// on every /v1/* response, so we exercise a successful path).
func TestV1Routes_ResponseHasXRequestID(t *testing.T) {
	cfg := testServerConfig("https://fake.supabase.co")
	q := &stubQuerier{
		returnCtx: middleware.TenantContext{
			OrgID:   uuid.New(),
			GroupID: uuid.New(),
			Role:    middleware.RoleMember,
		},
	}
	srv := server.New(cfg, q)

	token, _ := makeValidJWT(t)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/agents"},
		{http.MethodPost, "/v1/runs"},
	}

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			rid := res.Header.Get("X-Request-ID")
			if rid == "" {
				t.Errorf("AC7: X-Request-ID header is missing for %s %s", tc.method, tc.path)
				return
			}
			// Must be a valid UUID.
			if _, err := uuid.Parse(rid); err != nil {
				t.Errorf("AC7: X-Request-ID %q is not a valid UUID: %v", rid, err)
			}
		})
	}
}

// AC7 — also verify that a 401 response (no auth) still carries X-Request-ID.
func TestV1Routes_ResponseHasXRequestID_OnUnauthorized(t *testing.T) {
	cfg := testServerConfig("https://fake.supabase.co")
	q := &stubQuerier{returnErr: errNoMembership}
	srv := server.New(cfg, q)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents", nil)
	// No auth header — will 401.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	rid := res.Header.Get("X-Request-ID")
	if rid == "" {
		t.Error("AC7: X-Request-ID header must be present even on 401 responses")
		return
	}
	if _, err := uuid.Parse(rid); err != nil {
		t.Errorf("AC7: X-Request-ID %q on 401 is not a valid UUID: %v", rid, err)
	}
}

// ---------------------------------------------------------------------------
// AC8 — JSON log line per request has required slog fields
// ---------------------------------------------------------------------------

// TestLoggingMiddleware_JSONLogHasRequiredFields verifies AC8:
// each request produces a JSON log line containing level, msg, method, path,
// status, duration_ms, request_id.
func TestLoggingMiddleware_JSONLogHasRequiredFields(t *testing.T) {
	// Capture slog output into a buffer.
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(jsonHandler)

	// Build a minimal chain: RequestIDMiddleware → LoggingMiddleware → stub 200 handler.
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	chain := server.RequestIDMiddleware(server.LoggingMiddleware(logger, stub))

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	// Parse the captured log line.
	output := strings.TrimSpace(buf.String())
	if output == "" {
		t.Fatal("AC8: no log output was produced")
	}

	// There may be multiple lines; take the last one (the request-completed line).
	lines := strings.Split(output, "\n")
	lastLine := lines[len(lines)-1]

	var logEntry map[string]any
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
		t.Fatalf("AC8: log output is not valid JSON: %v — output: %s", err, lastLine)
	}

	required := []string{"level", "msg", "method", "path", "status", "duration_ms", "request_id"}
	for _, field := range required {
		if _, ok := logEntry[field]; !ok {
			t.Errorf("AC8: log line missing required field %q — entry: %s", field, lastLine)
		}
	}
}

// ---------------------------------------------------------------------------
// AC9 — supabaseQuerier.CurrentTenant sends POST to /rest/v1/rpc/current_tenant
//        with Authorization: Bearer <jwt> and apikey headers
// ---------------------------------------------------------------------------

// TestSupabaseQuerier_CurrentTenant_SendsCorrectHeaders verifies AC9 by
// standing up an httptest.Server that records the incoming request, then
// calling server.NewSupabaseQuerier(...).CurrentTenant(...).
func TestSupabaseQuerier_CurrentTenant_SendsCorrectHeaders(t *testing.T) {
	var (
		capturedMethod string
		capturedPath   string
		capturedAuth   string
		capturedAPIKey string
	)

	const fakeAnonKey = "test-anon-key-for-ac9"
	const fakeJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.stub.stub"

	// Fake Supabase server that returns a valid single-row membership response.
	stubSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		capturedAPIKey = r.Header.Get("apikey")

		// Return a minimal valid membership JSON array.
		orgID := uuid.New()
		groupID := uuid.New()
		resp := fmt.Sprintf(
			`[{"org_id":%q,"group_id":%q,"role":"group:member"}]`,
			orgID, groupID,
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, resp)
	}))
	defer stubSupabase.Close()

	q := server.NewSupabaseQuerier(stubSupabase.URL, fakeAnonKey)

	userID := uuid.New()
	// Inject the JWT into the context as auth.NewUserClient expects.
	ctx := auth.ContextWithJWT(context.Background(), fakeJWT)

	_, err := q.CurrentTenant(ctx, userID)
	if err != nil {
		t.Fatalf("AC9: CurrentTenant returned unexpected error: %v", err)
	}

	// Verify the HTTP call.
	wantPath := "/rest/v1/rpc/current_tenant"
	if capturedPath != wantPath {
		t.Errorf("AC9: POST path = %q, want %q", capturedPath, wantPath)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("AC9: HTTP method = %q, want POST", capturedMethod)
	}
	wantAuth := "Bearer " + fakeJWT
	if capturedAuth != wantAuth {
		t.Errorf("AC9: Authorization header = %q, want %q", capturedAuth, wantAuth)
	}
	if capturedAPIKey != fakeAnonKey {
		t.Errorf("AC9: apikey header = %q, want %q", capturedAPIKey, fakeAnonKey)
	}
}

// ---------------------------------------------------------------------------
// AC9 — supabaseQuerier.CurrentTenant returns ErrNoMembership on empty array
// ---------------------------------------------------------------------------

// TestSupabaseQuerier_CurrentTenant_EmptyArray_ReturnsErrNoMembership verifies
// the ErrNoMembership sentinel is returned when the RPC returns [].
func TestSupabaseQuerier_CurrentTenant_EmptyArray_ReturnsErrNoMembership(t *testing.T) {
	stubSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "[]") // empty membership result
	}))
	defer stubSupabase.Close()

	q := server.NewSupabaseQuerier(stubSupabase.URL, "anon-key")
	ctx := auth.ContextWithJWT(context.Background(), "some.jwt.token")

	_, err := q.CurrentTenant(ctx, uuid.New())
	if err == nil {
		t.Fatal("AC9: expected ErrNoMembership, got nil")
	}
	if !errors.Is(err, middleware.ErrNoMembership) {
		t.Errorf("AC9: expected errors.Is(err, ErrNoMembership) = true, got err = %v", err)
	}
}

// ---------------------------------------------------------------------------
// AC10 — compile check
// Covered implicitly: if the packages above do not exist, this file does not
// compile and the test run fails. Every type reference below is a typed symbol:
//   server.Config, server.New, server.LoadConfig, server.Run,
//   server.RequestIDMiddleware, server.LoggingMiddleware,
//   server.NewSupabaseQuerier,
//   auth.AuthMiddleware, auth.Config, auth.ContextWithJWT,
//   middleware.TenantMiddleware, middleware.RequireRole,
//   middleware.TenantContext, middleware.RoleMember, middleware.RoleManager,
//   middleware.ErrNoMembership.
// ---------------------------------------------------------------------------
