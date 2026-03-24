package orchestrator

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// WorkItemSource fetches work items from GitHub.
type WorkItemSource interface {
	FetchCandidates(ctx context.Context) ([]WorkItem, error)
	FetchStates(ctx context.Context, workItemIDs []string) ([]WorkItem, error)
}

// WorkerRunner executes a work item and returns the outcome.
type WorkerRunner interface {
	Run(ctx context.Context, item WorkItem, attempt *int) WorkerResult
}

// OrchestratorConfig holds configuration for the orchestrator.
type OrchestratorConfig struct {
	PollIntervalMs      int
	MaxConcurrentAgents int
	StallTimeoutMs      int
	MaxRetryBackoffMs   int
	Eligibility         EligibilityConfig
	ActiveValues        []string
	TerminalValues      []string
}

// Orchestrator owns the poll loop and all mutable scheduling state.
type Orchestrator struct {
	cfg     OrchestratorConfig
	source  WorkItemSource
	runner  WorkerRunner
	state   *State
	results chan WorkerResult
	mu      sync.RWMutex
}

// New creates an Orchestrator.
func New(cfg OrchestratorConfig, source WorkItemSource, runner WorkerRunner) *Orchestrator {
	if cfg.MaxRetryBackoffMs <= 0 {
		cfg.MaxRetryBackoffMs = 300000
	}
	return &Orchestrator{
		cfg:    cfg,
		source: source,
		runner: runner,
		state: &State{
			PollIntervalMs:      cfg.PollIntervalMs,
			MaxConcurrentAgents: cfg.MaxConcurrentAgents,
			Running:             make(map[string]*RunningEntry),
			Claimed:             make(map[string]bool),
			RetryAttempts:       make(map[string]*RetryEntry),
			Completed:           make(map[string]bool),
			HandedOff:           make(map[string]bool),
		},
		results: make(chan WorkerResult, 100),
	}
}

// Run starts the poll loop and blocks until ctx is cancelled.
func (o *Orchestrator) Run(ctx context.Context) {
	slog.Info("orchestrator starting poll loop", "interval_ms", o.cfg.PollIntervalMs)

	// Immediate first tick
	o.RunOnce(ctx)

	ticker := time.NewTicker(time.Duration(o.cfg.PollIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("orchestrator shutting down")
			return
		case result := <-o.results:
			o.handleWorkerResult(result)
		case <-ticker.C:
			o.RunOnce(ctx)
		}
	}
}

// Shutdown gracefully stops all running workers.
func (o *Orchestrator) Shutdown(ctx context.Context) {
	o.mu.Lock()
	running := make(map[string]*RunningEntry, len(o.state.Running))
	for k, v := range o.state.Running {
		running[k] = v
	}
	o.mu.Unlock()

	slog.Info("shutting down orchestrator", "running_count", len(running))

	// Cancel all running workers
	for id, entry := range running {
		if entry.CancelFunc != nil {
			slog.Info("cancelling worker", "work_item_id", id)
			entry.CancelFunc()
		}
	}

	// Wait for workers to drain (with timeout from context)
	deadline, hasDeadline := ctx.Deadline()
	for {
		o.mu.RLock()
		remaining := len(o.state.Running)
		o.mu.RUnlock()

		if remaining == 0 {
			break
		}

		if hasDeadline && time.Now().After(deadline) {
			slog.Warn("shutdown grace period expired", "remaining_workers", remaining)
			break
		}

		// Drain results
		select {
		case result := <-o.results:
			o.handleWorkerResult(result)
		case <-time.After(100 * time.Millisecond):
		}
	}

	o.ProcessResults()
}

// SetPendingRefresh marks the orchestrator for an extra reconciliation pass.
func (o *Orchestrator) SetPendingRefresh() {
	o.mu.Lock()
	o.state.PendingRefresh = true
	o.mu.Unlock()
}

