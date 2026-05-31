// Package middleware_test is the failing-first test suite for Task 0005 (rbac-middleware).
//
// TDD red phase: these tests reference github.com/nomenclator/houston2/internal/middleware
// and github.com/nomenclator/houston2/internal/auth, neither of which exists yet.
// The file will fail to compile until Coder creates the implementation. Every test
// is expected to FAIL (or fail to compile) until then.
//
// Run with:
//   go test ./tests/unit/... -run TestRBAC -v
//
// No network calls, no Supabase credentials, no environment variables required.
//
// AC mapping
// ----------
// AC1  — TestRBAC_TenantMiddleware_NoUserID_Returns401
// AC2  — TestRBAC_TenantMiddleware_NoMembership_Returns403
// AC3  — TestRBAC_TenantMiddleware_ValidUser_TenantContextPropagated
// AC4  — TestRBAC_RequireRole_BelowMinRole_Returns403
// AC5  — TestRBAC_RequireRole_ExactMinRole_CallsDownstream
// AC6  — TestRBAC_RequireRole_AboveMinRole_CallsDownstream
// AC7  — TestRBAC_RequireRole_WithoutTenantMiddleware_Returns500
// AC8  — TestRBAC_ParseRole
// AC9  — covered by stub TenantQuerier and all test helpers (no network, no env vars)
// AC10 — covered by compilation: every test references typed symbols from internal/middleware

package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/middleware"
)

// ---------------------------------------------------------------------------
// Stub TenantQuerier
// ---------------------------------------------------------------------------

// stubQuerier is an in-memory implementation of middleware.TenantQuerier that
// is fully controlled by each test — no network, no Supabase.
type stubQuerier struct {
	// result is returned when err is nil.
	result middleware.TenantContext
	// err is returned verbatim from CurrentTenant; use middleware.ErrNoMembership
	// to simulate a missing membership row.
	err error
}

func (s *stubQuerier) CurrentTenant(_ context.Context, _ uuid.UUID) (middleware.TenantContext, error) {
	return s.result, s.err
}

// ---------------------------------------------------------------------------
// Shared test fixtures
// ---------------------------------------------------------------------------

const testJWTSecret = "super-secret-for-testing-only-rbac"

// makeTestJWT creates a valid HS256-signed JWT whose sub is userID.String().
func makeTestJWT(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"exp": jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		"iat": jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("makeTestJWT: %v", err)
	}
	return signed
}

// testAuthCfg returns an auth.Config wired to the test JWT secret.
func testAuthCfg() auth.Config {
	return auth.Config{
		SupabaseURL: "https://fake.supabase.co",
		AnonKey:     "test-anon-key",
		JWTSecret:   testJWTSecret,
		Port:        "8080",
	}
}

// ctxWithUserID returns a context.Context that has been enriched with the given
// userID by running a valid JWT through auth.AuthMiddleware. This is the
// canonical way to seed user_id into context without touching the unexported
// context key — exactly what would happen in production when AuthMiddleware runs
// before TenantMiddleware.
func ctxWithUserID(t *testing.T, userID uuid.UUID) context.Context {
	t.Helper()
	token := makeTestJWT(t, userID)
	cfg := testAuthCfg()

	var captured context.Context
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r.Context()
	})
	mw := auth.AuthMiddleware(cfg)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.ServeHTTP(httptest.NewRecorder(), req)

	if captured == nil {
		t.Fatal("ctxWithUserID: inner handler was not called — AuthMiddleware rejected a valid JWT")
	}
	return captured
}

// ---------------------------------------------------------------------------
// Error envelope assertion helpers
// ---------------------------------------------------------------------------

type errorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func assertErrorEnvelope(t *testing.T, prefix string, body io.Reader, wantCode string, wantStatus int, gotStatus int) {
	t.Helper()
	if gotStatus != wantStatus {
		t.Errorf("%s: expected HTTP %d, got %d", prefix, wantStatus, gotStatus)
	}
	raw, _ := io.ReadAll(body)
	var env errorEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Errorf("%s: response body is not valid JSON: %v — body: %s", prefix, err, raw)
		return
	}
	if env.Error.Code != wantCode {
		t.Errorf("%s: error.code = %q, want %q", prefix, env.Error.Code, wantCode)
	}
	if env.Error.Message == "" {
		t.Errorf("%s: error.message must not be empty", prefix)
	}
	if env.Error.RequestID == "" {
		t.Errorf("%s: error.request_id must not be empty", prefix)
	}
}

