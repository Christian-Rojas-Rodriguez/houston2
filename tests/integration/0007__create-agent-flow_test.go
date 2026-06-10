// Package integration_test is the failing-first test suite for Task 0007
// (create-agent-flow).
//
// TDD red phase: these tests reference types in internal/handlers that do not
// exist yet. The file will not compile until the Coder creates:
//   - internal/handlers/agents.go        (AgentStore, AgentRecord, CreateAgent,
//                                          ErrTemplateNotFound, ErrNoCredentials)
//   - internal/handlers/supabase_agent_store.go (SupabaseAgentStore)
//   - server.go modification: POST /v1/agents wired to CreateAgent at RoleManager
//
// AC mapping
// ----------
// AC1  — TestCreateAgent_Blank_Returns201WithID
// AC2  — TestCreateAgent_RoleMember_Returns403
// AC3  — TestCreateAgent_Template_Returns201
// AC4  — TestCreateAgent_Template_MissingTemplateID_Returns400
// AC5  — TestCreateAgent_Template_NotFound_Returns404
// AC6  — TestCreateAgent_AIAssist_Returns201
// AC7  — TestCreateAgent_AIAssist_NoCredentials_Returns402
// AC8  — TestCreateAgent_GitHub_Returns201
// AC9  — TestCreateAgent_GitHub_NotFound_Returns422
// AC10 — TestCreateAgent_MissingName_Returns400
// AC11 — TestCreateAgent_InvalidSource_Returns400

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/handlers"
	"github.com/nomenclator/houston2/internal/middleware"
)

// ---------------------------------------------------------------------------
// Shared test fixtures
// ---------------------------------------------------------------------------

const integTestJWTSecret = "super-secret-for-0007-integration-tests"

// makeIntegJWT creates an HS256-signed JWT with the given sub and expiry.
func makeIntegJWT(t *testing.T, sub string, expiry time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": sub,
		"exp": jwt.NewNumericDate(expiry),
		"iat": jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(integTestJWTSecret))
	if err != nil {
		t.Fatalf("makeIntegJWT: %v", err)
	}
	return signed
}

// validManagerJWT returns a non-expired JWT for a fresh UUID subject.
func validManagerJWT(t *testing.T) (string, uuid.UUID) {
	t.Helper()
	id := uuid.New()
	return makeIntegJWT(t, id.String(), time.Now().Add(1*time.Hour)), id
}

// ---------------------------------------------------------------------------
// mockAgentStore — in-test implementation of handlers.AgentStore
// ---------------------------------------------------------------------------

type mockAgentStore struct {
	// CreateAgent behavior
	createErr        error
	capturedRecord   handlers.AgentRecord
	capturedContent  []byte
	createAgentCalls int

	// GetTemplate behavior
	templateContent []byte
	templateErr     error

	// GetAnthropicKey behavior
	anthropicKey string
	anthropicErr error
}

func (m *mockAgentStore) CreateAgent(_ context.Context, _ string, a handlers.AgentRecord, content []byte) error {
	m.createAgentCalls++
	m.capturedRecord = a
	m.capturedContent = content
	return m.createErr
}

func (m *mockAgentStore) GetTemplate(_ context.Context, _ string, _ string) ([]byte, error) {
	return m.templateContent, m.templateErr
}

func (m *mockAgentStore) GetAnthropicKey(_ context.Context, _ string, _ uuid.UUID) (string, error) {
	return m.anthropicKey, m.anthropicErr
}

// ---------------------------------------------------------------------------
// stubTenantQuerier — returns a fixed TenantContext or error
// ---------------------------------------------------------------------------

type stubTenantQuerier struct {
	returnCtx middleware.TenantContext
	returnErr error
}

func (s *stubTenantQuerier) CurrentTenant(_ context.Context, _ uuid.UUID) (middleware.TenantContext, error) {
	if s.returnErr != nil {
		return middleware.TenantContext{}, s.returnErr
	}
	return s.returnCtx, nil
}

// ---------------------------------------------------------------------------
// testChain builds a handler chain for POST /v1/agents with the given
// min role, store, and Anthropic API base URL.
// The chain mirrors what server.go will wire after task 0007 is implemented.
// ---------------------------------------------------------------------------