// RestoreRetry adds a retry entry (e.g., from bbolt on startup).
func (o *Orchestrator) RestoreRetry(entry RetryEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state.RetryAttempts[entry.WorkItemID] = &entry
	o.state.Claimed[entry.WorkItemID] = true
}

// RunOnce executes one poll-and-dispatch tick per spec Section 8.1:
// 1. Reconcile running work items
// 2. Fire due retries
// 3. Validate config (caller responsibility)
// 4. Fetch candidates
// 5. Sort and dispatch
func (o *Orchestrator) RunOnce(ctx context.Context) {
	now := time.Now()
	o.mu.Lock()
	o.state.LastPollAt = &now
	o.mu.Unlock()

	// 1. Process pending results
	o.ProcessResults()

	// 2. Reconcile: stall detection
	o.reconcileStalled()

	// 3. Reconcile: GitHub state refresh
	o.reconcileGitHubState(ctx)

	// 4. Fire due retries
	o.fireDueRetries(ctx)

	// 5. Fetch candidates
	items, err := o.source.FetchCandidates(ctx)
	if err != nil {
		slog.Error("candidate fetch failed", "error", err)
		o.mu.Lock()
		o.state.ErrorTotal++
		o.mu.Unlock()
		return
	}

	// 6. Sort for dispatch
	SortForDispatch(items)

	// 7. Dispatch eligible items
	for _, item := range items {
		o.mu.RLock()
		slots := o.cfg.MaxConcurrentAgents - len(o.state.Running)
		o.mu.RUnlock()

		if slots <= 0 {
			break
		}

		o.mu.RLock()
		eligible, reason := IsEligible(item, o.cfg.Eligibility, o.state, o.cfg.MaxConcurrentAgents)
		o.mu.RUnlock()

		if !eligible {
			slog.Debug("item not eligible", "work_item_id", item.WorkItemID, "reason", reason)
			continue
		}

		o.dispatch(ctx, item, nil)
	}
}

func (o *Orchestrator) reconcileStalled() {
	o.mu.RLock()
	stalled := DetectStalled(o.state, o.cfg.StallTimeoutMs)
	o.mu.RUnlock()

	for _, id := range stalled {
		slog.Warn("stalled worker detected, cancelling", "work_item_id", id)
		o.mu.Lock()
		entry, exists := o.state.Running[id]
		if exists && entry.CancelFunc != nil {
			entry.CancelFunc()
			entry.Phase = PhaseStalled
		}
		o.mu.Unlock()
	}
}

func (o *Orchestrator) reconcileGitHubState(ctx context.Context) {
	o.mu.RLock()
	var runningIDs []string
	for id := range o.state.Running {
		runningIDs = append(runningIDs, id)
	}
	pendingRefresh := o.state.PendingRefresh
	o.mu.RUnlock()

	if len(runningIDs) == 0 && !pendingRefresh {
		return
	}

	if len(runningIDs) > 0 {
		refreshed, err := o.source.FetchStates(ctx, runningIDs)
		if err != nil {
			slog.Debug("reconciliation refresh failed, keeping workers", "error", err)
		} else {
			refreshMap := make(map[string]WorkItem, len(refreshed))
			for _, item := range refreshed {
				refreshMap[item.WorkItemID] = item
			}

			o.mu.Lock()
			for _, id := range runningIDs {
				item, found := refreshMap[id]
				if !found {
					continue
				}

				action := ClassifyRefreshed(item, o.cfg.ActiveValues, o.cfg.TerminalValues)
				switch action {
				case ActionTerminate:
					slog.Info("terminating worker (terminal state)", "work_item_id", id)
					if entry, ok := o.state.Running[id]; ok && entry.CancelFunc != nil {
						entry.CancelFunc()
						entry.Phase = PhaseCanceled
					}
				case ActionStop:
					slog.Info("stopping worker (non-active state)", "work_item_id", id)
					if entry, ok := o.state.Running[id]; ok && entry.CancelFunc != nil {
						entry.CancelFunc()
						entry.Phase = PhaseCanceled
					}
				case ActionKeep:
					if entry, ok := o.state.Running[id]; ok {
						entry.WorkItem = item
					}
				}
			}
			o.mu.Unlock()
		}
	}

	if pendingRefresh {
		o.mu.Lock()
		o.state.PendingRefresh = false
		o.mu.Unlock()
	}
}