// ---------------------------------------------------------------------------
// AC1 — TenantMiddleware: no user_id in context → 401 unauthenticated
// ---------------------------------------------------------------------------

// TestRBAC_TenantMiddleware_NoUserID_Returns401 verifies acceptance criterion 1:
// when no user_id is present in the request context (auth.UserIDFromContext
// returns false), TenantMiddleware responds 401 with the unauthenticated
// error envelope and never calls the downstream handler.
func TestRBAC_TenantMiddleware_NoUserID_Returns401(t *testing.T) {
	q := &stubQuerier{} // will never be called
	mw := middleware.TenantMiddleware(q)

	downstreamCalled := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(downstream)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	// Deliberately NOT setting Authorization header — context has no user_id.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if downstreamCalled {
		t.Error("AC1: downstream handler must NOT be called when no user_id is in context")
	}
	assertErrorEnvelope(t, "AC1", res.Body, "unauthenticated", http.StatusUnauthorized, res.StatusCode)
}

// ---------------------------------------------------------------------------
// AC2 — TenantMiddleware: ErrNoMembership → 403 forbidden
// ---------------------------------------------------------------------------

// TestRBAC_TenantMiddleware_NoMembership_Returns403 verifies acceptance criterion 2:
// when the user_id is present but CurrentTenant returns ErrNoMembership, the
// middleware responds 403 with the forbidden error envelope and never calls
// the downstream handler.
func TestRBAC_TenantMiddleware_NoMembership_Returns403(t *testing.T) {
	userID := uuid.New()
	q := &stubQuerier{err: middleware.ErrNoMembership}
	mw := middleware.TenantMiddleware(q)

	downstreamCalled := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(downstream)

	// Seed user_id into context via AuthMiddleware (same path as production).
	seededCtx := ctxWithUserID(t, userID)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil).WithContext(seededCtx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if downstreamCalled {
		t.Error("AC2: downstream handler must NOT be called when CurrentTenant returns ErrNoMembership")
	}
	assertErrorEnvelope(t, "AC2", res.Body, "forbidden", http.StatusForbidden, res.StatusCode)
}

// ---------------------------------------------------------------------------
// AC3 — TenantMiddleware: valid user → TenantContext propagated to downstream
// ---------------------------------------------------------------------------

// TestRBAC_TenantMiddleware_ValidUser_TenantContextPropagated verifies
// acceptance criterion 3: when user_id is present and CurrentTenant returns a
// valid TenantContext, TenantFromContext called inside the downstream handler
// returns the exact same struct with ok == true.
func TestRBAC_TenantMiddleware_ValidUser_TenantContextPropagated(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()
	groupID := uuid.New()

	want := middleware.TenantContext{
		OrgID:   orgID,
		GroupID: groupID,
		Role:    middleware.RoleManager,
	}

	q := &stubQuerier{result: want}
	mw := middleware.TenantMiddleware(q)

	var got middleware.TenantContext
	var gotOK bool
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, gotOK = middleware.TenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(downstream)

	seededCtx := ctxWithUserID(t, userID)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil).WithContext(seededCtx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !gotOK {
		t.Fatal("AC3: TenantFromContext returned ok=false inside downstream handler")
	}
	if got.OrgID != want.OrgID {
		t.Errorf("AC3: OrgID = %v, want %v", got.OrgID, want.OrgID)
	}
	if got.GroupID != want.GroupID {
		t.Errorf("AC3: GroupID = %v, want %v", got.GroupID, want.GroupID)
	}
	if got.Role != want.Role {
		t.Errorf("AC3: Role = %v, want %v", got.Role, want.Role)
	}
}

