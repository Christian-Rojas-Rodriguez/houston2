// Package helpers provides the canonical fixture for Houston MVP acceptance tests.
//
// The fixture creates a deterministic Supabase state: 2 orgs, 3 groups, 3 users,
// 3 memberships, 2 agents, and 2 Storage objects. All entity IDs are fixed
// constants so cross-test assertions (e.g. "org_id must not leak to user A")
// remain stable across runs.
//
// Usage in tests:
//
//	func TestMyFeature(t *testing.T) {
//	    ids := helpers.Seed(t, os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_SERVICE_ROLE_KEY"))
//	    jwt := helpers.UserJWT(t, os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_SERVICE_ROLE_KEY"), ids.UserIDA)
//	    // ... test assertions ...
//	    // Teardown is called automatically via t.Cleanup.
//	}
package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Deterministic UUID constants — spec-mandated, never change without version bump
// ---------------------------------------------------------------------------

var (
	OrgID1     = uuid.MustParse("00000010-0000-0000-0000-000000000001")
	OrgID2     = uuid.MustParse("00000010-0000-0000-0000-000000000002")
	GroupID1G1 = uuid.MustParse("00000010-0000-0001-0000-000000000001") // Org1 / Group1 (general)
	GroupID1G2 = uuid.MustParse("00000010-0000-0001-0000-000000000002") // Org1 / Group2
	GroupID2G1 = uuid.MustParse("00000010-0000-0002-0000-000000000001") // Org2 / Group1 (general)
	UserIDA    = uuid.MustParse("00000010-0000-0000-0001-000000000001") // group:member in Org1/G1
	UserIDB    = uuid.MustParse("00000010-0000-0000-0001-000000000002") // group:manager in Org1/G2
	UserIDC    = uuid.MustParse("00000010-0000-0000-0001-000000000003") // org:owner in Org2/G1
	AgentID1   = uuid.MustParse("00000010-0000-0000-0002-000000000001") // agent in Org1/G1
	AgentID2   = uuid.MustParse("00000010-0000-0000-0002-000000000002") // agent in Org2/G1
)

// Fixture user credentials — email/password pairs used by UserJWT.
const (
	emailUserA    = "user-a@houston-fixture.test"
	emailUserB    = "user-b@houston-fixture.test"
	emailUserC    = "user-c@houston-fixture.test"
	passwordUserA = "houston-fixture-pass-A"
	passwordUserB = "houston-fixture-pass-B"
	passwordUserC = "houston-fixture-pass-C"
)

// ---------------------------------------------------------------------------
// FixtureIDs — returned by Seed, contains all deterministic fixture UUIDs
// ---------------------------------------------------------------------------

// FixtureIDs contains all UUIDs of the MVP fixture entities.
// All fields are non-zero and match the package-level UUID constants.
type FixtureIDs struct {
	OrgID1     uuid.UUID
	OrgID2     uuid.UUID
	GroupID1G1 uuid.UUID // Org1 / Group 1 (is_general=true)
	GroupID1G2 uuid.UUID // Org1 / Group 2
	GroupID2G1 uuid.UUID // Org2 / Group 1 (is_general=true)
	UserIDA    uuid.UUID // group:member in Org1/G1
	UserIDB    uuid.UUID // group:manager in Org1/G2
	UserIDC    uuid.UUID // org:owner in Org2/G1
	AgentID1   uuid.UUID // agent in Org1/G1
	AgentID2   uuid.UUID // agent in Org2/G1
}

// ---------------------------------------------------------------------------
// Seed — insert the MVP fixture, register automatic teardown
// ---------------------------------------------------------------------------