func (o *Orchestrator) fireDueRetries(ctx context.Context) {
	now := time.Now()

	o.mu.Lock()
	var dueEntries []RetryEntry
	for _, entry := range o.state.RetryAttempts {
		if now.After(entry.DueAt) || now.Equal(entry.DueAt) {
			dueEntries = append(dueEntries, *entry)
		}
	}
	o.mu.Unlock()

	if len(dueEntries) == 0 {
		return
	}

	// Fetch current candidates to check eligibility
	candidates, err := o.source.FetchCandidates(ctx)
	if err != nil {
		slog.Warn("retry fetch failed", "error", err)
		return
	}

	candidateMap := make(map[string]WorkItem, len(candidates))
	for _, c := range candidates {
		candidateMap[c.WorkItemID] = c
	}

	for _, entry := range dueEntries {
		item, found := candidateMap[entry.WorkItemID]
		if !found {
			// Item no longer in candidates — release claim
			slog.Info("retry: item no longer candidate, releasing", "work_item_id", entry.WorkItemID)
			o.mu.Lock()
			delete(o.state.RetryAttempts, entry.WorkItemID)
			delete(o.state.Claimed, entry.WorkItemID)
			o.mu.Unlock()
			continue
		}

		o.mu.RLock()
		eligible, reason := IsEligible(item, o.cfg.Eligibility, o.state, o.cfg.MaxConcurrentAgents)
		o.mu.RUnlock()

		if !eligible {
			if reason == "no available slots" {
				// Requeue with slot error
				slog.Info("retry: no slots, requeuing", "work_item_id", entry.WorkItemID)
				o.mu.Lock()
				o.state.RetryAttempts[entry.WorkItemID].DueAt = time.Now().Add(5 * time.Second)
				o.state.RetryAttempts[entry.WorkItemID].Error = "no available orchestrator slots"
				o.mu.Unlock()
			} else {
				// No longer eligible — release
				slog.Info("retry: item no longer eligible, releasing", "work_item_id", entry.WorkItemID, "reason", reason)
				o.mu.Lock()
				delete(o.state.RetryAttempts, entry.WorkItemID)
				delete(o.state.Claimed, entry.WorkItemID)
				o.mu.Unlock()
			}
			continue
		}

		// Dispatch the retry
		attempt := entry.Attempt
		o.dispatch(ctx, item, &attempt)
	}
}

func (o *Orchestrator) dispatch(ctx context.Context, item WorkItem, attempt *int) {
	// Create a cancellable context for this worker
	workerCtx, cancel := context.WithCancel(ctx)

	o.mu.Lock()
	o.state.Running[item.WorkItemID] = &RunningEntry{
		WorkItem:        item,
		CancelFunc:      cancel,
		IssueIdentifier: item.IssueIdentifier,
		Repository:      repoFullName(item.Repository),
		RetryAttempt:    attempt,
		Phase:           PhaseLaunchingAgent,
		StartedAt:       time.Now(),
	}
	o.state.Claimed[item.WorkItemID] = true
	delete(o.state.RetryAttempts, item.WorkItemID)
	o.state.AgentTotals.SessionsStarted++
	o.state.DispatchTotal++
	o.mu.Unlock()

	slog.Info("dispatching work item",
		"work_item_id", item.WorkItemID,
		"issue", item.IssueIdentifier,
		"repository", repoFullName(item.Repository),
		"attempt", attempt,
	)

	go func() {
		result := o.runner.Run(workerCtx, item, attempt)
		o.results <- result
	}()
}

