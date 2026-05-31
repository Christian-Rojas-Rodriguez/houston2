// Package unit_test is the failing-first unit test suite for Task 0010
// (seed-fixture).
//
// TDD red phase: these tests reference types in tests/helpers that do not
// exist yet. The file will not compile until Coder creates:
//   - tests/helpers/fixture.go  (FixtureIDs, Seed, Teardown, UserJWT)
//
// HTTP calls are intercepted via httptest.Server so no live Supabase is
// required for these unit tests.
//
// AC mapping
// ----------
// AC6  — TestFixtureIDs_AllFieldsNonZeroAndMatchConstants
// AC6b — TestSeed_ReturnsFixtureIDsMatchingConstants      (HTTP-mocked Seed)
// AC7  — TestSeed_RegistersCleanupTeardown
// AC8  — TestUserJWT_ReturnsNonEmptyJWTWithCorrectSub
// AC9  — TestUserJWT_UnknownUser_CallsFatal

package unit_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/tests/helpers"
)

// ---------------------------------------------------------------------------
// AC6 — FixtureIDs constants are all non-zero and unique
// ---------------------------------------------------------------------------

// TestFixtureIDs_AllFieldsNonZeroAndMatchConstants verifies that the exported
// UUID constants declared in the helpers package are non-zero and that a
// FixtureIDs struct built from them has the exact spec-mandated values.
//
// Spec AC6: helpers.Seed returns a FixtureIDs with all fields non-zero whose
// values coincide with the package UUID constants.
func TestFixtureIDs_AllFieldsNonZeroAndMatchConstants(t *testing.T) {
	t.Parallel()

	zero := uuid.UUID{}
	fields := map[string]uuid.UUID{
		"OrgID1":     helpers.OrgID1,
		"OrgID2":     helpers.OrgID2,
		"GroupID1G1": helpers.GroupID1G1,
		"GroupID1G2": helpers.GroupID1G2,
		"GroupID2G1": helpers.GroupID2G1,
		"UserIDA":    helpers.UserIDA,
		"UserIDB":    helpers.UserIDB,
		"UserIDC":    helpers.UserIDC,
		"AgentID1":   helpers.AgentID1,
		"AgentID2":   helpers.AgentID2,
	}

	for name, id := range fields {
		if id == zero {
			t.Errorf("helpers.%s is zero UUID — must be non-zero", name)
		}
	}

	// Verify specific expected values match spec §How UUIDs
	cases := []struct {
		name string
		got  uuid.UUID
		want string
	}{
		{"OrgID1", helpers.OrgID1, "00000010-0000-0000-0000-000000000001"},
		{"OrgID2", helpers.OrgID2, "00000010-0000-0000-0000-000000000002"},
		{"GroupID1G1", helpers.GroupID1G1, "00000010-0000-0001-0000-000000000001"},
		{"GroupID1G2", helpers.GroupID1G2, "00000010-0000-0001-0000-000000000002"},
		{"GroupID2G1", helpers.GroupID2G1, "00000010-0000-0002-0000-000000000001"},
		{"UserIDA", helpers.UserIDA, "00000010-0000-0000-0001-000000000001"},
		{"UserIDB", helpers.UserIDB, "00000010-0000-0000-0001-000000000002"},
		{"UserIDC", helpers.UserIDC, "00000010-0000-0000-0001-000000000003"},
		{"AgentID1", helpers.AgentID1, "00000010-0000-0000-0002-000000000001"},
		{"AgentID2", helpers.AgentID2, "00000010-0000-0000-0002-000000000002"},
	}
	for _, c := range cases {
		want := uuid.MustParse(c.want)
		if c.got != want {
			t.Errorf("helpers.%s = %v, want %v", c.name, c.got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Shared mock server builder
// ---------------------------------------------------------------------------

// newMockSupabase builds an httptest.Server that responds to all Supabase
// REST/Auth/Storage endpoints with minimal happy-path responses so that
// Seed/Teardown/UserJWT can complete without a real Supabase instance.
func newMockSupabase(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Auth Admin API — create user (POST /auth/v1/admin/users)
	// Auth Admin API — delete user (DELETE /auth/v1/admin/users/<id>)
	mux.HandleFunc("/auth/v1/admin/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"00000010-0000-0000-0001-000000000001","email":"user-a@houston-fixture.test"}`)
	})
	mux.HandleFunc("/auth/v1/admin/users/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})

	// Password sign-in — used by UserJWT
	mux.HandleFunc("/auth/v1/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("grant_type") == "password" {
			// Pre-encoded JWT: header.payload.sig
			// Payload: {"sub":"00000010-0000-0000-0001-000000000001"}
			token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
				"eyJzdWIiOiIwMDAwMDAxMC0wMDAwLTAwMDAtMDAwMS0wMDAwMDAwMDAwMDEifQ." +
				"SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
			resp := map[string]string{"access_token": token}
			b, _ := json.Marshal(resp)
			w.WriteHeader(http.StatusOK)
			w.Write(b)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"bad_request"}`)
		}
	})

	// PostgREST tables
	for _, table := range []string{"organizations", "groups", "memberships", "agents"} {
		table := table
		mux.HandleFunc("/rest/v1/"+table, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodPost:
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, `[]`)
			case http.MethodDelete:
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `[]`)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		})
	}

	// Storage — upload (POST) and delete (DELETE) objects
	mux.HandleFunc("/storage/v1/object/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"Key":"houston/fixture"}`)
	})

	return httptest.NewServer(mux)
}

// ---------------------------------------------------------------------------
// AC6b — Seed (HTTP-mocked) returns FixtureIDs matching constants
// ---------------------------------------------------------------------------

// TestSeed_ReturnsFixtureIDsMatchingConstants verifies that helpers.Seed
// returns a FixtureIDs struct whose fields equal the package-level UUID
// constants. Uses a mock HTTP server so no live Supabase is required.
//
// Spec AC6: helpers.Seed(t, url, key) returns a FixtureIDs with all fields
// non-zero whose values coincide with the constants.
func TestSeed_ReturnsFixtureIDsMatchingConstants(t *testing.T) {
	srv := newMockSupabase(t)
	defer srv.Close()

	ids := helpers.Seed(t, srv.URL, "fake-service-role-key")

	checks := []struct {
		name string
		got  uuid.UUID
		want uuid.UUID
	}{
		{"OrgID1", ids.OrgID1, helpers.OrgID1},
		{"OrgID2", ids.OrgID2, helpers.OrgID2},
		{"GroupID1G1", ids.GroupID1G1, helpers.GroupID1G1},
		{"GroupID1G2", ids.GroupID1G2, helpers.GroupID1G2},
		{"GroupID2G1", ids.GroupID2G1, helpers.GroupID2G1},
		{"UserIDA", ids.UserIDA, helpers.UserIDA},
		{"UserIDB", ids.UserIDB, helpers.UserIDB},
		{"UserIDC", ids.UserIDC, helpers.UserIDC},
		{"AgentID1", ids.AgentID1, helpers.AgentID1},
		{"AgentID2", ids.AgentID2, helpers.AgentID2},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("FixtureIDs.%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// AC7 — Seed registers t.Cleanup that calls Teardown
// ---------------------------------------------------------------------------

// TestSeed_RegistersCleanupTeardown verifies that after helpers.Seed is called,
// a cleanup function is registered that triggers Teardown DELETE requests after
// the test completes.
//
// Spec AC7: helpers.Seed(t, url, key) registers a t.Cleanup to call Teardown.
// After the test, fixture entities must not exist in Supabase.
func TestSeed_RegistersCleanupTeardown(t *testing.T) {
	deleteCalled := false

	mux := http.NewServeMux()

	mux.HandleFunc("/auth/v1/admin/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"00000010-0000-0000-0001-000000000001"}`)
	})
	mux.HandleFunc("/auth/v1/admin/users/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})

	for _, table := range []string{"organizations", "groups", "memberships", "agents"} {
		table := table
		mux.HandleFunc("/rest/v1/"+table, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodDelete {
				deleteCalled = true
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `[]`)
			} else {
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, `[]`)
			}
		})
	}

	mux.HandleFunc("/storage/v1/object/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"Key":"ok"}`)
	})

	srv := httptest.NewServer(mux)

	// Run Seed in an inner sub-test so its t.Cleanup fires before we check.
	t.Run("inner", func(t *testing.T) {
		helpers.Seed(t, srv.URL, "fake-key")
		// Teardown must NOT fire during the test — only in cleanup.
		if deleteCalled {
			t.Error("Teardown fired before test cleanup — must be deferred via t.Cleanup")
		}
	})

	// After inner sub-test finishes, its t.Cleanup callbacks have run.
	srv.Close()

	if !deleteCalled {
		t.Error("t.Cleanup did not trigger Teardown: no DELETE request received after test")
	}
}

