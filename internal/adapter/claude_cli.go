package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// ClaudeCLI implements AdapterClient by invoking the `claude` CLI in print mode.
// Each Prompt() call runs: claude -p --output-format json --permission-mode <mode>
// The CLI inherits the parent process environment (including ANTHROPIC_API_KEY if set)
// and uses locally-authenticated credentials (~/.claude).
type ClaudeCLI struct {
	binary   string
	model    string
	tools    []string
	permMode string
	cwd      string // workspace directory
	mu       sync.Mutex
	proc     *os.Process // current running process for cancellation
	updates  chan *Message
}

// ClaudeCLIConfig configures the Claude CLI adapter.
type ClaudeCLIConfig struct {
	Binary         string   // path to claude binary (default: "claude")
	Model          string   // model override (e.g., "sonnet", "opus")
	AllowedTools   []string // restrict tools (e.g., ["Read", "Edit", "Bash"])
	PermissionMode string   // "bypassPermissions", "default", etc.
	Cwd            string   // workspace directory for command execution
}

// CLIResult is the parsed JSON output from claude -p --output-format json.
type CLIResult struct {
	Type             string         `json:"type"`
	Subtype          string         `json:"subtype"`
	IsError          bool           `json:"is_error"`
	Result           string         `json:"result"`
	StopReason       string         `json:"stop_reason"`
	NumTurns         int            `json:"num_turns"`
	SessionID        string         `json:"session_id"`
	DurationMs       int            `json:"duration_ms"`
	DurationAPIMs    int            `json:"duration_api_ms"`
	TotalCostUSD     float64        `json:"total_cost_usd"`
	Usage            map[string]any `json:"usage"`
	PermissionDenials []any         `json:"permission_denials"`
}

// NewClaudeCLI creates a Claude CLI adapter.
func NewClaudeCLI(cfg ClaudeCLIConfig) *ClaudeCLI {
	binary := cfg.Binary
	if binary == "" {
		binary = "claude"
	}
	permMode := cfg.PermissionMode
	if permMode == "" {
		permMode = "bypassPermissions"
	}
	return &ClaudeCLI{
		binary:   binary,
		model:    cfg.Model,
		tools:    cfg.AllowedTools,
		permMode: permMode,
		cwd:      cfg.Cwd,
		updates:  make(chan *Message, 10),
	}
}

func (c *ClaudeCLI) Initialize(_ context.Context) (*InitResult, error) {
	// Verify claude binary exists on PATH
	path, err := exec.LookPath(c.binary)
	if err != nil {
		return nil, fmt.Errorf("adapter_not_found: claude binary %q not found on PATH: %w", c.binary, err)
	}
	slog.Info("claude CLI adapter initialized", "binary", path)
	return &InitResult{
		Provider:     "claude_code",
		Capabilities: map[string]any{"sessionReuse": false, "tokenUsage": true},
	}, nil
}

func (c *ClaudeCLI) NewSession(_ context.Context, params SessionParams) (string, error) {
	// CLI mode doesn't have persistent sessions — return a UUID for tracking
	id := uuid.New().String()
	slog.Info("claude CLI session created", "session_id", id, "cwd", params.Cwd)
	return id, nil
}

func (c *ClaudeCLI) Prompt(ctx context.Context, sessionID string, text string) (*PromptResult, error) {
	args := []string{"-p", "--output-format", "json"}

	if c.permMode != "" {
		args = append(args, "--permission-mode", c.permMode)
	}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	if len(c.tools) > 0 {
		args = append(args, "--allowed-tools", strings.Join(c.tools, ","))
	}

	cmd := exec.CommandContext(ctx, c.binary, args...)
	cmd.Dir = c.cwd
	cmd.Stdin = strings.NewReader(text)
	cmd.Env = os.Environ()

	// Capture stdout via pipe so we can track the process for cancellation
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &PromptResult{StopReason: StopFailed, Summary: fmt.Sprintf("stdout pipe: %v", err)}, nil
	}

	slog.Info("claude CLI executing",
		"session_id", sessionID,
		"model", c.model,
		"perm_mode", c.permMode,
		"prompt_len", len(text),
	)

	if err := cmd.Start(); err != nil {
		return &PromptResult{StopReason: StopFailed, Summary: fmt.Sprintf("start: %v", err)}, nil
	}

	// Track the process for cancellation AFTER start
	c.mu.Lock()
	c.proc = cmd.Process
	c.mu.Unlock()

	// Read all output
	output, readErr := io.ReadAll(stdout)

	// Wait for process to finish
	waitErr := cmd.Wait()

	c.mu.Lock()
	c.proc = nil
	c.mu.Unlock()

	if readErr != nil {
		slog.Error("claude CLI read failed", "session_id", sessionID, "error", readErr)
		return &PromptResult{StopReason: StopFailed, Summary: fmt.Sprintf("read: %v", readErr)}, nil
	}

	if waitErr != nil {
		var stderr string
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		slog.Error("claude CLI failed",
			"session_id", sessionID,
			"error", waitErr,
			"stderr", stderr,
		)
		return &PromptResult{
			StopReason: StopFailed,
			Summary:    fmt.Sprintf("claude CLI error: %v\n%s", waitErr, stderr),
		}, nil
	}

	// Parse JSON result
	var result CLIResult
	if err := json.Unmarshal(output, &result); err != nil {
		slog.Error("claude CLI output parse failed",
			"session_id", sessionID,
			"output_len", len(output),
			"error", err,
		)
		return &PromptResult{
			StopReason: StopFailed,
			Summary:    fmt.Sprintf("failed to parse claude output: %v", err),
		}, nil
	}

	slog.Info("claude CLI completed",
		"session_id", sessionID,
		"stop_reason", result.StopReason,
		"num_turns", result.NumTurns,
		"cost_usd", result.TotalCostUSD,
		"duration_ms", result.DurationMs,
		"is_error", result.IsError,
	)

	// Map stop reason
	stopReason := mapCLIStopReason(result)

	return &PromptResult{
		StopReason: stopReason,
		Summary:    truncate(result.Result, 500),
	}, nil
}

func (c *ClaudeCLI) Cancel(_ context.Context, _ string) error {
	c.mu.Lock()
	proc := c.proc
	c.mu.Unlock()
	if proc != nil {
		return proc.Kill()
	}
	return nil
}

func (c *ClaudeCLI) CloseSession(_ context.Context, _ string) error {
	return nil // no-op for CLI mode
}

func (c *ClaudeCLI) Close() error {
	return nil // no-op
}

func (c *ClaudeCLI) Updates() <-chan *Message {
	return c.updates
}

func (c *ClaudeCLI) PID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.proc != nil {
		return c.proc.Pid
	}
	return 0
}

func mapCLIStopReason(result CLIResult) StopReason {
	if result.IsError {
		return StopFailed
	}
	switch result.StopReason {
	case "end_turn":
		return StopCompleted
	case "stop_sequence":
		return StopCompleted
	case "max_tokens":
		return StopCompleted
	default:
		if result.Subtype == "error" {
			return StopFailed
		}
		return StopCompleted
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
