//go:build integration

// Package integration_test is the failing-first integration test suite for
// Task 0010 (seed-fixture).
//
// These tests require a live Supabase instance. Run with:
//
//	SUPABASE_URL=https://<ref>.supabase.co \
//	SUPABASE_SERVICE_ROLE_KEY=<key> \
//	go test -tags integration ./tests/integration/ -run 0010 -v
//
// TDD red phase: these tests reference helpers.Seed / helpers.Teardown /
// helpers.UserJWT which do not exist until Coder creates
// tests/helpers/fixture.go, and they also invoke scripts/seed.go via
// os/exec which does not exist until Coder creates it.
//
// AC mapping
// ----------
// AC1  — TestSeedScript_ExitZeroAndPrintsConfirmation
// AC2  — TestSeedScript_IdempotentSecondRun
// AC3  — TestSeedScript_TeardownExitZeroAndDeletesOrgs
// AC4  — TestSeed_OrgsExistAfterSeed
// AC5  — TestSeed_MembershipsExistWithCorrectRoles

package integration_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/nomenclator/houston2/tests/helpers"
)

// supabaseEnv reads SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY from the
// environment, skipping the test if either is unset.
func supabaseEnv(t *testing.T) (supabaseURL, serviceRoleKey string) {
	t.Helper()
	supabaseURL = os.Getenv("SUPABASE_URL")
	serviceRoleKey = os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if supabaseURL == "" || serviceRoleKey == "" {
		t.Skip("SUPABASE_URL or SUPABASE_SERVICE_ROLE_KEY not set — skipping integration test")
	}
	return
}

// ---------------------------------------------------------------------------
// AC1 — go run scripts/seed.go exits 0 and prints per-resource confirmation
// ---------------------------------------------------------------------------

// TestSeedScript_ExitZeroAndPrintsConfirmation verifies that running the seed
// script with valid credentials terminates with exit code 0 and emits at least
// one confirmation line per resource category: org, group, user, membership,
// agent, storage object.
//
// Spec AC1: go run scripts/seed.go with valid env vars exits 0 and prints
// confirmation of each resource type inserted.
func TestSeedScript_ExitZeroAndPrintsConfirmation(t *testing.T) {
	supabaseURL, serviceRoleKey := supabaseEnv(t)

	// First run a teardown to start clean (idempotent — OK if nothing exists).
	runSeedScript(t, supabaseURL, serviceRoleKey, "-teardown")

	out := runSeedScript(t, supabaseURL, serviceRoleKey)

	// Expect at least one confirmation line mentioning each resource category.
	categories := []string{"org", "group", "user", "membership", "agent", "storage"}
	for _, cat := range categories {
		if !strings.Contains(strings.ToLower(out), cat) {
			t.Errorf("seed output missing confirmation for %q category.\nFull output:\n%s", cat, out)
		}
	}

	// Cleanup after test.
	t.Cleanup(func() {
		runSeedScript(t, supabaseURL, serviceRoleKey, "-teardown")
	})
}

// ---------------------------------------------------------------------------
// AC2 — go run scripts/seed.go is idempotent on second run
// ---------------------------------------------------------------------------

// TestSeedScript_IdempotentSecondRun verifies that running the seed script
// twice in a row exits 0 both times without any duplicate-key errors.
//
// Spec AC2: running seed.go a second time exits 0 without duplicate-key errors.
func TestSeedScript_IdempotentSecondRun(t *testing.T) {
	supabaseURL, serviceRoleKey := supabaseEnv(t)

	// Start clean.
	runSeedScript(t, supabaseURL, serviceRoleKey, "-teardown")

	// First run.
	runSeedScript(t, supabaseURL, serviceRoleKey)

	// Second run — must also succeed.
	out := runSeedScript(t, supabaseURL, serviceRoleKey)

	// No error keywords expected.
	lower := strings.ToLower(out)
	for _, errKw := range []string{"duplicate", "conflict", "error", "failed"} {
		if strings.Contains(lower, errKw) {
			t.Errorf("second seed run output contains %q — not idempotent.\nFull output:\n%s", errKw, out)
		}
	}

	t.Cleanup(func() {
		runSeedScript(t, supabaseURL, serviceRoleKey, "-teardown")
	})
}

// ---------------------------------------------------------------------------
// AC3 — go run scripts/seed.go -teardown removes fixture orgs
// ---------------------------------------------------------------------------

// TestSeedScript_TeardownExitZeroAndDeletesOrgs verifies that -teardown exits
// 0 and that a subsequent GET /rest/v1/organizations with service_role does not
// return OrgID1 or OrgID2.
//
// Spec AC3: -teardown exits 0 and removes all fixture entities. A subsequent
// service_role query to organizations does not return the fixture orgs.
func TestSeedScript_TeardownExitZeroAndDeletesOrgs(t *testing.T) {
	supabaseURL, serviceRoleKey := supabaseEnv(t)

	// Seed first so teardown has something to delete.
	runSeedScript(t, supabaseURL, serviceRoleKey)

	// Teardown.
	runSeedScript(t, supabaseURL, serviceRoleKey, "-teardown")

	// Query organizations with service_role — fixture orgs must be gone.
	orgs := queryOrgs(t, supabaseURL, serviceRoleKey)

	for _, org := range orgs {
		id, _ := org["id"].(string)
		if id == helpers.OrgID1.String() || id == helpers.OrgID2.String() {
			t.Errorf("org %s still present after teardown — expected deletion", id)
		}
	}
}

