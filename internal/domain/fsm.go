package domain

import (
	"fmt"
	"time"
)

// ItemState represents the FSM state of a work item in the orchestrator.
type ItemState string

const (
	StateOpen       ItemState = "open"        // exists in tracker, not yet picked up
	StateQueued     ItemState = "queued"       // claimed, awaiting dispatch slot
	StatePreparing  ItemState = "preparing"    // workspace being created
	StateRunning    ItemState = "running"      // agent actively working
	StatePaused     ItemState = "paused"       // between-turn pause
	StateCompleted  ItemState = "completed"    // agent finished with commits
	StateHandedOff  ItemState = "handed_off"   // PR created, status moved
	StateNeedsHuman ItemState = "needs_human"  // requires human intervention
	StateFailed     ItemState = "failed"       // unrecoverable
)

// AllStates is the complete set of valid states.
var AllStates = []ItemState{
	StateOpen, StateQueued, StatePreparing, StateRunning,
	StatePaused, StateCompleted, StateHandedOff,
	StateNeedsHuman, StateFailed,
}

// IsTerminal returns true if the state means the item has left the orchestrator's active management.
func (s ItemState) IsTerminal() bool {
	return s == StateHandedOff || s == StateFailed
}

// IsActive returns true if the item is being actively worked on.
func (s ItemState) IsActive() bool {
	return s == StateRunning || s == StatePaused || s == StatePreparing
}

// Event represents a trigger that causes a state transition.
type Event string

const (
	EventClaim               Event = "claim"
	EventDispatch            Event = "dispatch"
	EventWorkspaceReady      Event = "workspace_ready"
	EventTurnCompleted       Event = "turn_completed"
	EventAgentExitedCommits  Event = "agent_exited_with_commits"
	EventAgentExitedEmpty    Event = "agent_exited_no_commits"
	EventPauseRequested      Event = "pause_requested"
	EventResume              Event = "resume"
	EventStallDetected       Event = "stall_detected"
	EventBudgetExceeded      Event = "budget_exceeded"
	EventError               Event = "error"
	EventCancelled           Event = "cancelled"
	EventPRCreated           Event = "pr_created"
	EventPRMerged            Event = "pr_merged"
	EventPRClosed            Event = "pr_closed"
	EventRetryManual         Event = "retry_manual"
)

// AllEvents is the complete set of valid events.
var AllEvents = []Event{
	EventClaim, EventDispatch, EventWorkspaceReady, EventTurnCompleted,
	EventAgentExitedCommits, EventAgentExitedEmpty, EventPauseRequested,
	EventResume, EventStallDetected, EventBudgetExceeded, EventError,
	EventCancelled, EventPRCreated, EventPRMerged, EventPRClosed,
	EventRetryManual,
}

// TransitionGuard is a named condition that must be true for a transition to fire.
type TransitionGuard string

const (
	GuardNone             TransitionGuard = ""
	GuardSlotAvailable    TransitionGuard = "slot_available"
	GuardConcurrencyOK    TransitionGuard = "concurrency_ok"
	GuardHasRetriesLeft   TransitionGuard = "has_retries_left"
	GuardMaxRetriesExhausted TransitionGuard = "max_retries_exhausted"
)

// transition defines a single valid state transition.
type transition struct {
	From  ItemState
	Event Event
	To    ItemState
	Guard TransitionGuard
}

// transitionTable is the declarative, exhaustive list of valid transitions.
// Any (From, Event) pair not in this table is an invalid transition.
var transitionTable = []transition{
	// Dispatch lifecycle
	{StateOpen, EventClaim, StateQueued, GuardSlotAvailable},
	{StateQueued, EventDispatch, StatePreparing, GuardConcurrencyOK},
	{StatePreparing, EventWorkspaceReady, StateRunning, GuardNone},
	{StatePreparing, EventError, StateFailed, GuardNone},

	// Agent execution
	{StateRunning, EventTurnCompleted, StateRunning, GuardNone},
	{StateRunning, EventAgentExitedCommits, StateCompleted, GuardNone},
	{StateRunning, EventAgentExitedEmpty, StateNeedsHuman, GuardNone},
	{StateRunning, EventPauseRequested, StatePaused, GuardNone},
	{StateRunning, EventStallDetected, StateNeedsHuman, GuardNone},
	{StateRunning, EventBudgetExceeded, StateNeedsHuman, GuardNone},
	{StateRunning, EventError, StateQueued, GuardHasRetriesLeft},
	{StateRunning, EventError, StateFailed, GuardMaxRetriesExhausted},
	{StateRunning, EventCancelled, StateOpen, GuardNone},

	// Pause/resume
	{StatePaused, EventResume, StateRunning, GuardNone},
	{StatePaused, EventCancelled, StateOpen, GuardNone},

	// Write-back
	{StateCompleted, EventPRCreated, StateHandedOff, GuardNone},
	{StateCompleted, EventError, StateNeedsHuman, GuardNone},

	// Post-handoff
	{StateHandedOff, EventPRMerged, StateOpen, GuardNone},
	{StateHandedOff, EventPRClosed, StateOpen, GuardNone},

	// Human intervention
	{StateNeedsHuman, EventRetryManual, StateQueued, GuardNone},
	{StateNeedsHuman, EventCancelled, StateOpen, GuardNone},

	// Failed recovery
	{StateFailed, EventRetryManual, StateQueued, GuardNone},
}

// ErrInvalidTransition is returned when a transition is not in the table.
var ErrInvalidTransition = fmt.Errorf("invalid state transition")

// TransitionResult contains the outcome of a transition attempt.
type TransitionResult struct {
	From  ItemState
	To    ItemState
	Event Event
	Guard TransitionGuard
}

// Transition attempts to move from the given state via the given event.
// It returns the target state and guard (if any), or ErrInvalidTransition.
//
// When multiple transitions match (same From+Event but different guards),
// the caller must supply the correct guard via guardSatisfied.
// If guardSatisfied is nil, only guardless transitions match.
func Transition(current ItemState, event Event, guardSatisfied func(TransitionGuard) bool) (TransitionResult, error) {
	if guardSatisfied == nil {
		guardSatisfied = func(g TransitionGuard) bool { return g == GuardNone }
	}

	for _, t := range transitionTable {
		if t.From == current && t.Event == event && guardSatisfied(t.Guard) {
			return TransitionResult{
				From:  t.From,
				To:    t.To,
				Event: event,
				Guard: t.Guard,
			}, nil
		}
	}
	return TransitionResult{}, fmt.Errorf("%w: cannot transition from %q via %q", ErrInvalidTransition, current, event)
}

// ValidTransitions returns all transitions valid from the given state.
func ValidTransitions(current ItemState) []transition {
	var result []transition
	for _, t := range transitionTable {
		if t.From == current {
			result = append(result, t)
		}
	}
	return result
}

// FSMEvent is an immutable record of a state transition, appended to the event log.
type FSMEvent struct {
	Timestamp  time.Time       `json:"timestamp"`
	ItemID     string          `json:"item_id"`
	From       ItemState       `json:"from"`
	To         ItemState       `json:"to"`
	Event      Event           `json:"event"`
	Guard      TransitionGuard `json:"guard,omitempty"`
	Metadata   map[string]any  `json:"metadata,omitempty"`
}
