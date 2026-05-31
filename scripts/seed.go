// scripts/seed.go — Task 0010: seed-fixture
//
// Reproducible MVP fixture script. Inserts 2 orgs / 3 groups / 3 users /
// 3 memberships / 2 agents / 2 Storage objects into Supabase using the
// service_role key (bypasses RLS). Fully idempotent.
//
// Usage:
//
//	export SUPABASE_URL=https://<ref>.supabase.co
//	export SUPABASE_SERVICE_ROLE_KEY=<key>
//	go run scripts/seed.go          # seed
//	go run scripts/seed.go -teardown # delete in reverse order
//
//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Deterministic UUID constants — must match tests/helpers/fixture.go
// ---------------------------------------------------------------------------

var (
	orgID1     = uuid.MustParse("00000010-0000-0000-0000-000000000001")
	orgID2     = uuid.MustParse("00000010-0000-0000-0000-000000000002")
	groupID1G1 = uuid.MustParse("00000010-0000-0001-0000-000000000001") // Org1 / Group1 (general)
	groupID1G2 = uuid.MustParse("00000010-0000-0001-0000-000000000002") // Org1 / Group2
	groupID2G1 = uuid.MustParse("00000010-0000-0002-0000-000000000001") // Org2 / Group1 (general)
	userIDA    = uuid.MustParse("00000010-0000-0000-0001-000000000001") // group:member
	userIDB    = uuid.MustParse("00000010-0000-0000-0001-000000000002") // group:manager
	userIDC    = uuid.MustParse("00000010-0000-0000-0001-000000000003") // org:owner
	agentID1   = uuid.MustParse("00000010-0000-0000-0002-000000000001") // agent in Org1/G1
	agentID2   = uuid.MustParse("00000010-0000-0000-0002-000000000002") // agent in Org2/G1
)

const (
	emailUserA    = "user-a@houston-fixture.test"
	emailUserB    = "user-b@houston-fixture.test"
	emailUserC    = "user-c@houston-fixture.test"
	passwordUserA = "houston-fixture-pass-A"
	passwordUserB = "houston-fixture-pass-B"
	passwordUserC = "houston-fixture-pass-C"
)

func main() {
	teardown := flag.Bool("teardown", false, "delete all fixture entities in reverse order")
	flag.Parse()

	supabaseURL := os.Getenv("SUPABASE_URL")
	serviceKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if supabaseURL == "" {
		fatalf("SUPABASE_URL is not set")
	}
	if serviceKey == "" {
		fatalf("SUPABASE_SERVICE_ROLE_KEY is not set")
	}

	s := &seeder{url: supabaseURL, key: serviceKey}

	if *teardown {
		runTeardown(s)
		return
	}
	runSeed(s)
}

// ---------------------------------------------------------------------------
// Seed — insert all fixture entities
// ---------------------------------------------------------------------------

func runSeed(s *seeder) {
	// 1. Auth users
	s.createUser(userIDA, emailUserA, passwordUserA)
	s.createUser(userIDB, emailUserB, passwordUserB)
	s.createUser(userIDC, emailUserC, passwordUserC)

	// 2. Organizations
	s.upsertOrg(orgID1, "Houston Fixture Org 1")
	s.upsertOrg(orgID2, "Houston Fixture Org 2")

	// 3. Groups
	s.upsertGroup(groupID1G1, orgID1, "General", true)
	s.upsertGroup(groupID1G2, orgID1, "Group 2", false)
	s.upsertGroup(groupID2G1, orgID2, "General", true)

	// 4. Memberships
	s.upsertMembership(userIDA, orgID1, groupID1G1, "group:member")
	s.upsertMembership(userIDB, orgID1, groupID1G2, "group:manager")
	s.upsertMembership(userIDC, orgID2, groupID2G1, "org:owner")

	// 5. Agents
	s.upsertAgent(agentID1, orgID1, groupID1G1, "blank", "fixture-agent-1")
	s.upsertAgent(agentID2, orgID2, groupID2G1, "blank", "fixture-agent-2")

	// 6. Storage objects
	s.upsertStorage(orgID1, groupID1G1, agentID1)
	s.upsertStorage(orgID2, groupID2G1, agentID2)

	fmt.Println("seed complete: all fixture entities inserted")
}