func testChain(
	t *testing.T,
	supabaseURL string,
	minRole middleware.Role,
	querier middleware.TenantQuerier,
	store handlers.AgentStore,
	anthropicURL string,
) http.Handler {
	t.Helper()
	authCfg := auth.Config{
		SupabaseURL: supabaseURL,
		AnonKey:     "test-anon-key",
		JWTSecret:   integTestJWTSecret,
		Port:        "0",
	}
	return auth.AuthMiddleware(authCfg)(
		middleware.TenantMiddleware(querier)(
			middleware.RequireRole(minRole)(
				handlers.CreateAgent(store, anthropicURL),
			),
		),
	)
}

// managerQuerier returns a stub querier granting RoleManager for the given
// org/group IDs.
func managerQuerier(orgID, groupID uuid.UUID) *stubTenantQuerier {
	return &stubTenantQuerier{
		returnCtx: middleware.TenantContext{
			OrgID:   orgID,
			GroupID: groupID,
			Role:    middleware.RoleManager,
		},
	}
}

// memberQuerier returns a stub querier granting RoleMember.
func memberQuerier(orgID, groupID uuid.UUID) *stubTenantQuerier {
	return &stubTenantQuerier{
		returnCtx: middleware.TenantContext{
			OrgID:   orgID,
			GroupID: groupID,
			Role:    middleware.RoleMember,
		},
	}
}

// postAgents performs a POST /v1/agents request against the given handler.
func postAgents(t *testing.T, h http.Handler, token string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("postAgents: marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

// assertIDResponse asserts status 201 and body {"id": "<non-empty uuid>"}.
func assertIDResponse(t *testing.T, res *http.Response) string {
	t.Helper()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d — body: %s", res.StatusCode, body)
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body is not valid JSON: %v — body: %s", err, body)
	}
	id := m["id"]
	if id == "" {
		t.Error("response body missing \"id\" field")
		return ""
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Errorf("\"id\" = %q is not a valid UUID: %v", id, err)
	}
	return id
}

// assertErrorCode asserts the response is an RFC §4.11 error envelope with
// the expected HTTP status and error code.
func assertErrorCode(t *testing.T, res *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != wantStatus {
		t.Errorf("expected HTTP %d, got %d — body: %s", wantStatus, res.StatusCode, body)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body is not valid JSON: %v — body: %s", err, body)
	}
	if env.Error.Code != wantCode {
		t.Errorf("error.code = %q, want %q — body: %s", env.Error.Code, wantCode, body)
	}
	if env.Error.Message == "" {
		t.Error("error.message must not be empty")
	}
}

// ---------------------------------------------------------------------------
// AC1 — blank source → 201 with UUID id, CLAUDE.md skeleton uploaded
// ---------------------------------------------------------------------------

func TestCreateAgent_Blank_Returns201WithID(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockAgentStore{}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		"name":   "my-agent",
		"source": "blank",
	})
	defer res.Body.Close()

	agentID := assertIDResponse(t, res)
	if agentID == "" {
		return
	}

	// CreateAgent must have been called exactly once.
	if store.createAgentCalls != 1 {
		t.Errorf("AC1: CreateAgent called %d times, want 1", store.createAgentCalls)
	}

	// The record must be scoped to the caller's org and group.
	if store.capturedRecord.OrgID != orgID {
		t.Errorf("AC1: record.OrgID = %v, want %v", store.capturedRecord.OrgID, orgID)
	}
	if store.capturedRecord.GroupID != groupID {
		t.Errorf("AC1: record.GroupID = %v, want %v", store.capturedRecord.GroupID, groupID)
	}
	if store.capturedRecord.Source != "blank" {
		t.Errorf("AC1: record.Source = %q, want \"blank\"", store.capturedRecord.Source)
	}

	// The uploaded CLAUDE.md must contain the agent name and skeleton marker.
	content := string(store.capturedContent)
	if !strings.Contains(content, "my-agent") {
		t.Errorf("AC1: CLAUDE.md does not contain agent name \"my-agent\" — content: %s", content)
	}
	if !strings.Contains(content, "# Agent") {
		t.Errorf("AC1: CLAUDE.md missing '# Agent' heading — content: %s", content)
	}
}

