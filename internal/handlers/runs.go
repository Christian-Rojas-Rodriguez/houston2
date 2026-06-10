package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/middleware"
	"github.com/nomenclator/houston2/internal/runtime"
)

// ErrAgentNotFound is returned by RunStore.GetAgent when the agent is not
// visible to the caller (RLS) or does not exist.
var ErrAgentNotFound = errors.New("agent not found")

// AgentRef identifies an agent and its tenant placement.
type AgentRef struct {
	ID      uuid.UUID
	OrgID   uuid.UUID
	GroupID uuid.UUID
}

// RunRecord is the data persisted for a new run.
type RunRecord struct {
	AgentID uuid.UUID
	OrgID   uuid.UUID
	GroupID uuid.UUID
	UserID  uuid.UUID
	Prompt  string
}

// RunStore abstracts all DB + Storage I/O for the run-agent flow.
// Implemented by SupabaseRunStore; mockable in tests.
type RunStore interface {
	// GetAgent verifies access (RLS) and returns the agent's tenant placement.
	// Returns ErrAgentNotFound when the agent is not visible to the caller.
	GetAgent(ctx context.Context, jwt string, agentID uuid.UUID) (AgentRef, error)
	// CreateRun inserts a runs row (status pending) and returns its id.
	CreateRun(ctx context.Context, jwt string, r RunRecord) (uuid.UUID, error)
	// UpdateRun sets the run's status (+ result).
	UpdateRun(ctx context.Context, jwt string, runID uuid.UUID, status, result string) error
	// DownloadContext hydrates destDir with the agent's Storage objects.
	DownloadContext(ctx context.Context, jwt string, a AgentRef, destDir string) error
}

// Runner executes the agent (a `claude` subprocess) in an isolated workspace.
type Runner interface {
	Run(ctx context.Context, workspaceDir, prompt, apiKey string) (string, error)
}

type createRunRequest struct {
	Prompt string `json:"prompt"`
}

const (
	statusPending = "pending"
	statusRunning = "running"
	statusDone    = "done"
	statusError   = "error"
)

// CreateRun returns an http.HandlerFunc for POST /v1/agents/{id}/runs.
// apiKey is the server's ANTHROPIC_API_KEY, injected into the claude subprocess.
func CreateRun(store RunStore, runner Runner, apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tc, ok := middleware.TenantFromContext(r.Context())
		if !ok {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "tenant context missing")
			return
		}
		_ = tc // role/tenant already enforced by middleware; run scopes to the agent
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "user id missing")
			return
		}
		jwt, _ := auth.JWTFromContext(r.Context())

		agentID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "invalid agent id in path")
			return
		}

		var req createRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
		if req.Prompt == "" {
			writeHandlerError(w, r, http.StatusBadRequest, "invalid_request", "prompt is required")
			return
		}

		// Verify access to the agent (RLS) and obtain its tenant placement.
		ref, err := store.GetAgent(r.Context(), jwt, agentID)
		if err != nil {
			if errors.Is(err, ErrAgentNotFound) {
				writeHandlerError(w, r, http.StatusNotFound, "not_found", "agent not found")
				return
			}
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to load agent")
			return
		}

		runID, err := store.CreateRun(r.Context(), jwt, RunRecord{
			AgentID: ref.ID,
			OrgID:   ref.OrgID,
			GroupID: ref.GroupID,
			UserID:  userID,
			Prompt:  req.Prompt,
		})
		if err != nil {
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to create run")
			return
		}

		// Server misconfiguration: no API key → mark run error, 502.
		if apiKey == "" {
			_ = store.UpdateRun(r.Context(), jwt, runID, statusError, "ANTHROPIC_API_KEY not configured on server")
			writeHandlerError(w, r, http.StatusBadGateway, "upstream_error", "ANTHROPIC_API_KEY not configured on server")
			return
		}

		_ = store.UpdateRun(r.Context(), jwt, runID, statusRunning, "")

		// Isolated workspace per run (cleaned up on return, even on error/panic).
		ws, err := runtime.NewWorkspace(runID)
		if err != nil {
			_ = store.UpdateRun(r.Context(), jwt, runID, statusError, "failed to create workspace")
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to create workspace")
			return
		}
		defer ws.Cleanup()

		// Hydrate the workspace with the agent's Storage context (RLS in force).
		if err := store.DownloadContext(r.Context(), jwt, ref, ws.Dir); err != nil {
			_ = store.UpdateRun(r.Context(), jwt, runID, statusError, "failed to gather context: "+err.Error())
			writeHandlerError(w, r, http.StatusInternalServerError, "internal_error", "failed to gather agent context")
			return
		}

		// Launch the real claude subprocess with the key injected.
		output, err := runner.Run(r.Context(), ws.Dir, req.Prompt, apiKey)
		if err != nil {
			_ = store.UpdateRun(r.Context(), jwt, runID, statusError, err.Error())
			writeHandlerError(w, r, http.StatusBadGateway, "upstream_error", "agent run failed")
			return
		}

		_ = store.UpdateRun(r.Context(), jwt, runID, statusDone, output)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"run_id": runID.String(),
			"status": statusDone,
			"result": output,
		})
	}
}
