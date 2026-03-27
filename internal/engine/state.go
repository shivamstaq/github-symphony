package engine

import (
	"context"
	"time"

	"github.com/shivamstaq/github-symphony/internal/agent"
	"github.com/shivamstaq/github-symphony/internal/domain"
)

// RunningEntry tracks one active worker.
type RunningEntry struct {
	WorkItem        domain.WorkItem
	Session         *agent.Session
	CancelFunc      context.CancelFunc
	Phase           domain.ItemState
	Paused          bool
	RetryAttempt    int
	StartedAt       time.Time
	LastActivityAt  time.Time
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	CostUSD         float64
	TurnsCompleted  int
	WorkspacePath   string
	BranchName      string
}

// RetryEntry is a scheduled retry.
type RetryEntry struct {
	WorkItemID      string
	IssueIdentifier string
	Attempt         int
	DueAt           time.Time
	Error           string
	WorkItem        *domain.WorkItem // stored for re-dispatch without re-fetch
}

// AgentTotals accumulates lifetime agent metrics.
type AgentTotals struct {
	InputTokens      int64
	OutputTokens     int64
	TotalTokens      int64
	SecondsRunning   float64
	Writebacks       int64
	SessionsStarted  int64
	CostUSD          float64
}

// State is the engine's authoritative runtime state.
// Only the engine's event loop goroutine reads/writes this — no mutexes needed.
type State struct {
	// Item FSM states (work item ID -> current state)
	ItemStates map[string]domain.ItemState

	// Running workers
	Running map[string]*RunningEntry

	// Retry queue
	RetryQueue map[string]*RetryEntry

	// Handed-off items (prevents re-dispatch)
	HandedOff map[string]bool

	// Aggregate metrics
	Totals AgentTotals

	// Poll tracking
	LastPollAt     *time.Time
	PendingRefresh bool

	// Counters
	DispatchTotal int64
	ErrorTotal    int64
	HandoffTotal  int64
}

// NewState creates an initialized empty state.
func NewState() *State {
	return &State{
		ItemStates: make(map[string]domain.ItemState),
		Running:    make(map[string]*RunningEntry),
		RetryQueue: make(map[string]*RetryEntry),
		HandedOff:  make(map[string]bool),
	}
}

// ItemState returns the current FSM state for a work item, defaulting to StateOpen.
func (s *State) ItemState(itemID string) domain.ItemState {
	if st, ok := s.ItemStates[itemID]; ok {
		return st
	}
	return domain.StateOpen
}

// SetItemState updates the FSM state for a work item.
func (s *State) SetItemState(itemID string, state domain.ItemState) {
	if state == domain.StateOpen {
		delete(s.ItemStates, itemID)
	} else {
		s.ItemStates[itemID] = state
	}
}

// IsClaimedOrRunning returns true if the item is in any active state.
func (s *State) IsClaimedOrRunning(itemID string) bool {
	st := s.ItemState(itemID)
	return st == domain.StateQueued || st == domain.StatePreparing ||
		st == domain.StateRunning || st == domain.StatePaused ||
		st == domain.StateCompleted
}

// RunningCount returns the number of currently running workers.
func (s *State) RunningCount() int {
	return len(s.Running)
}
