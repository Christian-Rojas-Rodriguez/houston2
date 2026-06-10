// Integration tests for Task 0008 (run-agent-flow).
//
// These mock the RunStore and Runner (no real Supabase, no real `claude`),
// exercising the handler + middleware chain via httptest. Reuses helpers from
// 0007__create-agent-flow_test.go (integTestJWTSecret, makeIntegJWT,
// memberQuerier, assertErrorCode, stubTenantQuerier).
//
// AC mapping
//   AC1 happy → TestCreateRun_Happy
//   AC2 member allowed → TestCreateRun_Happy (RoleMember chain)
//   AC3 agent not found → TestCreateRun_AgentNotFound
//   AC4 missing prompt → TestCreateRun_MissingPrompt
//   AC5 context download → TestCreateRun_Happy (download dir == runner dir)
//   AC6 runner error → TestCreateRun_RunnerError
//   AC7 no api key → TestCreateRun_NoAPIKey
//   AC9 auth gate → TestCreateRun_NoJWT

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/handlers"
	"github.com/nomenclator/houston2/internal/middleware"
)

// ---------------------------------------------------------------------------
// mocks
// ---------------------------------------------------------------------------

type mockRunStore struct {
	ref         handlers.AgentRef
	getAgentErr error
	createErr   error
	runID       uuid.UUID
	createdRun  handlers.RunRecord
	createCalls int
	statuses    []string
	downloadDir string
	downloadErr error
}

func (m *mockRunStore) GetAgent(_ context.Context, _ string, id uuid.UUID) (handlers.AgentRef, error) {
	if m.getAgentErr != nil {
		return handlers.AgentRef{}, m.getAgentErr
	}
	ref := m.ref
	ref.ID = id
	return ref, nil
}

func (m *mockRunStore) CreateRun(_ context.Context, _ string, r handlers.RunRecord) (uuid.UUID, error) {
	m.createCalls++
	if m.createErr != nil {
		return uuid.Nil, m.createErr
	}
	m.createdRun = r
	if m.runID == uuid.Nil {
		m.runID = uuid.New()
	}
	return m.runID, nil
}

func (m *mockRunStore) UpdateRun(_ context.Context, _ string, _ uuid.UUID, status, _ string) error {
	m.statuses = append(m.statuses, status)
	return nil
}

func (m *mockRunStore) DownloadContext(_ context.Context, _ string, _ handlers.AgentRef, destDir string) error {
	m.downloadDir = destDir
	return m.downloadErr
}

type mockRunner struct {
	output    string
	err       error
	called    bool
	gotDir    string
	gotPrompt string
	gotKey    string
}

func (m *mockRunner) Run(_ context.Context, dir, prompt, apiKey string) (string, error) {
	m.called = true
	m.gotDir, m.gotPrompt, m.gotKey = dir, prompt, apiKey
	return m.output, m.err
}

func (m *mockRunStore) lastStatus() string {
	if len(m.statuses) == 0 {
		return ""
	}
	return m.statuses[len(m.statuses)-1]
}

// ---------------------------------------------------------------------------
// harness
// ---------------------------------------------------------------------------

// runMux builds the real Auth→Tenant→RequireRole(Member)→CreateRun chain behind
// a ServeMux so r.PathValue("id") resolves (mirrors server.go wiring).
func runMux(t *testing.T, q middleware.TenantQuerier, store handlers.RunStore, runner handlers.Runner, apiKey string) http.Handler {
	t.Helper()
	authCfg := auth.Config{
		SupabaseURL: "https://fake.supabase.co",
		AnonKey:     "test-anon-key",
		JWTSecret:   integTestJWTSecret,
		Port:        "0",
	}
	chain := auth.AuthMiddleware(authCfg)(
		middleware.TenantMiddleware(q)(
			middleware.RequireRole(middleware.RoleMember)(
				handlers.CreateRun(store, runner, apiKey),
			),
		),
	)
	mux := http.NewServeMux()
	mux.Handle("POST /v1/agents/{id}/runs", chain)
	return mux
}

func postRun(t *testing.T, h http.Handler, token, agentID string, body any) *http.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agentID+"/runs", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func memberToken(t *testing.T) string {
	t.Helper()
	return makeIntegJWT(t, uuid.New().String(), time.Now().Add(1*time.Hour))
}

// ---------------------------------------------------------------------------
// AC1/AC2/AC5 — happy path: member runs the agent, claude output returned
// ---------------------------------------------------------------------------

