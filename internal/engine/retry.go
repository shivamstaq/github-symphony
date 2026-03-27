package engine

import (
	"context"
	"time"

	"github.com/shivamstaq/github-symphony/internal/agent"
	"github.com/shivamstaq/github-symphony/internal/domain"
)

// RetryDelay calculates exponential backoff capped at maxMs.
// Base delay is 10s, doubling each attempt: 10s, 20s, 40s, 80s, ...
func RetryDelay(attempt, maxMs int) time.Duration {
	delayMs := 10000 // 10s base
	for i := 1; i < attempt; i++ {
		delayMs *= 2
	}
	if maxMs > 0 && delayMs > maxMs {
		delayMs = maxMs
	}
	return time.Duration(delayMs) * time.Millisecond
}

// scheduleRetry adds a work item to the retry queue with exponential backoff.
func (e *Engine) scheduleRetry(itemID string, item domain.WorkItem, attempt int, errMsg string) {
	delay := RetryDelay(attempt, e.cfg.Agent.MaxRetryBackoffMs)
	e.state.RetryQueue[itemID] = &RetryEntry{
		WorkItemID:      itemID,
		IssueIdentifier: item.IssueIdentifier,
		Attempt:         attempt,
		DueAt:           time.Now().Add(delay),
		Error:           errMsg,
		WorkItem:        &item, // store for re-dispatch
	}
	e.logger.Info("retry scheduled",
		"item", item.IssueIdentifier,
		"attempt", attempt,
		"due_in", delay,
	)
}

// handleWorkerError decides between retry and failure based on retry count.
// Called when an agent exits with an error or StopFailed.
func (e *Engine) handleWorkerError(itemID string, item domain.WorkItem, result agent.Result, previousAttempt int) {
	maxRetries := e.cfg.Agent.MaxContinuationRetries

	if previousAttempt >= maxRetries {
		// Max retries exhausted → failed (terminal)
		_, _ = e.transition(itemID, domain.EventError, func(g domain.TransitionGuard) bool {
			return g == domain.GuardMaxRetriesExhausted
		})
		e.state.ErrorTotal++
		e.logger.Warn("max retries exhausted",
			"item", item.IssueIdentifier,
			"attempts", previousAttempt,
		)
	} else {
		// Has retries left → queued + schedule retry
		_, _ = e.transition(itemID, domain.EventError, func(g domain.TransitionGuard) bool {
			return g == domain.GuardHasRetriesLeft
		})
		errMsg := ""
		if result.Error != nil {
			errMsg = result.Error.Error()
		}
		e.scheduleRetry(itemID, item, previousAttempt+1, errMsg)
	}
}

// fireDueRetries scans the retry queue and emits EvtRetryDue for entries past their due time.
func (e *Engine) fireDueRetries() {
	now := time.Now()
	for itemID, re := range e.state.RetryQueue {
		if now.After(re.DueAt) {
			e.Emit(NewEvent(EvtRetryDue, itemID, RetryDuePayload{Attempt: re.Attempt}))
		}
	}
}

// handleRetryDue re-dispatches a work item whose retry timer has fired.
func (e *Engine) handleRetryDue(ctx context.Context, evt EngineEvent) {
	re, ok := e.state.RetryQueue[evt.ItemID]
	if !ok {
		return
	}

	// Remove from retry queue
	delete(e.state.RetryQueue, evt.ItemID)

	if re.WorkItem == nil {
		e.logger.Warn("retry entry has no stored work item, skipping", "item", evt.ItemID)
		// Release the item back to open so it can be re-fetched
		e.state.SetItemState(evt.ItemID, domain.StateOpen)
		return
	}

	e.logger.Info("retry firing", "item", re.IssueIdentifier, "attempt", re.Attempt)

	// Re-check eligibility before re-dispatch
	eligible, reason := IsEligible(*re.WorkItem, e.eligCfg, e.state, e.cfg.Agent.MaxConcurrent)
	if !eligible {
		e.logger.Info("retry item no longer eligible, releasing",
			"item", re.IssueIdentifier,
			"reason", reason,
		)
		e.state.SetItemState(evt.ItemID, domain.StateOpen)
		return
	}

	// Dispatch with retry attempt tracking
	if err := e.dispatchItemWithRetry(ctx, *re.WorkItem, re.Attempt); err != nil {
		e.logger.Error("retry dispatch failed",
			"item", re.IssueIdentifier,
			"error", err,
		)
		// Re-queue with incremented attempt
		e.scheduleRetry(evt.ItemID, *re.WorkItem, re.Attempt+1, err.Error())
	}
}

// dispatchItemWithRetry is like dispatchItem but carries the retry attempt count.
func (e *Engine) dispatchItemWithRetry(ctx context.Context, item domain.WorkItem, attempt int) error {
	// The item is already in StateQueued from the error transition.
	// Transition: queued -> preparing
	_, err := e.transition(item.WorkItemID, domain.EventDispatch, func(g domain.TransitionGuard) bool {
		return g == domain.GuardConcurrencyOK || g == domain.GuardNone
	})
	if err != nil {
		return err
	}

	e.state.DispatchTotal++

	workerCtx, cancel := context.WithCancel(ctx)
	entry := &RunningEntry{
		WorkItem:       item,
		CancelFunc:     cancel,
		Phase:          domain.StatePreparing,
		RetryAttempt:   attempt,
		StartedAt:      time.Now(),
		LastActivityAt: time.Now(),
	}
	e.state.Running[item.WorkItemID] = entry

	_, _ = e.transition(item.WorkItemID, domain.EventWorkspaceReady, nil)
	entry.Phase = domain.StateRunning

	go e.runWorker(workerCtx, item, entry)

	return nil
}