// ---------------------------------------------------------------------------
// AC4 — Chain TenantMiddleware→RequireRole(RoleManager): RoleMember → 403
// ---------------------------------------------------------------------------

// TestRBAC_RequireRole_BelowMinRole_Returns403 verifies acceptance criterion 4:
// when the stored TenantContext.Role is RoleMember and the chain requires
// RoleManager, the request is rejected with 403 and downstream is not called.
func TestRBAC_RequireRole_BelowMinRole_Returns403(t *testing.T) {
	userID := uuid.New()
	q := &stubQuerier{result: middleware.TenantContext{
		OrgID:   uuid.New(),
		GroupID: uuid.New(),
		Role:    middleware.RoleMember,
	}}

	downstreamCalled := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Chain: TenantMiddleware → RequireRole(RoleManager) → downstream
	chain := middleware.TenantMiddleware(q)(
		middleware.RequireRole(middleware.RoleManager)(downstream),
	)

	seededCtx := ctxWithUserID(t, userID)
	req := httptest.NewRequest(http.MethodGet, "/manager-only", nil).WithContext(seededCtx)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if downstreamCalled {
		t.Error("AC4: downstream must NOT be called when role is below minimum")
	}
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("AC4: expected 403, got %d", res.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// AC5 — Chain: TenantContext{RoleManager} → downstream called (200)
// ---------------------------------------------------------------------------

// TestRBAC_RequireRole_ExactMinRole_CallsDownstream verifies acceptance
// criterion 5: when the caller's role equals the required minimum (RoleManager),
// RequireRole passes through and downstream is called.
func TestRBAC_RequireRole_ExactMinRole_CallsDownstream(t *testing.T) {
	userID := uuid.New()
	q := &stubQuerier{result: middleware.TenantContext{
		OrgID:   uuid.New(),
		GroupID: uuid.New(),
		Role:    middleware.RoleManager,
	}}

	downstreamCalled := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	chain := middleware.TenantMiddleware(q)(
		middleware.RequireRole(middleware.RoleManager)(downstream),
	)

	seededCtx := ctxWithUserID(t, userID)
	req := httptest.NewRequest(http.MethodGet, "/manager-only", nil).WithContext(seededCtx)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if !downstreamCalled {
		t.Error("AC5: downstream must be called when role == RoleManager and min is RoleManager")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("AC5: expected 200 from downstream, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// AC6 — Chain: TenantContext{RoleOwner} → downstream called (200)
// ---------------------------------------------------------------------------

// TestRBAC_RequireRole_AboveMinRole_CallsDownstream verifies acceptance
// criterion 6: when the caller's role (RoleOwner) exceeds the required minimum
// (RoleManager), RequireRole passes through and downstream is called.
func TestRBAC_RequireRole_AboveMinRole_CallsDownstream(t *testing.T) {
	userID := uuid.New()
	q := &stubQuerier{result: middleware.TenantContext{
		OrgID:   uuid.New(),
		GroupID: uuid.New(),
		Role:    middleware.RoleOwner,
	}}

	downstreamCalled := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	chain := middleware.TenantMiddleware(q)(
		middleware.RequireRole(middleware.RoleManager)(downstream),
	)

	seededCtx := ctxWithUserID(t, userID)
	req := httptest.NewRequest(http.MethodGet, "/manager-only", nil).WithContext(seededCtx)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if !downstreamCalled {
		t.Error("AC6: downstream must be called when role == RoleOwner and min is RoleManager")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("AC6: expected 200 from downstream, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// AC7 — RequireRole without prior TenantMiddleware → 500 (ordering guard)
// ---------------------------------------------------------------------------

// TestRBAC_RequireRole_WithoutTenantMiddleware_Returns500 verifies acceptance
// criterion 7: if RequireRole is used without a preceding TenantMiddleware
// (so no TenantContext is in the context), the middleware must respond 500,
// not 403, to signal a programmer ordering error rather than an authorization
// denial.
func TestRBAC_RequireRole_WithoutTenantMiddleware_Returns500(t *testing.T) {
	downstreamCalled := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// RequireRole used directly — no TenantMiddleware in the chain.
	handler := middleware.RequireRole(middleware.RoleMember)(downstream)

	// Plain context: no user_id, no TenantContext.
	req := httptest.NewRequest(http.MethodGet, "/any", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if downstreamCalled {
		t.Error("AC7: downstream must NOT be called when TenantContext is absent")
	}
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("AC7: expected 500 (ordering guard), got %d", res.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// AC8 — ParseRole: valid strings → typed Role constants; unknown → error
// ---------------------------------------------------------------------------

// TestRBAC_ParseRole verifies acceptance criterion 8:
//   - "org:owner"     → RoleOwner, nil
//   - "group:manager" → RoleManager, nil
//   - "group:member"  → RoleMember, nil
//   - "superadmin"    → _, non-nil error
func TestRBAC_ParseRole(t *testing.T) {
	cases := []struct {
		input   string
		want    middleware.Role
		wantErr bool
	}{
		{"org:owner", middleware.RoleOwner, false},
		{"group:manager", middleware.RoleManager, false},
		{"group:member", middleware.RoleMember, false},
		{"superadmin", 0, true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			got, err := middleware.ParseRole(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("AC8: ParseRole(%q) expected non-nil error, got nil (role=%v)", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("AC8: ParseRole(%q) unexpected error: %v", tc.input, err)
				return
			}
			if got != tc.want {
				t.Errorf("AC8: ParseRole(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AC8 extension — role ordering invariant
// ---------------------------------------------------------------------------

// TestRBAC_RoleOrdering verifies that the integer enum satisfies the expected
// ordering: RoleMember < RoleManager < RoleOwner.
// This is load-bearing for the >= comparison in RequireRole.
func TestRBAC_RoleOrdering(t *testing.T) {
	if !(middleware.RoleMember < middleware.RoleManager) {
		t.Errorf("RoleOrdering: RoleMember (%d) must be < RoleManager (%d)",
			middleware.RoleMember, middleware.RoleManager)
	}
	if !(middleware.RoleManager < middleware.RoleOwner) {
		t.Errorf("RoleOrdering: RoleManager (%d) must be < RoleOwner (%d)",
			middleware.RoleManager, middleware.RoleOwner)
	}
}

// ---------------------------------------------------------------------------
// AC2 extension — wrapped ErrNoMembership is also handled as 403
// ---------------------------------------------------------------------------

// TestRBAC_TenantMiddleware_WrappedNoMembership_Returns403 verifies that
// errors.Is semantics work: a wrapped ErrNoMembership is still treated as 403,
// not 500. This exercises the errors.Is(err, ErrNoMembership) check in the
// middleware (Spec §Design decisions).
func TestRBAC_TenantMiddleware_WrappedNoMembership_Returns403(t *testing.T) {
	userID := uuid.New()
	wrappedErr := errors.Join(errors.New("db: query failed"), middleware.ErrNoMembership)
	q := &stubQuerier{err: wrappedErr}
	mw := middleware.TenantMiddleware(q)

	downstreamCalled := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(downstream)

	seededCtx := ctxWithUserID(t, userID)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil).WithContext(seededCtx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if downstreamCalled {
		t.Error("AC2-ext: downstream must NOT be called for wrapped ErrNoMembership")
	}
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("AC2-ext: expected 403 for wrapped ErrNoMembership, got %d", res.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// AC3 extension — TenantFromContext on empty context returns ok == false
// ---------------------------------------------------------------------------

// TestRBAC_TenantFromContext_EmptyContext_ReturnsFalse verifies that
// TenantFromContext on a plain background context returns the zero value and
// ok == false — symmetric with auth.UserIDFromContext behaviour.
func TestRBAC_TenantFromContext_EmptyContext_ReturnsFalse(t *testing.T) {
	tc, ok := middleware.TenantFromContext(context.Background())
	if ok {
		t.Error("AC3-ext: TenantFromContext(Background()) must return ok=false")
	}
	var zero middleware.TenantContext
	if tc != zero {
		t.Errorf("AC3-ext: TenantFromContext(Background()) must return zero TenantContext, got %+v", tc)
	}
}
