package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// ClaudeRunner runs the `claude` (Claude Code) CLI as a local subprocess.
// It satisfies the handlers.Runner interface structurally.
type ClaudeRunner struct {
	// Bin is the binary to exec; defaults to "claude".
	Bin string
}

// NewClaudeRunner constructs a ClaudeRunner using the `claude` binary on PATH.
func NewClaudeRunner() *ClaudeRunner {
	return &ClaudeRunner{Bin: "claude"}
}

// Run executes `claude --print <prompt>` in workspaceDir with ANTHROPIC_API_KEY
// injected into the environment, and returns captured stdout.
//
// `--print` runs Claude Code non-interactively (one-shot) and exits. The exact
// flag may need adjustment to the installed `claude` version; the Runner
// interface isolates callers from that detail.
func (c *ClaudeRunner) Run(ctx context.Context, workspaceDir, prompt, apiKey string) (string, error) {
	bin := c.Bin
	if bin == "" {
		bin = "claude"
	}

	cmd := exec.CommandContext(ctx, bin, "--print", prompt)
	cmd.Dir = workspaceDir
	cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+apiKey)

	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("claude run failed: %w: %s", err, ee.Stderr)
		}
		return "", fmt.Errorf("claude run failed: %w", err)
	}
	return string(out), nil
}
