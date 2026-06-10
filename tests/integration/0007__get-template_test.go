//go:build integration

// Integration test for the R-TEMPLATES fix (Task 0007).
//
// SupabaseAgentStore.GetTemplate previously read from a non-existent bucket
// `templates`; Task 0003 stores templates in the single bucket `houston` under
// name `templates/{id}/CLAUDE.md` (covered by the templates_readonly policy).
// This test exercises the REAL Storage path (the unit suite mocks AgentStore,
// which is why the bug went unnoticed — same blind spot as R-STORAGE).
//
// RED  (pre-fix):  GetTemplate hits /storage/v1/object/templates/... -> bucket
//                  missing -> error.
// GREEN (post-fix): /storage/v1/object/houston/templates/... -> content.
//
// Run with a live local Supabase:
//
//	SUPABASE_URL=http://127.0.0.1:54321 \
//	SUPABASE_SERVICE_ROLE_KEY=<key> SUPABASE_ANON_KEY=<key> \
//	go test -tags integration ./tests/integration/ -run GetTemplate -v

package integration_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/nomenclator/houston2/internal/handlers"
	"github.com/nomenclator/houston2/tests/helpers"
)

func TestGetTemplate_ReadsFromHoustonBucket(t *testing.T) {
	// supabaseEnv (defined in 0010__seed-fixture_test.go) skips if URL/service key unset.
	url, serviceRoleKey := supabaseEnv(t)
	anonKey := os.Getenv("SUPABASE_ANON_KEY")
	if anonKey == "" {
		t.Skip("SUPABASE_ANON_KEY not set — skipping GetTemplate integration test")
	}

	// Seed the fixture so a real auth user (UserA) exists for password-grant JWT.
	ids := helpers.Seed(t, url, serviceRoleKey)

	// Seed the template object into bucket houston via service_role (idempotent).
	const templateID = "sales"
	const wantBody = "# Sales Agent Template\n\nIntegration fixture for 0007 R-TEMPLATES.\n"
	seedTemplateObject(t, url, serviceRoleKey, templateID, wantBody)

	// Mint a real Supabase JWT for an authenticated user. templates_readonly
	// grants SELECT on name LIKE 'templates/%' to any authenticated role.
	jwt := helpers.UserJWT(t, url, serviceRoleKey, ids.UserIDA)

	store := handlers.NewSupabaseAgentStore(url, anonKey)
	got, err := store.GetTemplate(context.Background(), jwt, templateID)
	if err != nil {
		t.Fatalf("GetTemplate(%q) error = %v; R-TEMPLATES: must read from bucket 'houston'", templateID, err)
	}
	if string(got) != wantBody {
		t.Fatalf("GetTemplate body mismatch:\n got: %q\nwant: %q", got, wantBody)
	}
}

// seedTemplateObject uploads templates/{id}/CLAUDE.md to bucket houston via service_role.
func seedTemplateObject(t *testing.T, url, serviceRoleKey, templateID, body string) {
	t.Helper()
	path := url + "/storage/v1/object/houston/templates/" + templateID + "/CLAUDE.md"
	req, err := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build template upload: %v", err)
	}
	req.Header.Set("apikey", serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	req.Header.Set("Content-Type", "text/markdown")
	req.Header.Set("x-upsert", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("template upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("template upload: unexpected status %d", resp.StatusCode)
	}
}
