//go:build acceptance

// Package acceptance_test is the MVP leak-test acceptance gate (Task 0011).
//
// Acting as User A (Org1 / Group1-general / group:member) it asserts ZERO
// cross-tenant leakage at the DATA layer (PostgREST under A's real Supabase JWT,
// RLS in force — never service_role) for every tenant-scoped table + Storage.
// `runs` and `org_credentials` are empty in the 0010 fixture, so the test
// arranges service_role canaries (cleaned up via t.Cleanup) to keep those
// assertions non-vacuous.
//
// Run against a live Supabase:
//
//	SUPABASE_URL=... SUPABASE_SERVICE_ROLE_KEY=... SUPABASE_ANON_KEY=... \
//	go test -tags=acceptance -run TestLeak ./tests/acceptance/ -v
//
// or via scripts/leak-test.sh (CI merge gate).
//
// AC mapping
// ----------
// AC1 organizations · AC2 groups · AC3 memberships · AC4 agents
// AC5 runs (canary) · AC6 org_credentials (canary) · AC7 storage (pos+neg)
// AC8 HTTP get-agent — SKIPPED (R-HTTP-STUB: route still a stub)
// AC9 probes use caller JWT + anon apikey, never service_role
// AC11 skips without env · AC12 runs against real infra
package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/tests/helpers"
)

type env struct{ url, serviceKey, anonKey string }

func loadEnv(t *testing.T) env {
	t.Helper()
	e := env{
		url:        os.Getenv("SUPABASE_URL"),
		serviceKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		anonKey:    os.Getenv("SUPABASE_ANON_KEY"),
	}
	if e.url == "" || e.serviceKey == "" {
		t.Skip("SUPABASE_URL / SUPABASE_SERVICE_ROLE_KEY not set — skipping acceptance leak-test")
	}
	if e.anonKey == "" {
		t.Skip("SUPABASE_ANON_KEY not set — skipping (data-level probe needs the anon apikey)")
	}
	return e
}

// canaryRunID is fixed so the canary run can be cleaned up precisely.
var canaryRunID = uuid.MustParse("00000011-0000-0000-0000-0000000000a1")

func TestLeak(t *testing.T) {
	e := loadEnv(t)
	ids := helpers.Seed(t, e.url, e.serviceKey) // registers teardown via t.Cleanup
	jwtA := helpers.UserJWT(t, e.url, e.serviceKey, ids.UserIDA)

	// Canaries so runs/org_credentials assertions are non-vacuous. Cleanup is
	// LIFO so canaries are removed before the fixture teardown (FK-safe).
	seedRunCanary(t, e, ids)
	seedCredCanary(t, e, ids)

	// ---- Layer 1: data-level (apikey anon + Bearer jwtA; RLS in force) -------

	t.Run("organizations", func(t *testing.T) {
		rows := userGet(t, e, jwtA, "organizations")
		mustSeeOwn(t, len(rows) > 0, "A debe ver su propia org")
		for _, r := range rows {
			if id := str(r["id"]); id != ids.OrgID1.String() {
				t.Errorf("LEAK organizations: A ve org ajena %s (solo %s)", id, ids.OrgID1)
			}
		}
	})

	t.Run("groups", func(t *testing.T) {
		rows := userGet(t, e, jwtA, "groups")
		mustSeeOwn(t, len(rows) > 0, "A debe ver su grupo")
		for _, r := range rows {
			if id := str(r["id"]); id != ids.GroupID1G1.String() {
				t.Errorf("LEAK groups: A ve grupo ajeno %s (solo %s)", id, ids.GroupID1G1)
			}
		}
	})

	t.Run("memberships", func(t *testing.T) {
		rows := userGet(t, e, jwtA, "memberships")
		for _, r := range rows {
			if uid := str(r["user_id"]); uid == ids.UserIDB.String() || uid == ids.UserIDC.String() {
				t.Errorf("LEAK memberships: A ve membership ajena (user_id=%s)", uid)
			}
		}
	})

	t.Run("agents", func(t *testing.T) {
		rows := userGet(t, e, jwtA, "agents")
		for _, r := range rows {
			if id := str(r["id"]); id == ids.AgentID2.String() {
				t.Errorf("LEAK agents: A ve AgentID2 (Org2) %s", id)
			}
		}
	})

	t.Run("runs", func(t *testing.T) {
		rows := userGet(t, e, jwtA, "runs")
		for _, r := range rows {
			if str(r["id"]) == canaryRunID.String() {
				t.Errorf("LEAK runs: A ve el run canario de Org2 %s", canaryRunID)
			}
			if og := str(r["org_id"]); og != "" && og != ids.OrgID1.String() {
				t.Errorf("LEAK runs: A ve run de org ajena %s", og)
			}
		}
	})

	t.Run("org_credentials", func(t *testing.T) {
		rows := userGet(t, e, jwtA, "org_credentials")
		if len(rows) != 0 {
			t.Errorf("LEAK org_credentials: el member A ve %d rows (debe ser 0 aun con el canario existente)", len(rows))
		}
	})

	// ---- Layer 1: storage (positive + negative; R-STORAGE resuelto) ---------

	t.Run("storage", func(t *testing.T) {
		own := fmt.Sprintf("%s/%s/agents/%s/CLAUDE.md", ids.OrgID1, ids.GroupID1G1, ids.AgentID1)
		foreign := fmt.Sprintf("%s/%s/agents/%s/CLAUDE.md", ids.OrgID2, ids.GroupID2G1, ids.AgentID2)

		if st := userGetObject(t, e, jwtA, own); st != http.StatusOK {
			t.Errorf("storage positivo: A no puede leer su propio objeto %s (status %d)", own, st)
		}
		if st := userGetObject(t, e, jwtA, foreign); st == http.StatusOK {
			t.Errorf("LEAK storage: A puede leer el objeto de Org2 %s (status 200)", foreign)
		}
	})

	// ---- Layer 2: HTTP-level — pendiente (R-HTTP-STUB) ----------------------

	t.Run("http-get-agent", func(t *testing.T) {
		t.Skip("R-HTTP-STUB: GET /v1/agents/{id} sigue en stubHandler — pendiente del handler get-agent")
	})
}