// ---------------------------------------------------------------------------
// Teardown — delete all fixture entities in reverse order
// ---------------------------------------------------------------------------

func runTeardown(s *seeder) {
	// 1. Storage objects
	s.deleteStorage(orgID1, groupID1G1, agentID1)
	s.deleteStorage(orgID2, groupID2G1, agentID2)

	// 2. Agents
	s.deleteRow("agents", "id", agentID1.String())
	s.deleteRow("agents", "id", agentID2.String())

	// 3. Memberships
	s.deleteRow("memberships", "user_id", userIDA.String())
	s.deleteRow("memberships", "user_id", userIDB.String())
	s.deleteRow("memberships", "user_id", userIDC.String())

	// 4. Groups
	s.deleteRow("groups", "id", groupID1G1.String())
	s.deleteRow("groups", "id", groupID1G2.String())
	s.deleteRow("groups", "id", groupID2G1.String())

	// 5. Organizations
	s.deleteRow("organizations", "id", orgID1.String())
	s.deleteRow("organizations", "id", orgID2.String())

	// 6. Auth users
	s.deleteAuthUser(userIDA)
	s.deleteAuthUser(userIDB)
	s.deleteAuthUser(userIDC)

	fmt.Println("teardown complete: all fixture entities deleted")
}

// ---------------------------------------------------------------------------
// seeder
// ---------------------------------------------------------------------------

type seeder struct {
	url string
	key string
}

func (s *seeder) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("apikey", s.key)
	req.Header.Set("Authorization", "Bearer "+s.key)
	return http.DefaultClient.Do(req)
}

func (s *seeder) createUser(id uuid.UUID, email, password string) {
	body, _ := json.Marshal(map[string]interface{}{
		"id":            id.String(),
		"email":         email,
		"password":      password,
		"email_confirm": true,
	})
	req, err := http.NewRequest(http.MethodPost, s.url+"/auth/v1/admin/users", bytes.NewReader(body))
	must(err, "build createUser request")
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.do(req)
	must(err, "createUser "+email)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	// 200 = created, 422 = already exists (idempotent)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnprocessableEntity {
		fatalf("createUser %s: HTTP %d: %s", email, resp.StatusCode, raw)
	}
	fmt.Printf("user seeded: %s (HTTP %d)\n", email, resp.StatusCode)
}

func (s *seeder) deleteAuthUser(id uuid.UUID) {
	req, err := http.NewRequest(http.MethodDelete, s.url+"/auth/v1/admin/users/"+id.String(), nil)
	must(err, "build deleteAuthUser request")
	resp, err := s.do(req)
	must(err, "deleteAuthUser "+id.String())
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusNoContent {
		fmt.Printf("auth user deleted: %s\n", id)
		return
	}
	raw, _ := io.ReadAll(resp.Body)
	fatalf("deleteAuthUser %s: HTTP %d: %s", id, resp.StatusCode, raw)
}

func (s *seeder) upsertOrg(id uuid.UUID, name string) {
	row := map[string]interface{}{"id": id.String(), "name": name}
	s.upsertRow("organizations", row)
	fmt.Printf("org seeded: %s (%s)\n", name, id)
}

func (s *seeder) upsertGroup(id, orgID uuid.UUID, name string, isGeneral bool) {
	row := map[string]interface{}{
		"id":         id.String(),
		"org_id":     orgID.String(),
		"name":       name,
		"is_general": isGeneral,
	}
	s.upsertRow("groups", row)
	fmt.Printf("group seeded: %s in org=%s (general=%v)\n", id, orgID, isGeneral)
}