// Seed inserts the MVP fixture into Supabase using the service_role key
// (bypassing RLS). It registers t.Cleanup to call Teardown automatically
// when the test finishes. Fails the test with t.Fatal if any seed step fails.
//
// supabaseURL must be the base URL of the Supabase project (e.g.
// "https://<ref>.supabase.co" or "http://127.0.0.1:54321" for local dev).
// serviceRoleKey is the service_role JWT.
func Seed(t testing.TB, supabaseURL, serviceRoleKey string) FixtureIDs {
	t.Helper()

	s := &seeder{url: supabaseURL, key: serviceRoleKey}

	// 1. Auth users
	s.createUser(t, UserIDA, emailUserA, passwordUserA)
	s.createUser(t, UserIDB, emailUserB, passwordUserB)
	s.createUser(t, UserIDC, emailUserC, passwordUserC)

	// 2. Organizations
	s.upsertOrg(t, OrgID1, "Houston Fixture Org 1")
	s.upsertOrg(t, OrgID2, "Houston Fixture Org 2")

	// 3. Groups
	s.upsertGroup(t, GroupID1G1, OrgID1, "General", true)
	s.upsertGroup(t, GroupID1G2, OrgID1, "Group 2", false)
	s.upsertGroup(t, GroupID2G1, OrgID2, "General", true)

	// 4. Memberships
	s.upsertMembership(t, UserIDA, OrgID1, GroupID1G1, "group:member")
	s.upsertMembership(t, UserIDB, OrgID1, GroupID1G2, "group:manager")
	s.upsertMembership(t, UserIDC, OrgID2, GroupID2G1, "org:owner")

	// 5. Agents
	s.upsertAgent(t, AgentID1, OrgID1, GroupID1G1, "blank")
	s.upsertAgent(t, AgentID2, OrgID2, GroupID2G1, "blank")

	// 6. Storage objects
	s.upsertStorage(t, OrgID1, GroupID1G1, AgentID1)
	s.upsertStorage(t, OrgID2, GroupID2G1, AgentID2)

	// Register automatic teardown.
	t.Cleanup(func() {
		if err := Teardown(supabaseURL, serviceRoleKey); err != nil {
			t.Logf("fixture teardown warning: %v", err)
		}
	})

	return FixtureIDs{
		OrgID1:     OrgID1,
		OrgID2:     OrgID2,
		GroupID1G1: GroupID1G1,
		GroupID1G2: GroupID1G2,
		GroupID2G1: GroupID2G1,
		UserIDA:    UserIDA,
		UserIDB:    UserIDB,
		UserIDC:    UserIDC,
		AgentID1:   AgentID1,
		AgentID2:   AgentID2,
	}
}

// ---------------------------------------------------------------------------
// Teardown — delete all fixture entities in reverse dependency order
// ---------------------------------------------------------------------------

