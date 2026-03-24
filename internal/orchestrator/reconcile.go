package orchestrator

import (
	"strings"
	"time"
)

// ReconcileAction indicates what should happen to a running work item after refresh.
type ReconcileAction int

const (
	ActionKeep      ReconcileAction = iota // still active, update snapshot
	ActionTerminate                        // terminal — stop and cleanup workspace
	ActionStop                             // non-active, non-terminal — stop without cleanup
)

// DetectStalled returns work item IDs that have exceeded the stall timeout.
// If stallTimeoutMs <= 0, stall detection is disabled.
func DetectStalled(state *State, stallTimeoutMs int) []string {
	if stallTimeoutMs <= 0 {
		return nil
	}

	threshold := time.Duration(stallTimeoutMs) * time.Millisecond
	now := time.Now()
	var stalled []string

	for id, entry := range state.Running {
		var lastActivity time.Time
		if entry.LastAgentTimestamp != nil {
			lastActivity = *entry.LastAgentTimestamp
		} else {
			lastActivity = entry.StartedAt
		}

		if now.Sub(lastActivity) > threshold {
			stalled = append(stalled, id)
		}
	}

	return stalled
}

// ClassifyRefreshed determines what action to take on a running work item
// based on its refreshed GitHub state.
func ClassifyRefreshed(item WorkItem, activeValues, terminalValues []string) ReconcileAction {
	statusLower := strings.ToLower(item.ProjectStatus)
	stateLower := strings.ToLower(item.State)

	// Check terminal conditions
	isTerminalStatus := containsLower(terminalValues, statusLower)
	isTerminalState := stateLower == "closed"

	if isTerminalStatus || isTerminalState {
		return ActionTerminate
	}

	// Check active conditions
	isActiveStatus := containsLower(activeValues, statusLower)
	isOpenState := stateLower == "open" || stateLower == ""

	if isActiveStatus && isOpenState {
		return ActionKeep
	}

	// Neither active nor terminal
	return ActionStop
}

func containsLower(list []string, val string) bool {
	for _, v := range list {
		if strings.ToLower(v) == val {
			return true
		}
	}
	return false
}
