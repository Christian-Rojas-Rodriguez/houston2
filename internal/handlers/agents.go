package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/middleware"
)

// AgentStore is the single port for all storage and DB operations in this handler.
var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrNoCredentials    = errors.New("org has no anthropic key")
)

// AgentRecord is the data the store persists for a new agent.
type AgentRecord struct {
	ID      uuid.UUID `json:"id"`
	OrgID   uuid.UUID `json:"org_id"`
	GroupID uuid.UUID `json:"group_id"`
	Name    string    `json:"name"`
	Source  string    `json:"source"`
	Config  any       `json:"config,omitempty"`
}

// AgentStore abstracts all external I/O for the CreateAgent handler.
type AgentStore interface {
	// CreateAgent inserts the agent row into the DB and uploads claudeMD to
	// Storage at houston/{orgID}/{groupID}/agents/{agentID}/CLAUDE.md.
	CreateAgent(ctx context.Context, jwt string, a AgentRecord, claudeMD []byte) error

	// GetTemplate fetches templates/{templateID}/CLAUDE.md from Storage.
	// Returns ErrTemplateNotFound when the object does not exist.
	GetTemplate(ctx context.Context, jwt string, templateID string) ([]byte, error)

	// GetAnthropicKey returns the org's plaintext Anthropic key.
	// Returns ErrNoCredentials when no key is configured for the org.
	GetAnthropicKey(ctx context.Context, jwt string, orgID uuid.UUID) (string, error)
}

type createAgentRequest struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	TemplateID string `json:"template_id"`
	GitHubURL  string `json:"github_url"`
	AIPrompt   string `json:"ai_prompt"`
}

var validSources = map[string]bool{
	"blank":     true,
	"template":  true,
	"ai-assist": true,
	"github":    true,
}

// CreateAgent returns an http.HandlerFunc for POST /v1/agents.
// anthropicBaseURL is the Anthropic API base (empty → "https://api.anthropic.com").
func CreateAgent(store AgentStore, anthropicBaseURL string) http.HandlerFunc {
	if anthropicBaseURL == "" {
		anthropicBaseURL = "https://api.anthropic.com"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tc, ok := middleware.TenantFromContext(r.Context())
		if !ok {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "tenant context missing")
			return
		}

		jwt, _ := auth.JWTFromContext(r.Context())

		var req createAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}

		if req.Name == "" {
			writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "name is required")
			return
		}
		if !validSources[req.Source] {
			writeHandlerError(w, r, http.StatusBadRequest, "invalid_request",
				fmt.Sprintf("source must be one of: blank, template, ai-assist, github; got %q", req.Source))
			return
		}
		if req.Source == "template" && req.TemplateID == "" {
			writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "template_id is required for source=template")
			return
		}
		if req.Source == "github" && req.GitHubURL == "" {
			writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "github_url is required for source=github")
			return
		}

		agentID := uuid.New()

		claudeMD, err := buildClaudeMD(r.Context(), req, agentID, jwt, tc, store, anthropicBaseURL)
		if err != nil {
			writeSourceError(w, r, req.Source, err)
			return
		}

		record := AgentRecord{
			ID:      agentID,
			OrgID:   tc.OrgID,
			GroupID: tc.GroupID,
			Name:    req.Name,
			Source:  req.Source,
		}

		if err := store.CreateAgent(r.Context(), jwt, record, claudeMD); err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to create agent")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": agentID.String()})
	}
}

// buildClaudeMD produces the CLAUDE.md content for the given source.
func buildClaudeMD(
	ctx context.Context,
	req createAgentRequest,
	_ uuid.UUID,
	jwt string,
	tc middleware.TenantContext,
	store AgentStore,
	anthropicBaseURL string,
) ([]byte, error) {
	switch req.Source {
	case "blank":
		return blankCLAUDEMD(req.Name), nil

	case "template":
		content, err := store.GetTemplate(ctx, jwt, req.TemplateID)
		if err != nil {
			return nil, err
		}
		return content, nil

	case "ai-assist":
		key, err := store.GetAnthropicKey(ctx, jwt, tc.OrgID)
		if err != nil {
			return nil, err
		}
		return generateCLAUDEMD(ctx, req.Name, req.AIPrompt, key, anthropicBaseURL)

	case "github":
		return fetchGitHubCLAUDEMD(ctx, req.GitHubURL)

	default:
		return nil, fmt.Errorf("unknown source: %s", req.Source)
	}
}

