// Package claude implements the Agent interface for Claude Code CLI.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/shivamstaq/github-symphony/internal/agent"
)

// Config configures the Claude Code agent.
type Config struct {
	Binary         string   // path to claude binary (default: "claude")
	Model          string   // model override (e.g., "sonnet", "opus")
	AllowedTools   []string // restrict tools
	PermissionMode string   // "bypassPermissions", etc.
	LogDir         string   // directory for agent logs
	SocketDir      string   // directory for attach sockets
}

// Agent implements the agent.Agent interface using Claude Code CLI with PTY.
type Agent struct {
	cfg Config
}

// New creates a Claude Code agent.
func New(cfg Config) *Agent {
	if cfg.Binary == "" {
		cfg.Binary = "claude"
	}
	if cfg.PermissionMode == "" {
		cfg.PermissionMode = "bypassPermissions"
	}
	return &Agent{cfg: cfg}
}

// CLIResult is the parsed JSON output from claude -p --output-format json.
type CLIResult struct {
	Type          string         `json:"type"`
	Subtype       string         `json:"subtype"`
	IsError       bool           `json:"is_error"`
	Result        string         `json:"result"`
	StopReason    string         `json:"stop_reason"`
	NumTurns      int            `json:"num_turns"`
	SessionID     string         `json:"session_id"`
	DurationMs    int            `json:"duration_ms"`
	TotalCostUSD  float64        `json:"total_cost_usd"`
	Usage         map[string]any `json:"usage"`
}

func (a *Agent) Start(ctx context.Context, cfg agent.StartConfig) (*agent.Session, error) {
	sessionID := uuid.New().String()
	updates := make(chan agent.Update, 100)
	done := make(chan agent.Result, 1)

	// Build command args
	args := []string{"-p", "--output-format", "json"}
	if cfg.ResumeID != "" {
		args = append(args, "--resume", cfg.ResumeID)
	}
	if a.cfg.PermissionMode != "" {
		args = append(args, "--permission-mode", a.cfg.PermissionMode)
	}
	if a.cfg.Model != "" {
		args = append(args, "--model", a.cfg.Model)
	}
	if len(a.cfg.AllowedTools) > 0 {
		args = append(args, "--allowed-tools", strings.Join(a.cfg.AllowedTools, ","))
	}

	cmd := exec.CommandContext(ctx, a.cfg.Binary, args...)
	cmd.Dir = cfg.WorkDir
	cmd.Stdin = strings.NewReader(cfg.Prompt)
	cmd.Env = os.Environ()

	// Start in PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	// Create PTY session for output capture and attach
	itemID := sessionID // use session ID as item ID for socket naming
	ptySess, ptyErr := agent.NewPTYSession(ptmx, agent.PTYConfig{
		LogDir:    a.cfg.LogDir,
		SocketDir: a.cfg.SocketDir,
		ItemID:    itemID,
		RingSize:  64 * 1024,
	})

	socketPath := ""
	if ptyErr != nil {
		slog.Warn("PTY session setup failed, continuing without attach support", "error", ptyErr)
	} else {
		socketPath = ptySess.SocketPath()
	}

	// Run in background goroutine
	go func() {
		defer close(updates)
		defer close(done)
		if ptySess != nil {
			defer ptySess.Close()
		}

		updates <- agent.Update{
			Kind:      agent.UpdateTurnStarted,
			Message:   "claude session started",
			Timestamp: time.Now(),
		}

		// Read all output (JSON result comes on stdout after process exits)
		// With PTY, stdout and stderr are merged into the PTY
		// We need to wait for process exit and read the final JSON from the PTY buffer
		err := cmd.Wait()

		// Read remaining PTY output for the JSON result
		var output []byte
		if ptmx != nil {
			// Read any remaining data
			remaining := make([]byte, 1024*1024) // 1MB max
			n, _ := ptmx.Read(remaining)
			if n > 0 {
				output = remaining[:n]
			}
		}

		// Try to parse the JSON result from the output
		result := agent.Result{
			SessionID: sessionID,
		}

		if err != nil {
			result.StopReason = agent.StopFailed
			result.Error = err
		} else {
			// Parse JSON from output — find the last JSON object
			parsed := parseLastJSON(output)
			if parsed != nil {
				result.SessionID = parsed.SessionID
				result.CostUSD = parsed.TotalCostUSD
				result.NumTurns = parsed.NumTurns
				result.DurationMs = parsed.DurationMs
				result.StopReason = mapStopReason(parsed)

				updates <- agent.Update{
					Kind:    agent.UpdateTurnDone,
					Message: "turn completed",
					Tokens: agent.TokenUsage{
						Total: extractTotalTokens(parsed.Usage),
					},
					Timestamp: time.Now(),
				}
			} else {
				result.StopReason = agent.StopCompleted
			}
		}

		// Check for new commits
		result.HasCommits = agent.HasNewCommits(cfg.WorkDir)

		done <- result
	}()

	return &agent.Session{
		ID:         sessionID,
		PTY:        ptmx,
		SocketPath: socketPath,
		Updates:    updates,
		Done:       done,
	}, nil
}

// parseLastJSON extracts the last JSON object from mixed output.
func parseLastJSON(data []byte) *CLIResult {
	// Claude CLI outputs JSON as the last thing on stdout
	// Try parsing the entire output first
	var result CLIResult
	if err := json.Unmarshal(data, &result); err == nil {
		return &result
	}

	// Try to find JSON in the output (scan backwards for '{')
	s := string(data)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '{' {
			var r CLIResult
			if err := json.Unmarshal([]byte(s[i:]), &r); err == nil {
				return &r
			}
		}
	}
	return nil
}

func mapStopReason(result *CLIResult) agent.StopReason {
	if result.IsError {
		return agent.StopFailed
	}
	switch result.StopReason {
	case "end_turn", "stop_sequence", "max_tokens":
		return agent.StopCompleted
	default:
		if result.Subtype == "error" {
			return agent.StopFailed
		}
		return agent.StopCompleted
	}
}

func extractTotalTokens(usage map[string]any) int {
	if usage == nil {
		return 0
	}
	if v, ok := usage["total_tokens"]; ok {
		switch t := v.(type) {
		case float64:
			return int(t)
		case int:
			return t
		}
	}
	return 0
}

// Ensure Agent implements the interface at compile time.
var _ agent.Agent = (*Agent)(nil)
