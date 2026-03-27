// Package property contains property-based tests for the FSM.
// These tests generate random event sequences and verify that invariants
// always hold, regardless of the sequence.
package property

import (
	"math/rand"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/domain"
)

// guardForEvent returns a guard function appropriate for the given event
// when transitioning from the given state.
func guardForEvent(state domain.ItemState, event domain.Event) func(domain.TransitionGuard) bool {
	// For error events from running, randomly pick one of the two guards
	if state == domain.StateRunning && event == domain.EventError {
		if rand.Intn(2) == 0 {
			return func(g domain.TransitionGuard) bool { return g == domain.GuardHasRetriesLeft }
		}
		return func(g domain.TransitionGuard) bool { return g == domain.GuardMaxRetriesExhausted }
	}
	// For claim/dispatch, satisfy their guards
	if event == domain.EventClaim {
		return func(g domain.TransitionGuard) bool {
			return g == domain.GuardSlotAvailable || g == domain.GuardNone
		}
	}
	if event == domain.EventDispatch {
		return func(g domain.TransitionGuard) bool {
			return g == domain.GuardConcurrencyOK || g == domain.GuardNone
		}
	}
	return nil
}

// TestProperty_RandomSequencesNeverPanic verifies that applying random
// event sequences to the FSM never panics and always returns either
// a valid transition or ErrInvalidTransition.
func TestProperty_RandomSequencesNeverPanic(t *testing.T) {
	const numSequences = 10000
	const maxSeqLen = 50

	for i := 0; i < numSequences; i++ {
		state := domain.StateOpen
		seqLen := rand.Intn(maxSeqLen) + 1

		for j := 0; j < seqLen; j++ {
			event := domain.AllEvents[rand.Intn(len(domain.AllEvents))]
			guard := guardForEvent(state, event)
			result, err := domain.Transition(state, event, guard)
			if err == nil {
				state = result.To
			}
			// No panic = pass
		}
	}
}

// TestProperty_SingleStateInvariant verifies that after any valid transition,
// the item is in exactly one state (the target state).
func TestProperty_SingleStateInvariant(t *testing.T) {
	const numSequences = 5000
	const maxSeqLen = 30

	for i := 0; i < numSequences; i++ {
		state := domain.StateOpen

		for j := 0; j < maxSeqLen; j++ {
			event := domain.AllEvents[rand.Intn(len(domain.AllEvents))]
			guard := guardForEvent(state, event)
			result, err := domain.Transition(state, event, guard)
			if err == nil {
				state = result.To
			}

			// Verify state is one of AllStates
			found := false
			for _, s := range domain.AllStates {
				if state == s {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("state %q is not in AllStates after sequence of length %d", state, j+1)
			}
		}
	}
}

// TestProperty_NeedsHumanOnlyFromExpectedEvents verifies that needs_human
// is only reachable via the expected events (no_commits, stall, budget, writeback error).
func TestProperty_NeedsHumanOnlyFromExpectedEvents(t *testing.T) {
	allowedToNeedsHuman := map[domain.Event]bool{
		domain.EventAgentExitedEmpty: true,
		domain.EventStallDetected:   true,
		domain.EventBudgetExceeded:  true,
		domain.EventError:           true, // from completed (writeback error)
	}

	const numSequences = 10000
	const maxSeqLen = 50

	for i := 0; i < numSequences; i++ {
		state := domain.StateOpen

		for j := 0; j < maxSeqLen; j++ {
			event := domain.AllEvents[rand.Intn(len(domain.AllEvents))]
			guard := guardForEvent(state, event)
			result, err := domain.Transition(state, event, guard)
			if err == nil {
				if result.To == domain.StateNeedsHuman && !allowedToNeedsHuman[event] {
					t.Fatalf("reached needs_human via unexpected event %q from %q",
						event, result.From)
				}
				state = result.To
			}
		}
	}
}

// TestProperty_HandedOffRequiresPRCreated verifies that handed_off is only
// reachable via the pr_created event.
func TestProperty_HandedOffRequiresPRCreated(t *testing.T) {
	const numSequences = 10000
	const maxSeqLen = 50

	for i := 0; i < numSequences; i++ {
		state := domain.StateOpen

		for j := 0; j < maxSeqLen; j++ {
			event := domain.AllEvents[rand.Intn(len(domain.AllEvents))]
			guard := guardForEvent(state, event)
			result, err := domain.Transition(state, event, guard)
			if err == nil {
				if result.To == domain.StateHandedOff && event != domain.EventPRCreated {
					t.Fatalf("reached handed_off via %q from %q — only pr_created should lead here",
						event, result.From)
				}
				state = result.To
			}
		}
	}
}

// TestProperty_NoTerminalStateEscape verifies that failed and handed_off
// states can only be exited via specific events (retry_manual, pr_merged, pr_closed).
func TestProperty_NoTerminalStateEscape(t *testing.T) {
	terminalExits := map[domain.ItemState]map[domain.Event]bool{
		domain.StateHandedOff: {
			domain.EventPRMerged: true,
			domain.EventPRClosed: true,
		},
		domain.StateFailed: {
			domain.EventRetryManual: true,
		},
	}

	const numSequences = 10000
	const maxSeqLen = 50

	for i := 0; i < numSequences; i++ {
		state := domain.StateOpen

		for j := 0; j < maxSeqLen; j++ {
			event := domain.AllEvents[rand.Intn(len(domain.AllEvents))]
			guard := guardForEvent(state, event)
			prevState := state
			result, err := domain.Transition(state, event, guard)
			if err == nil {
				if exits, isTerminal := terminalExits[prevState]; isTerminal {
					if !exits[event] {
						t.Fatalf("escaped terminal state %q via %q — only %v should work",
							prevState, event, exits)
					}
				}
				state = result.To
			}
		}
	}
}

// TestProperty_EventLogIsValidSequence verifies that recording transitions
// and replaying them produces a consistent state.
func TestProperty_EventLogIsValidSequence(t *testing.T) {
	const numSequences = 5000
	const maxSeqLen = 30

	for i := 0; i < numSequences; i++ {
		state := domain.StateOpen
		var log []domain.TransitionResult

		for j := 0; j < maxSeqLen; j++ {
			event := domain.AllEvents[rand.Intn(len(domain.AllEvents))]
			guard := guardForEvent(state, event)
			result, err := domain.Transition(state, event, guard)
			if err == nil {
				log = append(log, result)
				state = result.To
			}
		}

		// Replay the log using the recorded guards and verify same final state
		replayState := domain.StateOpen
		for k, entry := range log {
			// Use the exact guard that was recorded to ensure deterministic replay
			recordedGuard := entry.Guard
			result, err := domain.Transition(replayState, entry.Event, func(g domain.TransitionGuard) bool {
				return g == recordedGuard
			})
			if err != nil {
				t.Fatalf("replay failed at step %d: %v (from=%q event=%q guard=%q)", k, err, replayState, entry.Event, recordedGuard)
			}
			if result.To != entry.To {
				t.Fatalf("replay diverged at step %d: got %q, want %q", k, result.To, entry.To)
			}
			replayState = result.To
		}

		if replayState != state {
			t.Fatalf("replay final state %q != original %q", replayState, state)
		}
	}
}