func blankCLAUDEMD(name string) []byte {
	return []byte(fmt.Sprintf(`# Agent: %s

> Created by Houston 2.0.

## Role

Describe the agent's role here.

## Instructions

1. Step one.
2. Step two.
`, name))
}

// generateCLAUDEMD calls the Anthropic messages API to produce CLAUDE.md content.
func generateCLAUDEMD(ctx context.Context, name, prompt, apiKey, baseURL string) ([]byte, error) {
	userMsg := fmt.Sprintf(
		"Generate a CLAUDE.md file for a Claude Code agent named %q.", name,
	)
	if prompt != "" {
		userMsg += "\n" + prompt
	}
	userMsg += "\nOutput only the CLAUDE.md file content, no explanations."

	body, _ := json.Marshal(map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 512,
		"messages": []map[string]string{
			{"role": "user", "content": userMsg},
		},
	})

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/v1/messages", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API returned %d", resp.StatusCode)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}
	for _, block := range result.Content {
		if block.Type == "text" {
			return []byte(block.Text), nil
		}
	}
	return nil, fmt.Errorf("anthropic response contained no text block")
}

// fetchGitHubCLAUDEMD fetches CLAUDE.md from a GitHub repository.
// github_url may be a full github.com URL or a test base URL + /owner/repo.
func fetchGitHubCLAUDEMD(ctx context.Context, githubURL string) ([]byte, error) {
	rawURL := resolveGitHubRawURL(githubURL)

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build github request: %w", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errGitHubNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errGitHubUpstream
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read github response: %w", err)
	}
	return content, nil
}

var (
	errGitHubNotFound = errors.New("github CLAUDE.md not found")
	errGitHubUpstream = errors.New("github upstream error")
)

// resolveGitHubRawURL converts a github.com URL or test base URL to a raw content URL.
// For https://github.com/owner/repo → https://raw.githubusercontent.com/owner/repo/HEAD/CLAUDE.md
// For test URLs (not github.com) → {url}/CLAUDE.md
func resolveGitHubRawURL(githubURL string) string {
	githubURL = strings.TrimRight(githubURL, "/")
	if strings.HasPrefix(githubURL, "https://github.com/") {
		// Convert github.com URL to raw.githubusercontent.com
		path := strings.TrimPrefix(githubURL, "https://github.com/")
		return "https://raw.githubusercontent.com/" + path + "/HEAD/CLAUDE.md"
	}
	// Test/non-github URLs: append /CLAUDE.md directly
	return githubURL + "/CLAUDE.md"
}

// writeSourceError maps source-specific errors to HTTP status codes.
func writeSourceError(w http.ResponseWriter, r *http.Request, source string, err error) {
	switch {
	case errors.Is(err, ErrTemplateNotFound):
		writeHandlerError(w, r, http.StatusNotFound, "not_found", "template not found")
	case errors.Is(err, ErrNoCredentials):
		writeHandlerError(w, r, http.StatusPaymentRequired, "payment_required", "org has no anthropic key configured")
	case errors.Is(err, errGitHubNotFound):
		writeHandlerError(w, r, http.StatusUnprocessableEntity, "unprocessable", "CLAUDE.md not found in GitHub repository")
	case errors.Is(err, errGitHubUpstream):
		writeHandlerError(w, r, http.StatusBadGateway, "upstream_error", "GitHub returned an error")
	default:
		writeHandlerError(w, r, http.StatusInternalServerError, "internal_error",
			fmt.Sprintf("failed to fetch %s content", source))
	}
}

// writeHandlerError writes an RFC §4.11 error envelope.
func writeHandlerError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":       code,
			"message":    message,
			"request_id": uuid.New().String(),
		},
	})
}
