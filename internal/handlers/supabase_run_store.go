package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
)

// SupabaseRunStore implements RunStore against Supabase PostgREST + Storage.
// All calls use the caller's JWT via auth.NewUserClient — never service_role.
type SupabaseRunStore struct {
	SupabaseURL string
	AnonKey     string
}

// NewSupabaseRunStore constructs a SupabaseRunStore.
func NewSupabaseRunStore(supabaseURL, anonKey string) *SupabaseRunStore {
	return &SupabaseRunStore{SupabaseURL: supabaseURL, AnonKey: anonKey}
}

// GetAgent selects the agent by id under the caller's JWT (RLS filters it).
func (s *SupabaseRunStore) GetAgent(ctx context.Context, jwt string, agentID uuid.UUID) (AgentRef, error) {
	url := fmt.Sprintf("%s/rest/v1/agents?id=eq.%s&select=id,org_id,group_id", s.SupabaseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return AgentRef{}, fmt.Errorf("build get agent request: %w", err)
	}
	req.Header.Set("apikey", s.AnonKey)

	resp, err := auth.NewUserClient(jwt).Do(req)
	if err != nil {
		return AgentRef{}, fmt.Errorf("get agent: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return AgentRef{}, fmt.Errorf("get agent: unexpected status %d", resp.StatusCode)
	}

	var rows []struct {
		ID      string `json:"id"`
		OrgID   string `json:"org_id"`
		GroupID string `json:"group_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return AgentRef{}, fmt.Errorf("decode agent: %w", err)
	}
	if len(rows) == 0 {
		return AgentRef{}, ErrAgentNotFound
	}

	id, err := uuid.Parse(rows[0].ID)
	if err != nil {
		return AgentRef{}, fmt.Errorf("invalid agent id: %w", err)
	}
	orgID, err := uuid.Parse(rows[0].OrgID)
	if err != nil {
		return AgentRef{}, fmt.Errorf("invalid org id: %w", err)
	}
	groupID, err := uuid.Parse(rows[0].GroupID)
	if err != nil {
		return AgentRef{}, fmt.Errorf("invalid group id: %w", err)
	}
	return AgentRef{ID: id, OrgID: orgID, GroupID: groupID}, nil
}

// CreateRun inserts a runs row (status pending) with a client-generated id.
func (s *SupabaseRunStore) CreateRun(ctx context.Context, jwt string, r RunRecord) (uuid.UUID, error) {
	runID := uuid.New()
	body, _ := json.Marshal(map[string]string{
		"id":       runID.String(),
		"agent_id": r.AgentID.String(),
		"org_id":   r.OrgID.String(),
		"group_id": r.GroupID.String(),
		"user_id":  r.UserID.String(),
		"prompt":   r.Prompt,
		"status":   "pending",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.SupabaseURL+"/rest/v1/runs", bytes.NewReader(body))
	if err != nil {
		return uuid.Nil, fmt.Errorf("build create run request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.AnonKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := auth.NewUserClient(jwt).Do(req)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create run: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return uuid.Nil, fmt.Errorf("create run: status %d: %s", resp.StatusCode, raw)
	}
	return runID, nil
}

// UpdateRun PATCHes the run's status (+ result).
func (s *SupabaseRunStore) UpdateRun(ctx context.Context, jwt string, runID uuid.UUID, status, result string) error {
	body, _ := json.Marshal(map[string]string{"status": status, "result": result})
	url := fmt.Sprintf("%s/rest/v1/runs?id=eq.%s", s.SupabaseURL, runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build update run request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.AnonKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := auth.NewUserClient(jwt).Do(req)
	if err != nil {
		return fmt.Errorf("update run: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("update run: status %d", resp.StatusCode)
	}
	return nil
}

// DownloadContext downloads the agent's Storage objects (+ the org `general`
// prefix) into destDir. Tolerant: a missing/empty prefix yields no error (the
// agent simply runs with whatever context exists).
func (s *SupabaseRunStore) DownloadContext(ctx context.Context, jwt string, a AgentRef, destDir string) error {
	client := auth.NewUserClient(jwt)
	prefixes := []string{
		fmt.Sprintf("%s/%s/agents/%s", a.OrgID, a.GroupID, a.ID),
		fmt.Sprintf("%s/general", a.OrgID),
	}
	for _, prefix := range prefixes {
		names, err := s.listObjects(ctx, client, prefix)
		if err != nil {
			return err
		}
		for _, name := range names {
			if err := s.downloadObject(ctx, client, prefix+"/"+name, filepath.Join(destDir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SupabaseRunStore) listObjects(ctx context.Context, client *http.Client, prefix string) ([]string, error) {
	body, _ := json.Marshal(map[string]any{"prefix": prefix + "/", "limit": 100})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.SupabaseURL+"/storage/v1/object/list/houston", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build list request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.AnonKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list storage %s: %w", prefix, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil // tolerate empty/forbidden prefix
	}

	var items []struct {
		Name string  `json:"name"`
		ID   *string `json:"id"` // folders have id == null
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode list %s: %w", prefix, err)
	}
	var names []string
	for _, it := range items {
		if it.Name == "" || it.ID == nil {
			continue // skip pseudo-folders
		}
		names = append(names, it.Name)
	}
	return names, nil
}

func (s *SupabaseRunStore) downloadObject(ctx context.Context, client *http.Client, objectName, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.SupabaseURL+"/storage/v1/object/houston/"+objectName, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("apikey", s.AnonKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", objectName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil // tolerate missing object
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s: %w", objectName, err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		return fmt.Errorf("mkdir for %s: %w", destPath, err)
	}
	return os.WriteFile(destPath, data, 0o600)
}