// Teardown deletes all MVP fixture entities from Supabase in reverse
// dependency order (storage → agents → memberships → groups → orgs → users).
// Idempotent: does not fail if entities are already absent.
//
// supabaseURL and serviceRoleKey are the same values passed to Seed.
func Teardown(supabaseURL, serviceRoleKey string) error {
	s := &seeder{url: supabaseURL, key: serviceRoleKey}
	var errs []error

	// 1. Storage objects
	if err := s.deleteStorage(OrgID1, GroupID1G1, AgentID1); err != nil {
		errs = append(errs, fmt.Errorf("delete storage agent1: %w", err))
	}
	if err := s.deleteStorage(OrgID2, GroupID2G1, AgentID2); err != nil {
		errs = append(errs, fmt.Errorf("delete storage agent2: %w", err))
	}

	// 2. Agents
	if err := s.deleteTableRow("agents", "id", AgentID1.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete agent1: %w", err))
	}
	if err := s.deleteTableRow("agents", "id", AgentID2.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete agent2: %w", err))
	}

	// 3. Memberships
	if err := s.deleteTableRow("memberships", "user_id", UserIDA.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete membership A: %w", err))
	}
	if err := s.deleteTableRow("memberships", "user_id", UserIDB.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete membership B: %w", err))
	}
	if err := s.deleteTableRow("memberships", "user_id", UserIDC.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete membership C: %w", err))
	}

	// 4. Groups
	if err := s.deleteTableRow("groups", "id", GroupID1G1.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete group1g1: %w", err))
	}
	if err := s.deleteTableRow("groups", "id", GroupID1G2.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete group1g2: %w", err))
	}
	if err := s.deleteTableRow("groups", "id", GroupID2G1.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete group2g1: %w", err))
	}

	// 5. Organizations
	if err := s.deleteTableRow("organizations", "id", OrgID1.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete org1: %w", err))
	}
	if err := s.deleteTableRow("organizations", "id", OrgID2.String()); err != nil {
		errs = append(errs, fmt.Errorf("delete org2: %w", err))
	}

	// 6. Auth users
	if err := s.deleteAuthUser(UserIDA); err != nil {
		errs = append(errs, fmt.Errorf("delete user A: %w", err))
	}
	if err := s.deleteAuthUser(UserIDB); err != nil {
		errs = append(errs, fmt.Errorf("delete user B: %w", err))
	}
	if err := s.deleteAuthUser(UserIDC); err != nil {
		errs = append(errs, fmt.Errorf("delete user C: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("teardown errors: %v", errs)
	}
	return nil
}

// ---------------------------------------------------------------------------
// UserJWT — obtain a signed JWT for a fixture user via password grant
// ---------------------------------------------------------------------------

// UserJWT obtains a valid Supabase JWT for the given fixture user by calling
// POST /auth/v1/token?grant_type=password with the user's fixture credentials.
//
// Fails the test with t.Fatal if:
//   - the user ID does not correspond to a known fixture user, or
//   - Supabase returns an error (e.g. user does not exist).
//
// Returns the raw access_token string (non-empty on success).
func UserJWT(t testing.TB, supabaseURL, serviceRoleKey string, userID uuid.UUID) string {
	t.Helper()

	// Resolve credentials for the given userID.
	email, password := fixtureCredentials(t, userID)

	// POST /auth/v1/token?grant_type=password
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	req, err := http.NewRequest(http.MethodPost,
		supabaseURL+"/auth/v1/token?grant_type=password",
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("UserJWT: build request for %s: %v", userID, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", serviceRoleKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("UserJWT: POST /auth/v1/token for %s: %v", userID, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("UserJWT: /auth/v1/token returned %d for user %s: %s",
			resp.StatusCode, userID, raw)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("UserJWT: unmarshal response for %s: %v\nbody: %s", userID, err, raw)
	}
	if result.AccessToken == "" {
		t.Fatalf("UserJWT: empty access_token for user %s\nbody: %s", userID, raw)
	}

	return result.AccessToken
}

// fixtureCredentials returns the email and password for a known fixture user.
// Calls t.Fatal if userID is not a known fixture user.
func fixtureCredentials(t testing.TB, userID uuid.UUID) (email, password string) {
	t.Helper()
	switch userID {
	case UserIDA:
		return emailUserA, passwordUserA
	case UserIDB:
		return emailUserB, passwordUserB
	case UserIDC:
		return emailUserC, passwordUserC
	default:
		t.Fatalf("UserJWT: unknown fixture userID %s — not in fixture (A=%s B=%s C=%s)",
			userID, UserIDA, UserIDB, UserIDC)
		return "", "" // unreachable
	}
}

// ---------------------------------------------------------------------------
// seeder — internal HTTP helper wrapping Supabase API calls
// ---------------------------------------------------------------------------

type seeder struct {
	url string // base Supabase URL, no trailing slash
	key string // service_role key
}

// serviceClient returns an http.Client that injects service_role authorization.
func (s *seeder) serviceClient() *http.Client {
	return http.DefaultClient
}

// do executes an HTTP request with service_role headers and returns the response.
func (s *seeder) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("apikey", s.key)
	req.Header.Set("Authorization", "Bearer "+s.key)
	return s.serviceClient().Do(req)
}

// createUser calls POST /auth/v1/admin/users to create a fixture user.
// Idempotent: if the email already exists, Supabase returns 422 which is treated
// as success (the user exists).
func (s *seeder) createUser(t testing.TB, id uuid.UUID, email, password string) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"id":            id.String(),
		"email":         email,
		"password":      password,
		"email_confirm": true,
	})
	req, err := http.NewRequest(http.MethodPost, s.url+"/auth/v1/admin/users", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("createUser %s: build request: %v", email, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.do(req)
	if err != nil {
		t.Fatalf("createUser %s: %v", email, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	// 200 = created, 422 = already exists (idempotent)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("createUser %s: unexpected status %d: %s", email, resp.StatusCode, raw)
	}
	t.Logf("user seeded: %s (HTTP %d)", email, resp.StatusCode)
}

// deleteAuthUser calls DELETE /auth/v1/admin/users/<id>.
// Idempotent: 404 is treated as success.
func (s *seeder) deleteAuthUser(id uuid.UUID) error {
	req, err := http.NewRequest(http.MethodDelete,
		s.url+"/auth/v1/admin/users/"+id.String(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusNoContent {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, raw)
}

// upsertOrg inserts or ignores a row in the organizations table.
func (s *seeder) upsertOrg(t testing.TB, id uuid.UUID, name string) {
	t.Helper()
	row := map[string]interface{}{"id": id.String(), "name": name}
	s.upsertRow(t, "organizations", row)
	t.Logf("org seeded: %s (%s)", name, id)
}

// upsertGroup inserts or ignores a row in the groups table.
func (s *seeder) upsertGroup(t testing.TB, id, orgID uuid.UUID, name string, isGeneral bool) {
	t.Helper()
	row := map[string]interface{}{
		"id":         id.String(),
		"org_id":     orgID.String(),
		"name":       name,
		"is_general": isGeneral,
	}
	s.upsertRow(t, "groups", row)
	t.Logf("group seeded: %s/%s (general=%v)", orgID, id, isGeneral)
}

// upsertMembership inserts or ignores a row in the memberships table.
func (s *seeder) upsertMembership(t testing.TB, userID, orgID, groupID uuid.UUID, role string) {
	t.Helper()
	row := map[string]interface{}{
		"user_id":  userID.String(),
		"org_id":   orgID.String(),
		"group_id": groupID.String(),
		"role":     role,
	}
	s.upsertRow(t, "memberships", row)
	t.Logf("membership seeded: user=%s role=%s", userID, role)
}

// upsertAgent inserts or ignores a row in the agents table.
func (s *seeder) upsertAgent(t testing.TB, id, orgID, groupID uuid.UUID, source string) {
	t.Helper()
	row := map[string]interface{}{
		"id":       id.String(),
		"org_id":   orgID.String(),
		"group_id": groupID.String(),
		"source":   source,
		"name":     "fixture-agent-" + id.String()[:8],
	}
	s.upsertRow(t, "agents", row)
	t.Logf("agent seeded: %s (source=%s)", id, source)
}

// upsertRow does a POST /rest/v1/<table> with Prefer: resolution=ignore-duplicates.
func (s *seeder) upsertRow(t testing.TB, table string, row map[string]interface{}) {
	t.Helper()
	body, _ := json.Marshal(row)
	req, err := http.NewRequest(http.MethodPost, s.url+"/rest/v1/"+table, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("upsertRow %s: build request: %v", table, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=ignore-duplicates,return=minimal")
	resp, err := s.do(req)
	if err != nil {
		t.Fatalf("upsertRow %s: %v", table, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("upsertRow %s: unexpected status %d: %s", table, resp.StatusCode, raw)
	}
}

// deleteTableRow does DELETE /rest/v1/<table>?<col>=eq.<val>.
// Idempotent: 404 and empty result sets are treated as success.
func (s *seeder) deleteTableRow(table, col, val string) error {
	url := fmt.Sprintf("%s/rest/v1/%s?%s=eq.%s", s.url, table, col, val)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Prefer", "return=minimal")
	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusNoContent {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("DELETE %s ?%s=%s: status %d: %s", table, col, val, resp.StatusCode, raw)
}

// upsertStorage uploads a CLAUDE.md stub to
// houston/{orgID}/{groupID}/agents/{agentID}/CLAUDE.md.
func (s *seeder) upsertStorage(t testing.TB, orgID, groupID, agentID uuid.UUID) {
	t.Helper()
	path := fmt.Sprintf("/storage/v1/object/houston/%s/%s/agents/%s/CLAUDE.md",
		orgID, groupID, agentID)
	content := []byte(fmt.Sprintf("# Fixture Agent\n\norg=%s group=%s agent=%s\n",
		orgID, groupID, agentID))

	req, err := http.NewRequest(http.MethodPost, s.url+path, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upsertStorage: build request: %v", err)
	}
	req.Header.Set("Content-Type", "text/markdown")
	req.Header.Set("x-upsert", "true")

	resp, err := s.do(req)
	if err != nil {
		t.Fatalf("upsertStorage: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("upsertStorage: unexpected status %d: %s", resp.StatusCode, raw)
	}
	t.Logf("storage object seeded: houston/%s/%s/agents/%s/CLAUDE.md", orgID, groupID, agentID)
}

// deleteStorage removes the CLAUDE.md object for a given agent from Storage.
// Idempotent: 404 is treated as success.
func (s *seeder) deleteStorage(orgID, groupID, agentID uuid.UUID) error {
	// Supabase Storage batch delete: DELETE /storage/v1/object/houston
	// with body {"prefixes":["path/to/object"]}
	objectPath := fmt.Sprintf("%s/%s/agents/%s/CLAUDE.md", orgID, groupID, agentID)
	body, _ := json.Marshal(map[string][]string{"prefixes": {objectPath}})

	req, err := http.NewRequest(http.MethodDelete,
		s.url+"/storage/v1/object/houston", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusNoContent {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("delete storage %s: status %d: %s", objectPath, resp.StatusCode, raw)
}