func TestCreateRun_Happy(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockRunStore{ref: handlers.AgentRef{OrgID: orgID, GroupID: groupID}}
	runner := &mockRunner{output: "hello from claude"}

	h := runMux(t, memberQuerier(orgID, groupID), store, runner, "test-key")
	agentID := uuid.New()

	res := postRun(t, h, memberToken(t), agentID.String(), map[string]string{"prompt": "hola"})
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("AC1: expected 200, got %d — %s", res.StatusCode, body)
	}
	var out map[string]string
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("AC1: decode body: %v", err)
	}
	if out["status"] != "done" || out["result"] != "hello from claude" || out["run_id"] == "" {
		t.Errorf("AC1: unexpected body %v", out)
	}
	if store.lastStatus() != "done" {
		t.Errorf("AC1: final run status = %q, want done (seq: %v)", store.lastStatus(), store.statuses)
	}
	if store.createdRun.AgentID != agentID || store.createdRun.OrgID != orgID || store.createdRun.GroupID != groupID {
		t.Errorf("AC1: run not scoped to the agent: %+v", store.createdRun)
	}
	if store.createdRun.UserID == uuid.Nil {
		t.Error("AC1: run user_id must be the caller")
	}
	if runner.gotKey != "test-key" {
		t.Errorf("AC7: runner got key %q, want test-key", runner.gotKey)
	}
	if runner.gotPrompt != "hola" {
		t.Errorf("AC1: runner got prompt %q", runner.gotPrompt)
	}
	// AC5: context hydrated into the same workspace the runner uses.
	if store.downloadDir == "" || store.downloadDir != runner.gotDir {
		t.Errorf("AC5: download dir %q != runner dir %q", store.downloadDir, runner.gotDir)
	}
	if !strings.Contains(runner.gotDir, "houston-run-") {
		t.Errorf("AC8: workspace not isolated per-run: %q", runner.gotDir)
	}
}

// ---------------------------------------------------------------------------
// AC3 — agent not visible → 404, no run created
// ---------------------------------------------------------------------------

func TestCreateRun_AgentNotFound(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockRunStore{getAgentErr: handlers.ErrAgentNotFound}
	h := runMux(t, memberQuerier(orgID, groupID), store, &mockRunner{}, "k")

	res := postRun(t, h, memberToken(t), uuid.New().String(), map[string]string{"prompt": "x"})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusNotFound, "not_found")
	if store.createCalls != 0 {
		t.Error("AC3: CreateRun must not be called when the agent is not found")
	}
}

// ---------------------------------------------------------------------------
// AC4 — missing prompt → 400
// ---------------------------------------------------------------------------

func TestCreateRun_MissingPrompt(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockRunStore{ref: handlers.AgentRef{OrgID: orgID, GroupID: groupID}}
	h := runMux(t, memberQuerier(orgID, groupID), store, &mockRunner{}, "k")

	res := postRun(t, h, memberToken(t), uuid.New().String(), map[string]string{})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusBadRequest, "invalid_request")
}

// ---------------------------------------------------------------------------
// AC6 — runner error → 502, run marked error
// ---------------------------------------------------------------------------

func TestCreateRun_RunnerError(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockRunStore{ref: handlers.AgentRef{OrgID: orgID, GroupID: groupID}}
	runner := &mockRunner{err: errors.New("claude exploded")}
	h := runMux(t, memberQuerier(orgID, groupID), store, runner, "k")

	res := postRun(t, h, memberToken(t), uuid.New().String(), map[string]string{"prompt": "x"})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusBadGateway, "upstream_error")
	if store.lastStatus() != "error" {
		t.Errorf("AC6: final status = %q, want error", store.lastStatus())
	}
}

// ---------------------------------------------------------------------------
// AC7 — no API key configured → 502, runner never invoked
// ---------------------------------------------------------------------------

func TestCreateRun_NoAPIKey(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockRunStore{ref: handlers.AgentRef{OrgID: orgID, GroupID: groupID}}
	runner := &mockRunner{output: "should not run"}
	h := runMux(t, memberQuerier(orgID, groupID), store, runner, "") // empty key

	res := postRun(t, h, memberToken(t), uuid.New().String(), map[string]string{"prompt": "x"})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusBadGateway, "upstream_error")
	if runner.called {
		t.Error("AC7: runner must not run without an API key")
	}
	if store.lastStatus() != "error" {
		t.Errorf("AC7: final status = %q, want error", store.lastStatus())
	}
}

// ---------------------------------------------------------------------------
// AC9 — no JWT → 401 (auth gate)
// ---------------------------------------------------------------------------

func TestCreateRun_NoJWT(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockRunStore{ref: handlers.AgentRef{OrgID: orgID, GroupID: groupID}}
	h := runMux(t, memberQuerier(orgID, groupID), store, &mockRunner{}, "k")

	res := postRun(t, h, "", uuid.New().String(), map[string]string{"prompt": "x"})
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("AC9: expected 401, got %d", res.StatusCode)
	}
}
