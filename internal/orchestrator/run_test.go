package orchestrator_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

type countingSource struct {
	fetchCount atomic.Int32
}

func (s *countingSource) FetchCandidates(_ context.Context) ([]orchestrator.WorkItem, error) {
	s.fetchCount.Add(1)
	return nil, nil
}

func (s *countingSource) FetchStates(_ context.Context, _ []string) ([]orchestrator.WorkItem, error) {
	return nil, nil
}

type noopRunner struct{}

func (n *noopRunner) Run(_ context.Context, item orchestrator.WorkItem, _ *int) orchestrator.WorkerResult {
	return orchestrator.WorkerResult{WorkItemID: item.WorkItemID, Outcome: orchestrator.OutcomeNormal}
}

func TestOrchestrator_Run_TicksOnInterval(t *testing.T) {
	source := &countingSource{}
	runner := &noopRunner{}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      50, // 50ms interval for fast test
		MaxConcurrentAgents: 5,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo"},
			ExecutableItemTypes: []string{"issue"},
		},
	}, source, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	orch.Run(ctx)

	// At 50ms interval over 200ms, should get ~4-5 ticks (1 immediate + 3-4 from ticker)
	count := source.fetchCount.Load()
	if count < 3 {
		t.Errorf("expected at least 3 fetches in 200ms at 50ms interval, got %d", count)
	}
}

func TestOrchestrator_SetPendingRefresh(t *testing.T) {
	source := &countingSource{}
	runner := &noopRunner{}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      1000,
		MaxConcurrentAgents: 5,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo"},
			ExecutableItemTypes: []string{"issue"},
		},
	}, source, runner)

	orch.SetPendingRefresh()
	state := orch.GetState()
	if !state.PendingRefresh {
		t.Error("expected PendingRefresh=true after SetPendingRefresh")
	}
}
