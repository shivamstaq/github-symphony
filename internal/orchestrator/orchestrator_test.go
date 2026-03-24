package orchestrator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

// mockSource implements orchestrator.WorkItemSource for testing.
type mockSource struct {
	mu    sync.Mutex
	items []orchestrator.WorkItem
}

func (m *mockSource) FetchCandidates(_ context.Context) ([]orchestrator.WorkItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.items, nil
}

func (m *mockSource) FetchStates(_ context.Context, _ []string) ([]orchestrator.WorkItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.items, nil
}

// mockRunner implements orchestrator.WorkerRunner for testing.
type mockRunner struct {
	mu       sync.Mutex
	launched []string
}

func (m *mockRunner) Run(_ context.Context, item orchestrator.WorkItem, _ *int) orchestrator.WorkerResult {
	m.mu.Lock()
	m.launched = append(m.launched, item.WorkItemID)
	m.mu.Unlock()

	// Simulate agent doing work
	time.Sleep(50 * time.Millisecond)

	return orchestrator.WorkerResult{
		WorkItemID: item.WorkItemID,
		Outcome:    orchestrator.OutcomeNormal,
	}
}

func TestOrchestrator_DispatchesSingleItem(t *testing.T) {
	source := &mockSource{
		items: []orchestrator.WorkItem{
			{
				WorkItemID:    "github:item1:issue1",
				ProjectItemID: "item1",
				ContentType:   "issue",
				Title:         "Test issue",
				State:         "open",
				ProjectStatus: "Todo",
				IssueNumber:   intPtr(1),
				Repository:    &orchestrator.Repository{Owner: "org", Name: "repo", FullName: "org/repo"},
			},
		},
	}

	runner := &mockRunner{}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      100,
		MaxConcurrentAgents: 5,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo", "In Progress"},
			TerminalValues:      []string{"Done"},
			ExecutableItemTypes: []string{"issue"},
		},
	}, source, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	orch.RunOnce(ctx)

	// Wait for worker to complete
	time.Sleep(200 * time.Millisecond)
	orch.ProcessResults()

	runner.mu.Lock()
	defer runner.mu.Unlock()

	if len(runner.launched) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(runner.launched))
	}
	if runner.launched[0] != "github:item1:issue1" {
		t.Errorf("wrong item dispatched: %s", runner.launched[0])
	}
}

func TestOrchestrator_RespectsMaxConcurrency(t *testing.T) {
	source := &mockSource{
		items: []orchestrator.WorkItem{
			{WorkItemID: "i1", ProjectItemID: "p1", ContentType: "issue", Title: "A", State: "open", ProjectStatus: "Todo", IssueNumber: intPtr(1), Repository: &orchestrator.Repository{FullName: "o/r"}},
			{WorkItemID: "i2", ProjectItemID: "p2", ContentType: "issue", Title: "B", State: "open", ProjectStatus: "Todo", IssueNumber: intPtr(2), Repository: &orchestrator.Repository{FullName: "o/r"}},
			{WorkItemID: "i3", ProjectItemID: "p3", ContentType: "issue", Title: "C", State: "open", ProjectStatus: "Todo", IssueNumber: intPtr(3), Repository: &orchestrator.Repository{FullName: "o/r"}},
		},
	}

	runner := &mockRunner{}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      100,
		MaxConcurrentAgents: 2, // Only 2 slots
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo"},
			ExecutableItemTypes: []string{"issue"},
		},
	}, source, runner)

	ctx := context.Background()
	orch.RunOnce(ctx)

	// Check that only 2 were dispatched (max concurrency)
	state := orch.GetState()
	if len(state.Running) > 2 {
		t.Errorf("expected at most 2 running, got %d", len(state.Running))
	}
}
