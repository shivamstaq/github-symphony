package agent

import (
	"context"
	"os"
	"time"
)

// Agent runs coding tasks in a workspace.
// Implementations: agent/claude, agent/opencode, agent/codex, agent/mock
type Agent interface {
	// Start launches the agent process in the given workspace.
	// Returns a Session handle for interaction.
	Start(ctx context.Context, cfg StartConfig) (*Session, error)
}

// StartConfig contains everything needed to start an agent session.
type StartConfig struct {
	WorkDir  string // workspace directory (CWD for agent)
	Prompt   string // rendered prompt template
	Title    string // issue title (for context)
	ResumeID string // previous session ID for continuation
	MaxTurns int    // maximum turns for this session
}

// Session represents a running agent process.
type Session struct {
	ID         string        // unique session identifier
	PTY        *os.File      // PTY master fd (nil for non-PTY agents)
	SocketPath string        // Unix socket path for attach
	Updates    <-chan Update  // real-time progress updates
	Done       <-chan Result  // final result when agent exits
}

// Update is a real-time progress notification from a running agent.
type Update struct {
	Kind      UpdateKind
	Message   string
	Tokens    TokenUsage
	Timestamp time.Time
}

// UpdateKind classifies an agent update.
type UpdateKind string

const (
	UpdateTurnStarted UpdateKind = "turn_started"
	UpdateTurnDone    UpdateKind = "turn_done"
	UpdateTokens      UpdateKind = "tokens"
	UpdateProgress    UpdateKind = "progress"
	UpdateError       UpdateKind = "error"
)

// Result is the final outcome of an agent session.
type Result struct {
	StopReason StopReason
	SessionID  string  // native session ID for resumption
	CostUSD    float64 // total cost of this session
	NumTurns   int
	DurationMs int
	HasCommits bool // true if new commits exist on branch
	Error      error
}

// StopReason explains why the agent session ended.
type StopReason string

const (
	StopCompleted      StopReason = "completed"
	StopFailed         StopReason = "failed"
	StopCancelled      StopReason = "cancelled"
	StopTimedOut       StopReason = "timed_out"
	StopBudgetExceeded StopReason = "budget_exceeded"
)

// TokenUsage tracks token consumption.
type TokenUsage struct {
	Input  int
	Output int
	Total  int
}