// ---------------------------------------------------------------------------
// AC4 — After seed, service_role query returns both fixture orgs
// ---------------------------------------------------------------------------

// TestSeed_OrgsExistAfterSeed verifies that after helpers.Seed completes,
// a GET /rest/v1/organizations with service_role returns at least the two
// fixture orgs identified by OrgID1 and OrgID2.
//
// Spec AC4: after seed, GET /rest/v1/organizations with service_role returns
// at least OrgID1 and OrgID2.
func TestSeed_OrgsExistAfterSeed(t *testing.T) {
	supabaseURL, serviceRoleKey := supabaseEnv(t)

	helpers.Seed(t, supabaseURL, serviceRoleKey)
	// t.Cleanup teardown registered by Seed — no manual cleanup needed.

	orgs := queryOrgs(t, supabaseURL, serviceRoleKey)

	found := map[string]bool{}
	for _, org := range orgs {
		id, _ := org["id"].(string)
		found[id] = true
	}

	if !found[helpers.OrgID1.String()] {
		t.Errorf("OrgID1 (%s) not found in organizations after seed", helpers.OrgID1)
	}
	if !found[helpers.OrgID2.String()] {
		t.Errorf("OrgID2 (%s) not found in organizations after seed", helpers.OrgID2)
	}
}

// ---------------------------------------------------------------------------
// AC5 — After seed, memberships have correct roles
// ---------------------------------------------------------------------------

// TestSeed_MembershipsExistWithCorrectRoles verifies that after helpers.Seed
// completes, a GET /rest/v1/memberships with service_role returns exactly 3
// membership rows for the fixture user IDs, each with the correct role.
//
// Spec AC5: after seed, GET /rest/v1/memberships with service_role returns
// exactly 3 rows for fixture users with roles group:member, group:manager,
// org:owner.
func TestSeed_MembershipsExistWithCorrectRoles(t *testing.T) {
	supabaseURL, serviceRoleKey := supabaseEnv(t)

	helpers.Seed(t, supabaseURL, serviceRoleKey)

	memberships := queryMemberships(t, supabaseURL, serviceRoleKey)

	// Build a map user_id → role for the fixture users only.
	fixtureUserIDs := map[string]string{
		helpers.UserIDA.String(): "",
		helpers.UserIDB.String(): "",
		helpers.UserIDC.String(): "",
	}
	fixtureCount := 0
	for _, m := range memberships {
		uid, _ := m["user_id"].(string)
		if _, ok := fixtureUserIDs[uid]; ok {
			role, _ := m["role"].(string)
			fixtureUserIDs[uid] = role
			fixtureCount++
		}
	}

	if fixtureCount != 3 {
		t.Errorf("expected 3 fixture membership rows, got %d", fixtureCount)
	}

	expectedRoles := map[string]string{
		helpers.UserIDA.String(): "group:member",
		helpers.UserIDB.String(): "group:manager",
		helpers.UserIDC.String(): "org:owner",
	}
	for uid, wantRole := range expectedRoles {
		gotRole := fixtureUserIDs[uid]
		if gotRole != wantRole {
			t.Errorf("user %s: role = %q, want %q", uid, gotRole, wantRole)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// runSeedScript executes `go run scripts/seed.go [args...]` with the given
// Supabase credentials, fails the test if the exit code is non-zero, and
// returns the combined stdout+stderr output.
func runSeedScript(t *testing.T, supabaseURL, serviceRoleKey string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"run", "./scripts/seed.go"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Env = append(os.Environ(),
		"SUPABASE_URL="+supabaseURL,
		"SUPABASE_SERVICE_ROLE_KEY="+serviceRoleKey,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed script failed (args=%v):\n%s\nerror: %v", args, out, err)
	}
	return string(out)
}

// queryOrgs performs GET /rest/v1/organizations with service_role and returns
// the parsed JSON rows.
func queryOrgs(t *testing.T, supabaseURL, serviceRoleKey string) []map[string]interface{} {
	t.Helper()
	return queryTable(t, supabaseURL, serviceRoleKey, "organizations")
}

// queryMemberships performs GET /rest/v1/memberships with service_role.
func queryMemberships(t *testing.T, supabaseURL, serviceRoleKey string) []map[string]interface{} {
	t.Helper()
	return queryTable(t, supabaseURL, serviceRoleKey, "memberships")
}

// queryTable performs a GET /rest/v1/<table> with service_role authorization
// and returns the parsed JSON array.
func queryTable(t *testing.T, supabaseURL, serviceRoleKey, table string) []map[string]interface{} {
	t.Helper()

	url := fmt.Sprintf("%s/rest/v1/%s", supabaseURL, table)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s request: %v", table, err)
	}
	req.Header.Set("apikey", serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /rest/v1/%s: %v", table, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("GET /rest/v1/%s returned %d: %s", table, resp.StatusCode, body)
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal %s response: %v\nbody: %s", table, err, body)
	}
	return rows
}
