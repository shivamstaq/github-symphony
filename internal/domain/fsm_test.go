package domain

import (
	"errors"
	"testing"
)

// guardAlways satisfies any guard.
func guardAlways(_ TransitionGuard) bool { return true }

// guardNoneOnly only satisfies no-guard transitions.
func guardNoneOnly(g TransitionGuard) bool { return g == GuardNone }

// guardSpecific returns a function that satisfies exactly the given guard.
func guardSpecific(want TransitionGuard) func(TransitionGuard) bool {
	return func(g TransitionGuard) bool { return g == want }
}

func TestTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  ItemState
		event Event
		guard func(TransitionGuard) bool
		to    ItemState
	}{
		// Dispatch lifecycle
		{"open->queued via claim", StateOpen, EventClaim, guardAlways, StateQueued},
		{"queued->preparing via dispatch", StateQueued, EventDispatch, guardAlways, StatePreparing},
		{"preparing->running via workspace_ready", StatePreparing, EventWorkspaceReady, nil, StateRunning},
		{"preparing->failed via error", StatePreparing, EventError, nil, StateFailed},

		// Agent execution
		{"running->running via turn_completed", StateRunning, EventTurnCompleted, nil, StateRunning},
		{"running->completed via agent_exited_with_commits", StateRunning, EventAgentExitedCommits, nil, StateCompleted},
		{"running->needs_human via agent_exited_no_commits", StateRunning, EventAgentExitedEmpty, nil, StateNeedsHuman},
		{"running->paused via pause_requested", StateRunning, EventPauseRequested, nil, StatePaused},
		{"running->needs_human via stall_detected", StateRunning, EventStallDetected, nil, StateNeedsHuman},
		{"running->needs_human via budget_exceeded", StateRunning, EventBudgetExceeded, nil, StateNeedsHuman},
		{"running->queued via error (has retries)", StateRunning, EventError, guardSpecific(GuardHasRetriesLeft), StateQueued},
		{"running->failed via error (max retries)", StateRunning, EventError, guardSpecific(GuardMaxRetriesExhausted), StateFailed},
		{"running->open via cancelled", StateRunning, EventCancelled, nil, StateOpen},

		// Pause/resume
		{"paused->running via resume", StatePaused, EventResume, nil, StateRunning},
		{"paused->open via cancelled", StatePaused, EventCancelled, nil, StateOpen},

		// Write-back
		{"completed->handed_off via pr_created", StateCompleted, EventPRCreated, nil, StateHandedOff},
		{"completed->needs_human via error", StateCompleted, EventError, nil, StateNeedsHuman},

		// Post-handoff
		{"handed_off->open via pr_merged", StateHandedOff, EventPRMerged, nil, StateOpen},
		{"handed_off->open via pr_closed", StateHandedOff, EventPRClosed, nil, StateOpen},

		// Human intervention
		{"needs_human->queued via retry_manual", StateNeedsHuman, EventRetryManual, nil, StateQueued},
		{"needs_human->open via cancelled", StateNeedsHuman, EventCancelled, nil, StateOpen},

		// Failed recovery
		{"failed->queued via retry_manual", StateFailed, EventRetryManual, nil, StateQueued},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transition(tt.from, tt.event, tt.guard)
			if err != nil {
				t.Fatalf("expected valid transition, got error: %v", err)
			}
			if result.To != tt.to {
				t.Errorf("expected To=%q, got %q", tt.to, result.To)
			}
			if result.From != tt.from {
				t.Errorf("expected From=%q, got %q", tt.from, result.From)
			}
			if result.Event != tt.event {
				t.Errorf("expected Event=%q, got %q", tt.event, result.Event)
			}
		})
	}
}

func TestTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  ItemState
		event Event
	}{
		// Can't claim something already queued
		{"queued+claim", StateQueued, EventClaim},
		// Can't dispatch from open
		{"open+dispatch", StateOpen, EventDispatch},
		// Can't complete from open
		{"open+agent_exited_commits", StateOpen, EventAgentExitedCommits},
		// Can't pause from queued
		{"queued+pause", StateQueued, EventPauseRequested},
		// Can't resume from running
		{"running+resume", StateRunning, EventResume},
		// Can't create PR from running
		{"running+pr_created", StateRunning, EventPRCreated},
		// Can't claim from handed_off
		{"handed_off+claim", StateHandedOff, EventClaim},
		// Can't dispatch from failed
		{"failed+dispatch", StateFailed, EventDispatch},
		// Can't turn_completed from paused
		{"paused+turn_completed", StatePaused, EventTurnCompleted},
		// Can't merge PR from completed
		{"completed+pr_merged", StateCompleted, EventPRMerged},
		// Can't stall from open
		{"open+stall", StateOpen, EventStallDetected},
		// Can't exceed budget from queued
		{"queued+budget", StateQueued, EventBudgetExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transition(tt.from, tt.event, nil)
			if err == nil {
				t.Fatal("expected error for invalid transition, got nil")
			}
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("expected ErrInvalidTransition, got: %v", err)
			}
		})
	}
}

func TestTransition_GuardedErrorFromRunning(t *testing.T) {
	// Running + error with has_retries_left -> queued
	result, err := Transition(StateRunning, EventError, guardSpecific(GuardHasRetriesLeft))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.To != StateQueued {
		t.Errorf("expected queued, got %q", result.To)
	}

	// Running + error with max_retries_exhausted -> failed
	result, err = Transition(StateRunning, EventError, guardSpecific(GuardMaxRetriesExhausted))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.To != StateFailed {
		t.Errorf("expected failed, got %q", result.To)
	}

	// Running + error with no guard satisfied -> invalid
	_, err = Transition(StateRunning, EventError, guardNoneOnly)
	if err == nil {
		t.Fatal("expected error when no guard is satisfied for running+error")
	}
}

func TestValidTransitions(t *testing.T) {
	// Open should have exactly 1 valid transition: claim
	trans := ValidTransitions(StateOpen)
	if len(trans) != 1 {
		t.Errorf("expected 1 transition from open, got %d", len(trans))
	}

	// Running should have many valid transitions
	trans = ValidTransitions(StateRunning)
	if len(trans) < 8 {
		t.Errorf("expected >=8 transitions from running, got %d", len(trans))
	}

	// Every state should have at least one exit transition
	for _, state := range AllStates {
		if state == StateOpen {
			// open can only be claimed
			continue
		}
		trans := ValidTransitions(state)
		if len(trans) == 0 {
			t.Errorf("state %q has no exit transitions (dead state)", state)
		}
	}
}

func TestInvariant_NoItemInTwoTerminalStates(t *testing.T) {
	// This is a structural invariant: the FSM is deterministic.
	// For any (from, event, guard) triple, there is at most one target state.
	type key struct {
		from  ItemState
		event Event
		guard TransitionGuard
	}
	seen := make(map[key]ItemState)
	for _, tr := range transitionTable {
		k := key{tr.From, tr.Event, tr.Guard}
		if existing, ok := seen[k]; ok {
			t.Errorf("duplicate transition: (%q, %q, %q) -> %q AND %q",
				tr.From, tr.Event, tr.Guard, existing, tr.To)
		}
		seen[k] = tr.To
	}
}

func TestInvariant_AllStatesReachable(t *testing.T) {
	reachable := make(map[ItemState]bool)
	reachable[StateOpen] = true // initial state

	changed := true
	for changed {
		changed = false
		for _, tr := range transitionTable {
			if reachable[tr.From] && !reachable[tr.To] {
				reachable[tr.To] = true
				changed = true
			}
		}
	}

	for _, state := range AllStates {
		if !reachable[state] {
			t.Errorf("state %q is unreachable from open", state)
		}
	}
}

func TestInvariant_NeedsHumanOnlyFromNoProgress(t *testing.T) {
	// needs_human should only be reachable via:
	// - agent_exited_no_commits (from running)
	// - stall_detected (from running)
	// - budget_exceeded (from running)
	// - error (from completed, i.e. write-back failed)
	allowedEvents := map[Event]bool{
		EventAgentExitedEmpty: true,
		EventStallDetected:   true,
		EventBudgetExceeded:  true,
		EventError:           true,
	}

	for _, tr := range transitionTable {
		if tr.To == StateNeedsHuman {
			if !allowedEvents[tr.Event] {
				t.Errorf("needs_human reached via unexpected event %q from %q", tr.Event, tr.From)
			}
		}
	}
}
