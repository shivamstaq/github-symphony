package adapter

import "context"

// AdapterClient is the unified interface for all agent adapters.
type AdapterClient interface {
	Initialize(ctx context.Context) (*InitResult, error)
	NewSession(ctx context.Context, params SessionParams) (string, error)
	Prompt(ctx context.Context, sessionID string, text string) (*PromptResult, error)
	Cancel(ctx context.Context, sessionID string) error
	CloseSession(ctx context.Context, sessionID string) error
	Close() error
	Updates() <-chan *Message
	PID() int
}

// InitResult holds the capabilities from initialization.
type InitResult struct {
	Provider     string
	Capabilities map[string]any
}

// SessionParams for creating a new session.
type SessionParams struct {
	Cwd              string
	Title            string
	Model            string
	Tools            []any
	MCPServers       []any
	ProviderParams   map[string]any
	ResumeSessionID  string // Resume a previous Claude session (for continuation turns)
}

// PromptResult holds the result of a prompt turn.
type PromptResult struct {
	StopReason  StopReason
	Summary     string
	SessionID   string  // Claude session ID for resumption
	CostUSD     float64 // Cost of this turn
	NumTurns    int     // Internal turns Claude took
	DurationMs  int     // Wall-clock duration
}

// AdapterConfig holds configuration for creating an adapter.
type AdapterConfig struct {
	Kind           string // "claude_code", "opencode", "codex"
	Command        string
	Args           []string
	Cwd            string
	Env            []string
	Model          string
	AllowedTools   []string // for Claude CLI: restrict tools
	PermissionMode string   // for Claude CLI: permission handling mode
}
