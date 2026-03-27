package engine

import (
	"context"
	"strings"

	"github.com/shivamstaq/github-symphony/internal/domain"
)

// ReconcileAction describes what to do with a running item after state refresh.
type ReconcileAction string

const (
	ActionKeep      ReconcileAction = "keep"      // still active, continue
	ActionTerminate ReconcileAction = "terminate"  // terminal status or closed
	ActionStop      ReconcileAction = "stop"       // non-active, non-terminal
)

// ClassifyRefreshed determines the reconciliation action for a work item
// based on its refreshed state from the tracker.
func ClassifyRefreshed(item domain.WorkItem, activeValues, terminalValues []string) ReconcileAction {
	// Closed issue → terminate
	if item.State != "" && strings.ToLower(item.State) == "closed" {
		return ActionTerminate
	}

	// Terminal project status → terminate
	for _, v := range terminalValues {
		if strings.EqualFold(item.ProjectStatus, v) {
			return ActionTerminate
		}
	}

	// Active status + open → keep running
	for _, v := range activeValues {
		if strings.EqualFold(item.ProjectStatus, v) {
			if item.State == "" || strings.ToLower(item.State) == "open" {
				return ActionKeep
			}
		}
	}

	// Not active, not terminal → stop (moved to a non-active status)
	return ActionStop
}

// reconcileRunningItems refreshes the state of all running items from the tracker
// and applies the appropriate action.
func (e *Engine) reconcileRunningItems(ctx context.Context) {
	if len(e.state.Running) == 0 || e.tracker == nil {
		return
	}

	// Collect IDs of running items
	ids := make([]string, 0, len(e.state.Running))
	for id := range e.state.Running {
		ids = append(ids, id)
	}

	// Fetch fresh state from tracker
	items, err := e.tracker.FetchStates(ctx, ids)
	if err != nil {
		e.logger.Error("reconcile fetch failed", "error", err)
		return
	}

	// Build lookup
	fresh := make(map[string]domain.WorkItem, len(items))
	for _, item := range items {
		fresh[item.WorkItemID] = item
	}

	// Apply actions
	for itemID, entry := range e.state.Running {
		item, found := fresh[itemID]
		if !found {
			// Item disappeared from project — treat as terminated
			e.logger.Warn("item missing from tracker, cancelling", "item", entry.WorkItem.IssueIdentifier)
			entry.CancelFunc()
			delete(e.state.Running, itemID)
			_, _ = e.transition(itemID, domain.EventCancelled, nil)
			continue
		}

		action := ClassifyRefreshed(item, e.cfg.Tracker.ActiveValues, e.cfg.Tracker.TerminalValues)
		switch action {
		case ActionKeep:
			// Update snapshot
			entry.WorkItem = item
		case ActionTerminate, ActionStop:
			e.logger.Info("reconcile: stopping worker",
				"item", entry.WorkItem.IssueIdentifier,
				"action", action,
				"status", item.ProjectStatus,
				"state", item.State,
			)
			entry.CancelFunc()
			delete(e.state.Running, itemID)
			_, _ = e.transition(itemID, domain.EventCancelled, nil)
		}
	}
}