// ---------------------------------------------------------------------------
// AC8 — UserJWT returns non-empty JWT with correct sub
// ---------------------------------------------------------------------------

// TestUserJWT_ReturnsNonEmptyJWTWithCorrectSub verifies that helpers.UserJWT
// returns a non-empty string and that the JWT payload's "sub" field equals
// the requested userID.
//
// Spec AC8: helpers.UserJWT(t, url, key, UserIDA) returns a string that, when
// decoded as JWT, has sub == UserIDA.String().
func TestUserJWT_ReturnsNonEmptyJWTWithCorrectSub(t *testing.T) {
	t.Parallel()

	expectedSub := helpers.UserIDA.String() // "00000010-0000-0000-0001-000000000001"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/token" && r.URL.Query().Get("grant_type") == "password" {
			// Return a JWT whose payload has sub = UserIDA.String()
			// Payload (base64url): {"sub":"00000010-0000-0000-0001-000000000001"}
			token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
				"eyJzdWIiOiIwMDAwMDAxMC0wMDAwLTAwMDAtMDAwMS0wMDAwMDAwMDAwMDEifQ." +
				"SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
			resp := map[string]string{"access_token": token}
			b, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(b)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got := helpers.UserJWT(t, srv.URL, "fake-key", helpers.UserIDA)

	if got == "" {
		t.Fatal("UserJWT returned empty string")
	}

	sub := extractJWTSub(t, got)
	if sub != expectedSub {
		t.Errorf("JWT sub = %q, want %q", sub, expectedSub)
	}
}

