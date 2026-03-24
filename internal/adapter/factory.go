package adapter

import (
	"context"
	"fmt"
	"os"
)

// NewAdapter creates an adapter client for the given agent kind.
func NewAdapter(cfg AdapterConfig) (AdapterClient, error) {
	// Validate CWD exists if specified
	if cfg.Cwd != "" {
		if info, err := os.Stat(cfg.Cwd); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("invalid_workspace_cwd: %q is not a valid directory", cfg.Cwd)
		}
	}

	switch cfg.Kind {
	case "claude_code":
		return newGenericAdapter(cfg, "claude_code")
	case "opencode":
		return newGenericAdapter(cfg, "opencode")
	case "codex":
		return newGenericAdapter(cfg, "codex")
	default:
		return nil, fmt.Errorf("adapter_not_found: unsupported agent kind %q", cfg.Kind)
	}
}

// genericAdapter wraps SubprocessAdapter with the unified AdapterClient interface.
type genericAdapter struct {
	sub      *SubprocessAdapter
	provider string
	idSeq    int
}

func newGenericAdapter(cfg AdapterConfig, provider string) (*genericAdapter, error) {
	sub, err := NewSubprocessAdapter(SubprocessConfig{
		Command: cfg.Command,
		Args:    cfg.Args,
		Cwd:     cfg.Cwd,
		Env:     cfg.Env,
	})
	if err != nil {
		return nil, fmt.Errorf("adapter %s: %w", provider, err)
	}
	return &genericAdapter{sub: sub, provider: provider}, nil
}

func (a *genericAdapter) allocID() int {
	a.idSeq++
	return a.idSeq
}

func (a *genericAdapter) Initialize(ctx context.Context) (*InitResult, error) {
	resp, err := a.sub.SendRequest(ctx, Request{
		ID:     a.allocID(),
		Method: "initialize",
		Params: map[string]any{
			"protocolVersion":   1,
			"clientInfo":        map[string]any{"name": "symphony", "version": "3.0"},
			"requestedProvider": a.provider,
			"clientCapabilities": map[string]any{
				"toolExecution":      true,
				"permissionHandling": true,
				"userInputHandling":  true,
				"mcp":                true,
				"images":             false,
				"audio":              false,
			},
			"_meta": map[string]any{
				"traceId": fmt.Sprintf("tr_%s_%d", a.provider, a.idSeq),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	result := &InitResult{Provider: getStrFromMap(resp.Result, "provider")}
	if caps, ok := resp.Result["capabilities"].(map[string]any); ok {
		result.Capabilities = caps
	}
	return result, nil
}

func (a *genericAdapter) NewSession(ctx context.Context, params SessionParams) (string, error) {
	p := map[string]any{
		"cwd":      params.Cwd,
		"title":    params.Title,
		"provider": a.provider,
	}
	if params.Model != "" {
		p["model"] = params.Model
	}
	if params.ProviderParams != nil {
		p["providerParams"] = map[string]any{a.provider: params.ProviderParams}
	}
	if params.Tools != nil {
		p["tools"] = params.Tools
	}
	if params.MCPServers != nil {
		p["mcpServers"] = params.MCPServers
	}

	resp, err := a.sub.SendRequest(ctx, Request{ID: a.allocID(), Method: "session/new", Params: p})
	if err != nil {
		return "", err
	}
	sessionID := getStrFromMap(resp.Result, "sessionId")
	if sessionID == "" {
		return "", fmt.Errorf("adapter %s: session/new returned empty sessionId", a.provider)
	}
	return sessionID, nil
}

func (a *genericAdapter) Prompt(ctx context.Context, sessionID string, text string) (*PromptResult, error) {
	resp, err := a.sub.SendRequest(ctx, Request{
		ID:     a.allocID(),
		Method: "session/prompt",
		Params: map[string]any{
			"sessionId":    sessionID,
			"continuation": false,
			"input":        []any{map[string]any{"type": "text", "text": text}},
		},
	})
	if err != nil {
		return nil, err
	}
	return &PromptResult{
		StopReason: StopReason(getStrFromMap(resp.Result, "stopReason")),
		Summary:    getStrFromMap(resp.Result, "summary"),
	}, nil
}

func (a *genericAdapter) Cancel(ctx context.Context, sessionID string) error {
	_, err := a.sub.SendRequest(ctx, Request{
		ID:     a.allocID(),
		Method: "session/cancel",
		Params: map[string]any{"sessionId": sessionID},
	})
	return err
}

func (a *genericAdapter) CloseSession(ctx context.Context, sessionID string) error {
	_, err := a.sub.SendRequest(ctx, Request{
		ID:     a.allocID(),
		Method: "session/close",
		Params: map[string]any{"sessionId": sessionID},
	})
	return err
}

func (a *genericAdapter) Close() error {
	return a.sub.Close()
}

func (a *genericAdapter) Updates() <-chan *Message {
	return a.sub.Updates()
}

func (a *genericAdapter) PID() int {
	return a.sub.PID()
}

func getStrFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
