package engine

import (
	"time"

	"github.com/shivamstaq/github-symphony/internal/domain"
)

// detectStalls scans running entries and fires EvtStallDetected for workers
// that haven't reported activity within the configured stall timeout.
func (e *Engine) detectStalls() {
	stallTimeoutMs := e.cfg.Agent.StallTimeoutMs
	if stallTimeoutMs <= 0 {
		return // stall detection disabled
	}
	threshold := time.Duration(stallTimeoutMs) * time.Millisecond
	now := time.Now()

	for itemID, entry := range e.state.Running {
		if entry.Phase != domain.StateRunning {
			continue
		}
		elapsed := now.Sub(entry.LastActivityAt)
		if elapsed > threshold {
			e.Emit(NewEvent(EvtStallDetected, itemID, StallDetectedPayload{
				LastActivity: entry.LastActivityAt,
				Threshold:    threshold,
			}))
		}
	}
}

// handleStallDetected kills the stalled worker and transitions to needs_human.
func (e *Engine) handleStallDetected(evt EngineEvent) {
	itemID := evt.ItemID
	entry, ok := e.state.Running[itemID]
	if !ok {
		return
	}

	payload := evt.Payload.(StallDetectedPayload)
	e.logger.Warn("stall detected, killing worker",
		"item", entry.WorkItem.IssueIdentifier,
		"last_activity", payload.LastActivity,
		"threshold", payload.Threshold,
	)

	// Kill the worker process
	entry.CancelFunc()
	delete(e.state.Running, itemID)

	// FSM: running -> needs_human
	_, _ = e.transition(itemID, domain.EventStallDetected, nil)
}
