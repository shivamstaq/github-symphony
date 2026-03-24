package claude

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync/atomic"

	"github.com/shivamstaq/github-symphony/internal/adapter"
)

// Config for the Claude Code adapter.
type Config struct {
	Command string   // e.g. "tsx" or "bash" for testing
	Args    []string // e.g. ["sidecar/claude/src/index.ts"]
	Cwd     string
	Env     []string
	Model   string
}

// InitResult holds the capabilities from initialization.
type InitResult struct {
	Provider     string
	Capabilities map[string]any
}

// SessionParams for creating a new session.
type SessionParams struct {
	Cwd            string
	Title          string
	Model          string
	Tools          []any
	MCPServers     []any
	ProviderParams map[string]any
}

// PromptResult holds the result of a prompt turn.
type PromptResult struct {
	StopReason adapter.StopReason
	Summary    string
}

// Adapter manages the Claude Code sidecar subprocess.
type Adapter struct {
	sub    *adapter.SubprocessAdapter
	nextID atomic.Int64
}

// New creates and starts the Claude Code adapter subprocess.
func New(cfg Config) (*Adapter, error) {
	sub, err := adapter.NewSubprocessAdapter(adapter.SubprocessConfig{
		Command: cfg.Command,
		Args:    cfg.Args,
		Cwd:     cfg.Cwd,
		Env:     cfg.Env,
	})
	if err != nil {
		return nil, fmt.Errorf("claude adapter: %w", err)
	}

	a := &Adapter{
		sub: sub,
	}
	a.nextID.Store(1)
	return a, nil
}

func (a *Adapter) allocID() int {
	return int(a.nextID.Add(1) - 1)
}

// Initialize sends the initialize handshake.
func (a *Adapter) Initialize(ctx context.Context) (*InitResult, error) {
	resp, err := a.sub.SendRequest(ctx, adapter.Request{
		ID:     a.allocID(),
		Method: "initialize",
		Params: map[string]any{
			"protocolVersion": 1,
			"clientInfo":      map[string]any{"name": "symphony", "version": "3.0"},
			"requestedProvider": "claude_code",
			"clientCapabilities": map[string]any{
				"toolExecution":     true,
				"permissionHandling": true,
				"userInputHandling": true,
				"mcp":               true,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	result := &InitResult{
		Provider: getStr(resp.Result, "provider"),
	}
	if caps, ok := resp.Result["capabilities"].(map[string]any); ok {
		result.Capabilities = caps
	}

	slog.Info("claude adapter initialized", "provider", result.Provider)
	return result, nil
}

// NewSession creates a new session bound to a workspace.
func (a *Adapter) NewSession(ctx context.Context, params SessionParams) (string, error) {
	reqParams := map[string]any{
		"cwd":   params.Cwd,
		"title": params.Title,
	}
	if params.Model != "" {
		reqParams["model"] = params.Model
	}
	if params.ProviderParams != nil {
		reqParams["providerParams"] = params.ProviderParams
	}

	resp, err := a.sub.SendRequest(ctx, adapter.Request{
		ID:     a.allocID(),
		Method: "session/new",
		Params: reqParams,
	})
	if err != nil {
		return "", err
	}

	sessionID := getStr(resp.Result, "sessionId")
	if sessionID == "" {
		return "", fmt.Errorf("claude adapter: session/new returned empty sessionId")
	}

	slog.Info("claude session created", "session_id", sessionID)
	return sessionID, nil
}

// Prompt sends a prompt turn and waits for the result.
func (a *Adapter) Prompt(ctx context.Context, sessionID, text string) (*PromptResult, error) {
	resp, err := a.sub.SendRequest(ctx, adapter.Request{
		ID:     a.allocID(),
		Method: "session/prompt",
		Params: map[string]any{
			"sessionId":    sessionID,
			"continuation": false,
			"input": []any{
				map[string]any{
					"type": "text",
					"text": text,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return &PromptResult{
		StopReason: adapter.StopReason(getStr(resp.Result, "stopReason")),
		Summary:    getStr(resp.Result, "summary"),
	}, nil
}

// Cancel cancels an in-flight prompt turn.
func (a *Adapter) Cancel(ctx context.Context, sessionID string) error {
	_, err := a.sub.SendRequest(ctx, adapter.Request{
		ID:     a.allocID(),
		Method: "session/cancel",
		Params: map[string]any{"sessionId": sessionID},
	})
	return err
}

// CloseSession ends a session.
func (a *Adapter) CloseSession(ctx context.Context, sessionID string) error {
	_, err := a.sub.SendRequest(ctx, adapter.Request{
		ID:     a.allocID(),
		Method: "session/close",
		Params: map[string]any{"sessionId": sessionID},
	})
	return err
}

// Close terminates the adapter subprocess.
func (a *Adapter) Close() error {
	return a.sub.Close()
}

// Updates returns the channel of streaming notifications.
func (a *Adapter) Updates() <-chan *adapter.Message {
	return a.sub.Updates()
}

// CheckDependencies verifies node and tsx are available.
func CheckDependencies() error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("claude adapter: node not found on PATH: %w", err)
	}
	if _, err := exec.LookPath("tsx"); err != nil {
		return fmt.Errorf("claude adapter: tsx not found on PATH: %w", err)
	}

	// Check node version >= 22
	out, err := exec.Command("node", "--version").Output()
	if err != nil {
		return fmt.Errorf("claude adapter: cannot check node version: %w", err)
	}
	slog.Debug("node version", "version", string(out))

	return nil
}

func getStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
