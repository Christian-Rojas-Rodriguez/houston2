//go:build acceptance

// Package acceptance_test — MVP RBAC acceptance gate (Task 0012).
//
// Table-driven over the RFC §4.5 role×operation matrix vs the REAL server
// (server.New + real supabaseQuerier) with REAL fixture JWTs (ES256, validated
// via the project JWKS by the fixed auth middleware). Live cells: create-agent
// role gate + run-agent cross-org/cross-group denials + auth. Cells whose
// endpoints aren't wired yet skip-with-reason.
//
// Run with a live Supabase:
//   SUPABASE_URL=... SUPABASE_SERVICE_ROLE_KEY=... SUPABASE_ANON_KEY=... SUPABASE_JWT_SECRET=... \
//   go test -tags=acceptance -run TestRBAC ./tests/acceptance/ -v
// or via scripts/rbac-test.sh.
//
// Reuses helpers from 0011__leak-test_test.go (env, loadEnv, svcInsert, svcDelete).
package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/server"
	"github.com/nomenclator/houston2/tests/helpers"
)

// canaryG2Agent is a fixed-id agent seeded into Org1/Group2 (non-general) so the
// cross-group denial (member A, in the general group, can't see it) is testable.
var canaryG2Agent = uuid.MustParse("00000012-0000-0000-0002-000000000001")

