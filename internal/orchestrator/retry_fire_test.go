package orchestrator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

// retrySource tracks calls and returns configurable items.
type retrySource struct {
	mu    sync.Mutex
	items []orchestrator.WorkItem
	calls int
}

func (s *retrySource) FetchCandidates(_ context.Context) ([]orchestrator.WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.items, nil
}

func (s *retrySource) FetchStates(_ context.Context, _ []string) ([]orchestrator.WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items, nil
}

func TestOrchestrator_RetryTimerFires(t *testing.T) {
	num := 1
	item := orchestrator.WorkItem{
		WorkItemID: "github:p1:i1", ProjectItemID: "p1",
		ContentType: "issue", Title: "Retry test", State: "open",
		ProjectStatus: "Todo", IssueNumber: &num,
		Repository: &orchestrator.Repository{FullName: "org/repo"},
	}

	source := &retrySource{items: []orchestrator.WorkItem{item}}
	runner := &mockRunner{}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      50,
		MaxConcurrentAgents: 5,
		MaxRetryBackoffMs:   300000,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo"},
			ExecutableItemTypes: []string{"issue"},
		},
		ActiveValues:   []string{"Todo"},
		TerminalValues: []string{"Done"},
	}, source, runner)

	// Manually inject a due retry entry
	orch.RestoreRetry(orchestrator.RetryEntry{
		WorkItemID: "github:p1:i1",
		Attempt:    1,
		DueAt:      time.Now().Add(-1 * time.Second), // already due
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	orch.Run(ctx)

	// The retry should have fired and dispatched the item
	runner.mu.Lock()
	launched := len(runner.launched)
	runner.mu.Unlock()

	if launched < 1 {
		t.Errorf("expected retry to fire and dispatch, got %d launches", launched)
	}
}

func TestOrchestrator_RetryReleasesNonEligible(t *testing.T) {
	// Source returns empty — item is gone
	source := &retrySource{items: nil}
	runner := &noopRunner{}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      50,
		MaxConcurrentAgents: 5,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo"},
			ExecutableItemTypes: []string{"issue"},
		},
	}, source, runner)

	orch.RestoreRetry(orchestrator.RetryEntry{
		WorkItemID: "github:p1:gone",
		Attempt:    1,
		DueAt:      time.Now().Add(-1 * time.Second),
	})

	// Run one tick
	orch.RunOnce(context.Background())

	state := orch.GetState()
	if _, exists := state.RetryAttempts["github:p1:gone"]; exists {
		t.Error("expected retry to be released for missing item")
	}
	if state.Claimed["github:p1:gone"] {
		t.Error("expected claim to be released")
	}
}

func TestOrchestrator_ReconciliationTerminatesWorker(t *testing.T) {
	num := 1
	source := &retrySource{
		items: []orchestrator.WorkItem{{
			WorkItemID: "github:p1:i1", ProjectItemID: "p1",
			ContentType: "issue", Title: "Closing", State: "closed",
			ProjectStatus: "Done", IssueNumber: &num,
			Repository: &orchestrator.Repository{FullName: "org/repo"},
		}},
	}

	// Use a slow runner so the item is still "running" when reconciliation happens
	slowRunner := &slowMockRunner{delay: 2 * time.Second}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      50,
		MaxConcurrentAgents: 5,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo"},
			ExecutableItemTypes: []string{"issue"},
		},
		ActiveValues:   []string{"Todo"},
		TerminalValues: []string{"Done"},
	}, source, slowRunner)

	// Manually add a running entry
	ctx := context.Background()
	workerCtx, cancel := context.WithCancel(ctx)
	orch.InjectRunning("github:p1:i1", &orchestrator.RunningEntry{
		WorkItem: orchestrator.WorkItem{
			WorkItemID: "github:p1:i1", ProjectStatus: "Todo", State: "open",
		},
		CancelFunc: cancel,
		StartedAt:  time.Now(),
	})

	// Run reconciliation
	orch.RunOnce(ctx)

	// The cancel should have been called
	select {
	case <-workerCtx.Done():
		// Good — worker was cancelled
	default:
		t.Error("expected worker to be cancelled by reconciliation")
	}
}

func TestOrchestrator_ShutdownCancelsWorkers(t *testing.T) {
	source := &retrySource{}
	runner := &slowMockRunner{delay: 5 * time.Second}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      1000,
		MaxConcurrentAgents: 5,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo"},
			ExecutableItemTypes: []string{"issue"},
		},
	}, source, runner)

	// Add a running entry with cancel
	_, cancel := context.WithCancel(context.Background())
	orch.InjectRunning("github:p1:i1", &orchestrator.RunningEntry{
		WorkItem:   orchestrator.WorkItem{WorkItemID: "github:p1:i1"},
		CancelFunc: cancel,
		StartedAt:  time.Now(),
	})

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer shutdownCancel()

	orch.Shutdown(shutdownCtx)

	// Should have processed without hanging
}

type slowMockRunner struct {
	delay time.Duration
}

func (r *slowMockRunner) Run(ctx context.Context, item orchestrator.WorkItem, _ *int) orchestrator.WorkerResult {
	select {
	case <-ctx.Done():
		return orchestrator.WorkerResult{WorkItemID: item.WorkItemID, Outcome: orchestrator.OutcomeFailure, Error: ctx.Err()}
	case <-time.After(r.delay):
		return orchestrator.WorkerResult{WorkItemID: item.WorkItemID, Outcome: orchestrator.OutcomeNormal}
	}
}