func (s *seeder) upsertMembership(userID, orgID, groupID uuid.UUID, role string) {
	row := map[string]interface{}{
		"user_id":  userID.String(),
		"org_id":   orgID.String(),
		"group_id": groupID.String(),
		"role":     role,
	}
	s.upsertRow("memberships", row)
	fmt.Printf("membership seeded: user=%s role=%s\n", userID, role)
}

func (s *seeder) upsertAgent(id, orgID, groupID uuid.UUID, source, name string) {
	row := map[string]interface{}{
		"id":       id.String(),
		"org_id":   orgID.String(),
		"group_id": groupID.String(),
		"source":   source,
		"name":     name,
	}
	s.upsertRow("agents", row)
	fmt.Printf("agent seeded: %s (source=%s)\n", id, source)
}

func (s *seeder) upsertRow(table string, row map[string]interface{}) {
	body, _ := json.Marshal(row)
	req, err := http.NewRequest(http.MethodPost, s.url+"/rest/v1/"+table, bytes.NewReader(body))
	must(err, "build upsertRow "+table)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=ignore-duplicates,return=minimal")
	resp, err := s.do(req)
	must(err, "upsertRow "+table)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		fatalf("upsertRow %s: HTTP %d: %s", table, resp.StatusCode, raw)
	}
}

func (s *seeder) deleteRow(table, col, val string) {
	url := fmt.Sprintf("%s/rest/v1/%s?%s=eq.%s", s.url, table, col, val)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	must(err, "build deleteRow "+table)
	req.Header.Set("Prefer", "return=minimal")
	resp, err := s.do(req)
	must(err, "deleteRow "+table)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusNoContent {
		fmt.Printf("row deleted: %s where %s=%s\n", table, col, val)
		return
	}
	raw, _ := io.ReadAll(resp.Body)
	fatalf("deleteRow %s: HTTP %d: %s", table, resp.StatusCode, raw)
}

func (s *seeder) upsertStorage(orgID, groupID, agentID uuid.UUID) {
	path := fmt.Sprintf("/storage/v1/object/houston/%s/%s/agents/%s/CLAUDE.md",
		orgID, groupID, agentID)
	content := []byte(fmt.Sprintf("# Fixture Agent\n\norg=%s group=%s agent=%s\n",
		orgID, groupID, agentID))
	req, err := http.NewRequest(http.MethodPost, s.url+path, bytes.NewReader(content))
	must(err, "build upsertStorage")
	req.Header.Set("Content-Type", "text/markdown")
	req.Header.Set("x-upsert", "true")
	resp, err := s.do(req)
	must(err, "upsertStorage")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		fatalf("upsertStorage: HTTP %d: %s", resp.StatusCode, raw)
	}
	fmt.Printf("storage object seeded: houston/%s/%s/agents/%s/CLAUDE.md\n", orgID, groupID, agentID)
}

func (s *seeder) deleteStorage(orgID, groupID, agentID uuid.UUID) {
	objectPath := fmt.Sprintf("%s/%s/agents/%s/CLAUDE.md", orgID, groupID, agentID)
	body, _ := json.Marshal(map[string][]string{"prefixes": {objectPath}})
	req, err := http.NewRequest(http.MethodDelete, s.url+"/storage/v1/object/houston", bytes.NewReader(body))
	must(err, "build deleteStorage")
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.do(req)
	must(err, "deleteStorage")
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusNoContent {
		fmt.Printf("storage object deleted: houston/%s\n", objectPath)
		return
	}
	raw, _ := io.ReadAll(resp.Body)
	fatalf("deleteStorage: HTTP %d: %s", resp.StatusCode, raw)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func must(err error, ctx string) {
	if err != nil {
		fatalf("%s: %v", ctx, err)
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "seed: "+format+"\n", args...)
	os.Exit(1)
}