// ---------------------------------------------------------------------------
// AC2 — RoleMember caller → 403 forbidden, handler never called
// ---------------------------------------------------------------------------

func TestCreateAgent_RoleMember_Returns403(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockAgentStore{}
	querier := memberQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t) // JWT is valid but role is member
	res := postAgents(t, h, tok, map[string]any{
		"name":   "test",
		"source": "blank",
	})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusForbidden, "forbidden")

	if store.createAgentCalls != 0 {
		t.Errorf("AC2: CreateAgent must not be called for a forbidden request; got %d calls",
			store.createAgentCalls)
	}
}

// ---------------------------------------------------------------------------
// AC3 — template source with valid template_id → 201, content from template
// ---------------------------------------------------------------------------

func TestCreateAgent_Template_Returns201(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	templateContent := []byte("# Agent: from-template\n\nGenerated from sales template.")
	store := &mockAgentStore{
		templateContent: templateContent,
	}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		"name":        "from-template",
		"source":      "template",
		"template_id": "sales",
	})
	defer res.Body.Close()

	assertIDResponse(t, res)

	if store.createAgentCalls != 1 {
		t.Errorf("AC3: CreateAgent called %d times, want 1", store.createAgentCalls)
	}
	if store.capturedRecord.Source != "template" {
		t.Errorf("AC3: record.Source = %q, want \"template\"", store.capturedRecord.Source)
	}
	// The uploaded content must match what the template store returned.
	if !bytes.Equal(store.capturedContent, templateContent) {
		t.Errorf("AC3: uploaded content = %q, want %q",
			store.capturedContent, templateContent)
	}
}

// ---------------------------------------------------------------------------
// AC4 — template source, missing template_id → 400 invalid_request
// ---------------------------------------------------------------------------

func TestCreateAgent_Template_MissingTemplateID_Returns400(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockAgentStore{}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		"name":   "t",
		"source": "template",
		// template_id deliberately omitted
	})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusBadRequest, "invalid_request")

	if store.createAgentCalls != 0 {
		t.Errorf("AC4: CreateAgent must not be called on validation error")
	}
}

// ---------------------------------------------------------------------------
// AC5 — template source, template not found → 404 not_found
// ---------------------------------------------------------------------------

func TestCreateAgent_Template_NotFound_Returns404(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockAgentStore{
		templateErr: handlers.ErrTemplateNotFound,
	}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		"name":        "t",
		"source":      "template",
		"template_id": "nonexistent",
	})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusNotFound, "not_found")
}

// ---------------------------------------------------------------------------
// AC6 — ai-assist with valid anthropic key → 201, CLAUDE.md uploaded
// ---------------------------------------------------------------------------

func TestCreateAgent_AIAssist_Returns201(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()

	// Stub Anthropic API: returns a minimal messages response.
	mockAnthropicContent := "# Agent: ai-agent\n\nAI-generated instructions."
	mockAnthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		resp := fmt.Sprintf(`{
			"content": [{"type": "text", "text": %q}],
			"stop_reason": "end_turn"
		}`, mockAnthropicContent)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, resp)
	}))
	defer mockAnthropic.Close()

	store := &mockAgentStore{
		anthropicKey: "sk-ant-test-key",
	}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, mockAnthropic.URL)

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		"name":      "ai-agent",
		"source":    "ai-assist",
		"ai_prompt": "sales assistant for B2B SaaS",
	})
	defer res.Body.Close()

	assertIDResponse(t, res)

	if store.createAgentCalls != 1 {
		t.Errorf("AC6: CreateAgent called %d times, want 1", store.createAgentCalls)
	}
	// The uploaded content must come from the Anthropic response.
	if !strings.Contains(string(store.capturedContent), "# Agent") {
		t.Errorf("AC6: uploaded CLAUDE.md missing expected content — got: %s",
			store.capturedContent)
	}
}

