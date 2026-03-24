package codex

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync/atomic"

	"github.com/shivamstaq/github-symphony/internal/adapter"
)

type Config struct {
	Command string
	Args    []string
	Cwd     string
	Env     []string
	Model   string
}

type InitResult struct {
	Provider     string
	Capabilities map[string]any
}

type SessionParams struct {
	Cwd            string
	Title          string
	Model          string
	ProviderParams map[string]any
}

type PromptResult struct {
	StopReason adapter.StopReason
	Summary    string
}

type Adapter struct {
	sub    *adapter.SubprocessAdapter
	nextID atomic.Int64
}

func New(cfg Config) (*Adapter, error) {
	sub, err := adapter.NewSubprocessAdapter(adapter.SubprocessConfig{
		Command: cfg.Command,
		Args:    cfg.Args,
		Cwd:     cfg.Cwd,
		Env:     cfg.Env,
	})
	if err != nil {
		return nil, fmt.Errorf("codex adapter: %w", err)
	}
	a := &Adapter{sub: sub}
	a.nextID.Store(1)
	return a, nil
}

func (a *Adapter) allocID() int { return int(a.nextID.Add(1) - 1) }

func (a *Adapter) Initialize(ctx context.Context) (*InitResult, error) {
	resp, err := a.sub.SendRequest(ctx, adapter.Request{
		ID:     a.allocID(),
		Method: "initialize",
		Params: map[string]any{
			"protocolVersion":    1,
			"clientInfo":         map[string]any{"name": "symphony", "version": "3.0"},
			"requestedProvider":  "codex",
			"clientCapabilities": map[string]any{"toolExecution": true, "permissionHandling": true},
		},
	})
	if err != nil {
		return nil, err
	}
	result := &InitResult{Provider: getStr(resp.Result, "provider")}
	if caps, ok := resp.Result["capabilities"].(map[string]any); ok {
		result.Capabilities = caps
	}
	slog.Info("codex adapter initialized", "provider", result.Provider)
	return result, nil
}

func (a *Adapter) NewSession(ctx context.Context, params SessionParams) (string, error) {
	p := map[string]any{"cwd": params.Cwd, "title": params.Title, "provider": "codex"}
	if params.Model != "" {
		p["model"] = params.Model
	}
	if params.ProviderParams != nil {
		p["providerParams"] = map[string]any{"codex": params.ProviderParams}
	}
	resp, err := a.sub.SendRequest(ctx, adapter.Request{ID: a.allocID(), Method: "session/new", Params: p})
	if err != nil {
		return "", err
	}
	return getStr(resp.Result, "sessionId"), nil
}

func (a *Adapter) Prompt(ctx context.Context, sessionID, text string) (*PromptResult, error) {
	resp, err := a.sub.SendRequest(ctx, adapter.Request{
		ID: a.allocID(), Method: "session/prompt",
		Params: map[string]any{
			"sessionId": sessionID, "continuation": false,
			"input": []any{map[string]any{"type": "text", "text": text}},
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

func (a *Adapter) Close() error { return a.sub.Close() }

func CheckDependencies() error {
	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("codex adapter: codex not found on PATH: %w", err)
	}
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