// extractJWTSub decodes the payload section of a JWT (without verifying
// signature) and returns the "sub" string claim.
func extractJWTSub(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT does not have 3 parts: %q", token)
	}

	// base64url → base64 std, then add padding
	payload := parts[1]
	payload = strings.ReplaceAll(payload, "-", "+")
	payload = strings.ReplaceAll(payload, "_", "/")
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("base64 decode JWT payload: %v", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		t.Fatalf("unmarshal JWT claims: %v", err)
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		t.Fatalf("JWT has no 'sub' string claim: %v", claims)
	}
	return sub
}

// ---------------------------------------------------------------------------
// AC9 — UserJWT with unknown userID calls t.Fatal
// ---------------------------------------------------------------------------

// TestUserJWT_UnknownUser_CallsFatal verifies that helpers.UserJWT calls
// t.Fatal when Supabase returns an error for the password grant (e.g. invalid
// credentials / user not found), rather than silently returning an empty token.
//
// Spec AC9: helpers.UserJWT with a userID that does not exist calls t.Fatal.
func TestUserJWT_UnknownUser_CallsFatal(t *testing.T) {
	t.Parallel()

	// Mock that always returns 400 for any password grant.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"Invalid login credentials"}`)
	}))
	defer srv.Close()

	unknownID := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")

	fatalCalled := false
	mockT := &fatalCapture{TB: t, onFatal: func() { fatalCalled = true }}

	// helpers.UserJWT must call t.Fatal; because t.Fatal calls runtime.Goexit,
	// we capture the panic emitted by fatalCapture in a goroutine.
	done := make(chan struct{})
	go func() {
		defer func() {
			// Recover the fatalSignal panic so the goroutine exits cleanly.
			if r := recover(); r != nil {
				if _, ok := r.(fatalSignal); !ok {
					panic(r) // re-raise unexpected panics
				}
			}
			close(done)
		}()
		helpers.UserJWT(mockT, srv.URL, "fake-key", unknownID)
	}()
	<-done

	if !fatalCalled {
		t.Error("UserJWT did not call t.Fatal for unknown user — expected a fatal error")
	}
}

// fatalCapture wraps testing.TB and captures Fatal/Fatalf calls.
type fatalCapture struct {
	testing.TB
	onFatal func()
}

func (f *fatalCapture) Fatal(args ...interface{}) {
	f.onFatal()
	panic(fatalSignal{})
}

func (f *fatalCapture) Fatalf(format string, args ...interface{}) {
	f.onFatal()
	panic(fatalSignal{})
}

func (f *fatalCapture) Helper() {}

// fatalSignal is the panic value used by fatalCapture to simulate runtime.Goexit.
type fatalSignal struct{}