// ProcessResults drains completed worker results and updates state.
func (o *Orchestrator) ProcessResults() {
	for {
		select {
		case result := <-o.results:
			o.handleWorkerResult(result)
		default:
			return
		}
	}
}

func (o *Orchestrator) handleWorkerResult(result WorkerResult) {
	o.mu.Lock()
	defer o.mu.Unlock()

	entry, exists := o.state.Running[result.WorkItemID]
	if exists {
		elapsed := time.Since(entry.StartedAt).Seconds()
		o.state.AgentTotals.SecondsRunning += elapsed
		o.state.AgentTotals.InputTokens += int64(entry.InputTokens)
		o.state.AgentTotals.OutputTokens += int64(entry.OutputTokens)
		o.state.AgentTotals.TotalTokens += int64(entry.TotalTokens)
	}

	delete(o.state.Running, result.WorkItemID)

	switch result.Outcome {
	case OutcomeHandoff:
		o.state.Completed[result.WorkItemID] = true
		o.state.HandedOff[result.WorkItemID] = true
		delete(o.state.Claimed, result.WorkItemID)
		o.state.HandoffTotal++
		slog.Info("work item handed off", "work_item_id", result.WorkItemID)

	case OutcomeNormal:
		o.state.Completed[result.WorkItemID] = true
		o.state.RetryAttempts[result.WorkItemID] = &RetryEntry{
			WorkItemID:      result.WorkItemID,
			IssueIdentifier: issueIDFromEntry(entry),
			Attempt:         1,
			DueAt:           time.Now().Add(1000 * time.Millisecond),
		}
		slog.Info("work item normal exit, scheduling continuation", "work_item_id", result.WorkItemID)

	case OutcomeFailure:
		attempt := 1
		if entry != nil && entry.RetryAttempt != nil {
			attempt = *entry.RetryAttempt + 1
		}
		backoff := RetryBackoffMs(attempt, o.cfg.MaxRetryBackoffMs)
		o.state.RetryAttempts[result.WorkItemID] = &RetryEntry{
			WorkItemID:      result.WorkItemID,
			IssueIdentifier: issueIDFromEntry(entry),
			Attempt:         attempt,
			DueAt:           time.Now().Add(time.Duration(backoff) * time.Millisecond),
			Error:           errString(result.Error),
		}
		o.state.ErrorTotal++
		slog.Warn("work item failed, scheduling retry",
			"work_item_id", result.WorkItemID,
			"attempt", attempt,
			"backoff_ms", backoff,
			"error", result.Error,
		)
	}
}

// GetState returns a snapshot of the current orchestrator state.
func (o *Orchestrator) GetState() State {
	o.mu.RLock()
	defer o.mu.RUnlock()

	s := *o.state
	running := make(map[string]*RunningEntry, len(o.state.Running))
	for k, v := range o.state.Running {
		running[k] = v
	}
	s.Running = running

	retries := make(map[string]*RetryEntry, len(o.state.RetryAttempts))
	for k, v := range o.state.RetryAttempts {
		retries[k] = v
	}
	s.RetryAttempts = retries

	return s
}

// GetRetryEntries returns all current retry entries (for persisting to bbolt).
func (o *Orchestrator) GetRetryEntries() []RetryEntry {
	o.mu.RLock()
	defer o.mu.RUnlock()
	entries := make([]RetryEntry, 0, len(o.state.RetryAttempts))
	for _, e := range o.state.RetryAttempts {
		entries = append(entries, *e)
	}
	return entries
}

// InjectRunning adds a running entry directly (for testing).
func (o *Orchestrator) InjectRunning(id string, entry *RunningEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state.Running[id] = entry
	o.state.Claimed[id] = true
}

func repoFullName(r *Repository) string {
	if r == nil {
		return ""
	}
	return r.FullName
}

func issueIDFromEntry(entry *RunningEntry) string {
	if entry == nil {
		return ""
	}
	return entry.IssueIdentifier
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
