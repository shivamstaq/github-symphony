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
	Eligibility         EligibilityConfig
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
		},
		results: make(chan WorkerResult, 100),
	}
}

// RunOnce executes one poll-and-dispatch tick.
func (o *Orchestrator) RunOnce(ctx context.Context) {
	// 1. Process any pending worker results first
	o.ProcessResults()

	// 2. Fetch candidates
	items, err := o.source.FetchCandidates(ctx)
	if err != nil {
		slog.Error("candidate fetch failed", "error", err)
		return
	}

	// 3. Sort for dispatch
	SortForDispatch(items)

	// 4. Dispatch eligible items
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

func (o *Orchestrator) dispatch(ctx context.Context, item WorkItem, attempt *int) {
	o.mu.Lock()
	o.state.Running[item.WorkItemID] = &RunningEntry{
		WorkItem:        item,
		IssueIdentifier: item.IssueIdentifier,
		Repository:      item.Repository.FullName,
		RetryAttempt:    attempt,
		StartedAt:       time.Now(),
	}
	o.state.Claimed[item.WorkItemID] = true
	delete(o.state.RetryAttempts, item.WorkItemID)
	o.state.AgentTotals.SessionsStarted++
	o.mu.Unlock()

	slog.Info("dispatching work item",
		"work_item_id", item.WorkItemID,
		"issue", item.IssueIdentifier,
		"repository", item.Repository.FullName,
	)

	// Launch worker goroutine
	go func() {
		result := o.runner.Run(ctx, item, attempt)
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
		// Accumulate runtime
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
		delete(o.state.Claimed, result.WorkItemID)
		slog.Info("work item handed off", "work_item_id", result.WorkItemID)

	case OutcomeNormal:
		// Normal exit without handoff: schedule continuation retry
		o.state.Completed[result.WorkItemID] = true
		o.state.RetryAttempts[result.WorkItemID] = &RetryEntry{
			WorkItemID: result.WorkItemID,
			Attempt:    1,
			DueAt:      time.Now().Add(1000 * time.Millisecond),
		}
		slog.Info("work item normal exit, scheduling continuation", "work_item_id", result.WorkItemID)

	case OutcomeFailure:
		// Schedule exponential backoff retry
		attempt := 1
		if entry != nil && entry.RetryAttempt != nil {
			attempt = *entry.RetryAttempt + 1
		}
		backoff := min(10000*(1<<(attempt-1)), 300000) // capped at 5 min
		o.state.RetryAttempts[result.WorkItemID] = &RetryEntry{
			WorkItemID: result.WorkItemID,
			Attempt:    attempt,
			DueAt:      time.Now().Add(time.Duration(backoff) * time.Millisecond),
			Error:      errString(result.Error),
		}
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

	// Return a shallow copy
	s := *o.state
	running := make(map[string]*RunningEntry, len(o.state.Running))
	for k, v := range o.state.Running {
		running[k] = v
	}
	s.Running = running
	return s
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
