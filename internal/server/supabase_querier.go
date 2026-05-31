package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/middleware"
)

// supabaseQuerier satisfies middleware.TenantQuerier by calling the
// current_tenant() security-definer function via Supabase PostgREST.
// It uses the user's own JWT (via auth.NewUserClient) — never the service_role key.
type supabaseQuerier struct {
	supabaseURL string
	anonKey     string
}

// NewSupabaseQuerier creates a TenantQuerier backed by Supabase PostgREST.
func NewSupabaseQuerier(supabaseURL, anonKey string) middleware.TenantQuerier {
	return &supabaseQuerier{supabaseURL: supabaseURL, anonKey: anonKey}
}

func (q *supabaseQuerier) CurrentTenant(ctx context.Context, userID uuid.UUID) (middleware.TenantContext, error) {
	jwt, ok := auth.JWTFromContext(ctx)
	if !ok {
		return middleware.TenantContext{}, fmt.Errorf("supabaseQuerier: no JWT in context")
	}

	client := auth.NewUserClient(jwt)
	url := q.supabaseURL + "/rest/v1/rpc/current_tenant"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return middleware.TenantContext{}, fmt.Errorf("supabaseQuerier: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", q.anonKey)

	resp, err := client.Do(req)
	if err != nil {
		return middleware.TenantContext{}, fmt.Errorf("supabaseQuerier: do request: %w", err)
	}
	defer resp.Body.Close()

	var rows []struct {
		OrgID   string `json:"org_id"`
		GroupID string `json:"group_id"`
		Role    string `json:"role"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return middleware.TenantContext{}, fmt.Errorf("supabaseQuerier: decode: %w", err)
	}
	if len(rows) == 0 {
		return middleware.TenantContext{}, middleware.ErrNoMembership
	}

	orgID, err := uuid.Parse(rows[0].OrgID)
	if err != nil {
		return middleware.TenantContext{}, fmt.Errorf("supabaseQuerier: invalid org_id: %w", err)
	}
	groupID, err := uuid.Parse(rows[0].GroupID)
	if err != nil {
		return middleware.TenantContext{}, fmt.Errorf("supabaseQuerier: invalid group_id: %w", err)
	}
	role, err := middleware.ParseRole(rows[0].Role)
	if err != nil {
		return middleware.TenantContext{}, fmt.Errorf("supabaseQuerier: invalid role: %w", err)
	}

	return middleware.TenantContext{OrgID: orgID, GroupID: groupID, Role: role}, nil
}