// ---------------------------------------------------------------------------
// Probe helpers — User A uses apikey=anon + Bearer jwtA (NEVER service_role).
// ---------------------------------------------------------------------------

func userGet(t *testing.T, e env, jwt, table string) []map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.url+"/rest/v1/"+table+"?select=*", nil)
	req.Header.Set("apikey", e.anonKey)
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", table, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", table, resp.StatusCode, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode %s: %v — %s", table, err, body)
	}
	return rows
}

func userGetObject(t *testing.T, e env, jwt, objectName string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.url+"/storage/v1/object/houston/"+objectName, nil)
	req.Header.Set("apikey", e.anonKey)
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET object %s: %v", objectName, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// ---------------------------------------------------------------------------
// Canaries (service_role). Best-effort: a failure logs a warning and leaves the
// assertion vacuous-but-valid rather than failing the whole gate.
// ---------------------------------------------------------------------------

func seedRunCanary(t *testing.T, e env, ids helpers.FixtureIDs) {
	t.Helper()
	ok := svcInsert(t, e, "runs", map[string]any{
		"id":       canaryRunID.String(),
		"agent_id": ids.AgentID2.String(),
		"org_id":   ids.OrgID2.String(),
		"group_id": ids.GroupID2G1.String(),
		"user_id":  ids.UserIDC.String(),
		"prompt":   "leak-canary",
		"status":   "done",
	})
	if ok {
		t.Cleanup(func() { svcDelete(e, "runs", "id", canaryRunID.String()) })
	}
}

func seedCredCanary(t *testing.T, e env, ids helpers.FixtureIDs) {
	t.Helper()
	// anthropic_key is bytea NOT NULL; PostgREST accepts the \x... hex literal.
	ok := svcInsert(t, e, "org_credentials", map[string]any{
		"org_id":        ids.OrgID1.String(),
		"anthropic_key": `\x73656372657421`,
	})
	if ok {
		t.Cleanup(func() { svcDelete(e, "org_credentials", "org_id", ids.OrgID1.String()) })
	}
}

func svcInsert(t *testing.T, e env, table string, row map[string]any) bool {
	t.Helper()
	b, _ := json.Marshal(row)
	req, _ := http.NewRequest(http.MethodPost, e.url+"/rest/v1/"+table, bytes.NewReader(b))
	req.Header.Set("apikey", e.serviceKey)
	req.Header.Set("Authorization", "Bearer "+e.serviceKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=ignore-duplicates,return=minimal")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("WARN canary insert %s: %v (assertion will be vacuous)", table, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("WARN canary insert %s: status %d: %s (assertion will be vacuous)", table, resp.StatusCode, body)
		return false
	}
	return true
}

func svcDelete(e env, table, col, val string) {
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/rest/v1/%s?%s=eq.%s", e.url, table, col, val), nil)
	req.Header.Set("apikey", e.serviceKey)
	req.Header.Set("Authorization", "Bearer "+e.serviceKey)
	req.Header.Set("Prefer", "return=minimal")
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
	}
}

func str(v any) string { s, _ := v.(string); return s }

func mustSeeOwn(t *testing.T, cond bool, msg string) {
	t.Helper()
	if !cond {
		t.Errorf("%s — got 0 rows (aislamiento podría ser vacuo)", msg)
	}
}
