package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
)

// SupabaseAgentStore implements AgentStore against the Supabase REST API.
// All calls use the caller's JWT via auth.NewUserClient — never a service_role key.
type SupabaseAgentStore struct {
	SupabaseURL string
	AnonKey     string
}

// NewSupabaseAgentStore constructs a SupabaseAgentStore.
func NewSupabaseAgentStore(supabaseURL, anonKey string) *SupabaseAgentStore {
	return &SupabaseAgentStore{SupabaseURL: supabaseURL, AnonKey: anonKey}
}

// CreateAgent inserts the agent row into Postgres via PostgREST and uploads
// claudeMD to Supabase Storage. Both calls use the user's JWT so RLS applies.
func (s *SupabaseAgentStore) CreateAgent(ctx context.Context, jwt string, a AgentRecord, claudeMD []byte) error {
	body, _ := json.Marshal(a)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.SupabaseURL+"/rest/v1/agents", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build insert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.AnonKey)
	req.Header.Set("Prefer", "return=minimal")

	client := auth.NewUserClient(jwt)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("insert agent: unexpected status %d", resp.StatusCode)
	}

	// Upload CLAUDE.md to Storage.
	storagePath := fmt.Sprintf("/storage/v1/object/houston/%s/%s/agents/%s/CLAUDE.md",
		a.OrgID, a.GroupID, a.ID)
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.SupabaseURL+storagePath, bytes.NewReader(claudeMD))
	if err != nil {
		return fmt.Errorf("build upload request: %w", err)
	}
	uploadReq.Header.Set("Content-Type", "text/plain")
	uploadReq.Header.Set("apikey", s.AnonKey)

	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		return fmt.Errorf("upload CLAUDE.md: %w", err)
	}
	uploadResp.Body.Close()
	if uploadResp.StatusCode >= 300 {
		return fmt.Errorf("upload CLAUDE.md: unexpected status %d", uploadResp.StatusCode)
	}
	return nil
}

// GetTemplate fetches templates/{templateID}/CLAUDE.md from Supabase Storage.
func (s *SupabaseAgentStore) GetTemplate(ctx context.Context, jwt string, templateID string) ([]byte, error) {
	path := fmt.Sprintf("/storage/v1/object/templates/%s/CLAUDE.md", templateID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.SupabaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build template request: %w", err)
	}
	req.Header.Set("apikey", s.AnonKey)

	client := auth.NewUserClient(jwt)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrTemplateNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch template: unexpected status %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	return buf.Bytes(), nil
}

// GetAnthropicKey calls the get_org_anthropic_key security-definer RPC.
// Returns ErrNoCredentials when the org has no key configured.
func (s *SupabaseAgentStore) GetAnthropicKey(ctx context.Context, jwt string, orgID uuid.UUID) (string, error) {
	body, _ := json.Marshal(map[string]string{"org_id": orgID.String()})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.SupabaseURL+"/rest/v1/rpc/get_org_anthropic_key", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.AnonKey)

	client := auth.NewUserClient(jwt)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch anthropic key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return "", ErrNoCredentials
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch anthropic key: unexpected status %d", resp.StatusCode)
	}

	var result string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", ErrNoCredentials
	}
	if result == "" {
		return "", ErrNoCredentials
	}
	return result, nil
}
