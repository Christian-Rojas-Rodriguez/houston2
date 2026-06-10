// Package runtime provides the per-run isolated workspace and the Claude Code
// subprocess runner for the run-agent flow (Task 0008).
package runtime

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Workspace is an isolated per-run temporary directory hydrated with the
// agent's Storage context before the claude subprocess runs.
type Workspace struct {
	Dir string
}

// NewWorkspace creates /tmp/houston-run-{runID}/ (0700). The caller must call
// Cleanup when the run finishes.
func NewWorkspace(runID uuid.UUID) (*Workspace, error) {
	dir := filepath.Join(os.TempDir(), "houston-run-"+runID.String())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Workspace{Dir: dir}, nil
}

// Cleanup removes the workspace directory and its contents. Idempotent.
func (w *Workspace) Cleanup() error {
	if w == nil || w.Dir == "" {
		return nil
	}
	return os.RemoveAll(w.Dir)
}