func TestRBAC(t *testing.T) {
	e := loadEnv(t) // SUPABASE_URL + SERVICE_ROLE_KEY + ANON_KEY (skips if unset)
	ids := helpers.Seed(t, e.url, e.serviceKey)
	jwtA := helpers.UserJWT(t, e.url, e.serviceKey, ids.UserIDA) // member, Org1/Group1-general
	jwtB := helpers.UserJWT(t, e.url, e.serviceKey, ids.UserIDB) // manager, Org1/Group2
	jwtC := helpers.UserJWT(t, e.url, e.serviceKey, ids.UserIDC) // owner, Org2

	// Cross-group canary: a non-general agent in Org1/Group2.
	seedCanaryAgent(t, e, canaryG2Agent, ids.OrgID1, ids.GroupID1G2)

	cfg := server.Config{
		SupabaseURL: e.url,
		AnonKey:     e.anonKey,
		JWTSecret:   os.Getenv("SUPABASE_JWT_SECRET"), // HS256 fallback; ES256 tokens use JWKS
		Port:        "0",
	}
	srv := server.New(cfg, server.NewSupabaseQuerier(e.url, e.anonKey))

	do := func(method, path, jwt, body string) (int, []byte) {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Content-Type", "application/json")
		if jwt != "" {
			req.Header.Set("Authorization", "Bearer "+jwt)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		res := rec.Result()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, b
	}

	// ---- LIVE: create-agent role gate (POST /v1/agents requires RoleManager) ----
	t.Run("M1_member_create_agent_403", func(t *testing.T) {
		st, b := do(http.MethodPost, "/v1/agents", jwtA, `{"name":"rbac","source":"blank"}`)
		assertForbidden(t, st, b)
	})
	t.Run("M2_manager_create_agent_allowed", func(t *testing.T) {
		st, b := do(http.MethodPost, "/v1/agents", jwtB, `{"name":"rbac-m2","source":"blank"}`)
		assertNotDenied(t, st, b)
		cleanupCreatedAgent(t, e, b, ids.OrgID1, ids.GroupID1G2)
	})
	t.Run("M3_owner_create_agent_allowed", func(t *testing.T) {
		st, b := do(http.MethodPost, "/v1/agents", jwtC, `{"name":"rbac-m3","source":"blank"}`)
		assertNotDenied(t, st, b)
		cleanupCreatedAgent(t, e, b, ids.OrgID2, ids.GroupID2G1)
	})
	t.Run("M4_no_jwt_401", func(t *testing.T) {
		st, _ := do(http.MethodPost, "/v1/agents", "", `{"name":"x","source":"blank"}`)
		assertStatus(t, st, http.StatusUnauthorized)
	})

	// ---- LIVE: run-agent denials (cheap — fail at GetAgent before any claude run) ----
	t.Run("M16_cross_org_run_404", func(t *testing.T) {
		st, _ := do(http.MethodPost, "/v1/agents/"+ids.AgentID2.String()+"/runs", jwtA, `{"prompt":"x"}`)
		assertStatus(t, st, http.StatusNotFound) // A (Org1) cannot see Org2's agent
	})
	t.Run("M17_cross_group_run_404", func(t *testing.T) {
		st, _ := do(http.MethodPost, "/v1/agents/"+canaryG2Agent.String()+"/runs", jwtA, `{"prompt":"x"}`)
		assertStatus(t, st, http.StatusNotFound) // A (Group1-general) cannot see Group2's non-general agent
	})
	t.Run("run_no_jwt_401", func(t *testing.T) {
		st, _ := do(http.MethodPost, "/v1/agents/"+ids.AgentID1.String()+"/runs", "", `{"prompt":"x"}`)
		assertStatus(t, st, http.StatusUnauthorized)
	})
	// M15 (allowed own-group run) is NOT asserted: it triggers a real claude run
	// (cost + latency). It is functional and covered by Task 0008.

	// ---- PENDING: endpoints not wired yet (skip with reason; no silent green) ----
	for _, p := range []struct{ name, reason string }{
		{"M5_M7_groups_crud", "pending: rutas /v1/groups CRUD no ruteadas"},
		{"M8_M11_memberships_crud", "pending: rutas /v1/memberships CRUD no ruteadas"},
		{"M12_M14_credentials", "pending: /v1/orgs/{id}/credentials no ruteado (0009 difirió wiring)"},
		{"M18_read_run_rbac", "pending: GET /v1/runs/{id} es stub sin RBAC"},
		{"M19_credentials_cross_org", "pending: /v1/orgs/{id}/credentials no ruteado"},
	} {
		p := p
		t.Run(p.name, func(t *testing.T) { t.Skip(p.reason) })
	}
}

// ---------------------------------------------------------------------------
// helpers (env, svcInsert, svcDelete reused from 0011__leak-test_test.go)
// ---------------------------------------------------------------------------

func seedCanaryAgent(t *testing.T, e env, id, org, group uuid.UUID) {
	t.Helper()
	ok := svcInsert(t, e, "agents", map[string]any{
		"id":       id.String(),
		"org_id":   org.String(),
		"group_id": group.String(),
		"source":   "blank",
		"name":     "rbac-canary-g2",
	})
	if ok {
		t.Cleanup(func() { svcDelete(e, "agents", "id", id.String()) })
	}
}

func cleanupCreatedAgent(t *testing.T, e env, body []byte, org, group uuid.UUID) {
	t.Helper()
	var resp map[string]string
	if json.Unmarshal(body, &resp) != nil || resp["id"] == "" {
		return
	}
	id := resp["id"]
	t.Cleanup(func() {
		svcDelete(e, "agents", "id", id)
		deleteStorageObject(e, fmt.Sprintf("%s/%s/agents/%s/CLAUDE.md", org, group, id))
	})
}

func deleteStorageObject(e env, objectName string) {
	body, _ := json.Marshal(map[string][]string{"prefixes": {objectName}})
	req, _ := http.NewRequest(http.MethodDelete, e.url+"/storage/v1/object/houston", strings.NewReader(string(body)))
	req.Header.Set("apikey", e.serviceKey)
	req.Header.Set("Authorization", "Bearer "+e.serviceKey)
	req.Header.Set("Content-Type", "application/json")
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
	}
}

func assertForbidden(t *testing.T, status int, body []byte) {
	t.Helper()
	if status != http.StatusForbidden {
		t.Errorf("expected 403, got %d — %s", status, body)
		return
	}
	var er struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &er) != nil || er.Error.Code != "forbidden" {
		t.Errorf("403 body is not a forbidden envelope: %s", body)
	}
}

func assertStatus(t *testing.T, status, want int) {
	t.Helper()
	if status != want {
		t.Errorf("expected %d, got %d", want, status)
	}
}

func assertNotDenied(t *testing.T, status int, body []byte) {
	t.Helper()
	if status == http.StatusForbidden || status == http.StatusUnauthorized {
		t.Errorf("expected allowed (not 403/401), got %d — %s", status, body)
	}
}