// ---------------------------------------------------------------------------
// AC7 — ai-assist, org has no anthropic key → 402 payment_required
// ---------------------------------------------------------------------------

func TestCreateAgent_AIAssist_NoCredentials_Returns402(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockAgentStore{
		anthropicErr: handlers.ErrNoCredentials,
	}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		"name":   "t",
		"source": "ai-assist",
	})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusPaymentRequired, "payment_required")

	if store.createAgentCalls != 0 {
		t.Errorf("AC7: CreateAgent must not be called when credentials are missing")
	}
}

// ---------------------------------------------------------------------------
// AC8 — github source, valid URL, CLAUDE.md exists → 201, content from GitHub
// ---------------------------------------------------------------------------

func TestCreateAgent_GitHub_Returns201(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()

	githubCLAUDE := "# Agent: github-agent\n\nLoaded from GitHub."

	// Stub GitHub raw content server.
	mockGitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler expects a request for CLAUDE.md at the HEAD ref.
		if !strings.HasSuffix(r.URL.Path, "/CLAUDE.md") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, githubCLAUDE)
	}))
	defer mockGitHub.Close()

	store := &mockAgentStore{}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)

	// Provide a github_url that resolves to our mock server.
	// The handler should fetch CLAUDE.md from the raw URL derived from this.
	// We use a special test URL format the handler will understand:
	// for tests, pass the mock server URL directly as github_url.
	res := postAgents(t, h, tok, map[string]any{
		"name":       "github-agent",
		"source":     "github",
		"github_url": mockGitHub.URL + "/owner/repo",
	})
	defer res.Body.Close()

	assertIDResponse(t, res)

	if store.createAgentCalls != 1 {
		t.Errorf("AC8: CreateAgent called %d times, want 1", store.createAgentCalls)
	}
	if !strings.Contains(string(store.capturedContent), "github-agent") {
		t.Errorf("AC8: uploaded content does not contain expected text — got: %s",
			store.capturedContent)
	}
}

// ---------------------------------------------------------------------------
// AC9 — github source, URL returns 404 → 422 unprocessable
// ---------------------------------------------------------------------------

func TestCreateAgent_GitHub_NotFound_Returns422(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()

	// Mock GitHub that always returns 404.
	mockGitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer mockGitHub.Close()

	store := &mockAgentStore{}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		"name":       "t",
		"source":     "github",
		"github_url": mockGitHub.URL + "/owner/missing-repo",
	})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusUnprocessableEntity, "unprocessable")

	if store.createAgentCalls != 0 {
		t.Errorf("AC9: CreateAgent must not be called when GitHub returns 404")
	}
}

// ---------------------------------------------------------------------------
// AC10 — missing name → 400 invalid_request
// ---------------------------------------------------------------------------

func TestCreateAgent_MissingName_Returns400(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockAgentStore{}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		// name deliberately omitted
		"source": "blank",
	})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusBadRequest, "invalid_request")
}

// ---------------------------------------------------------------------------
// AC11 — invalid source value → 400 invalid_request
// ---------------------------------------------------------------------------

func TestCreateAgent_InvalidSource_Returns400(t *testing.T) {
	orgID, groupID := uuid.New(), uuid.New()
	store := &mockAgentStore{}
	querier := managerQuerier(orgID, groupID)

	h := testChain(t, "https://fake.supabase.co", middleware.RoleManager, querier, store, "")

	tok, _ := validManagerJWT(t)
	res := postAgents(t, h, tok, map[string]any{
		"name":   "t",
		"source": "magic",
	})
	defer res.Body.Close()

	assertErrorCode(t, res, http.StatusBadRequest, "invalid_request")
}

// ---------------------------------------------------------------------------
// Compile check — typed symbol references
// The following blank identifiers ensure every public symbol declared in
// internal/handlers/agents.go is referenced in this file, so a missing
// type causes a compile error rather than a silent no-op.
// ---------------------------------------------------------------------------

var (
	_ handlers.AgentStore  = (*mockAgentStore)(nil)
	_ handlers.AgentRecord = handlers.AgentRecord{}
	_ error                = handlers.ErrTemplateNotFound
	_ error                = handlers.ErrNoCredentials
)
