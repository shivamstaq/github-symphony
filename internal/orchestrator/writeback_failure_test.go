package orchestrator_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

// mockFailingRunner simulates a worker that encounters a write-back failure.
type mockFailingRunner struct{}

func (m *mockFailingRunner) Run(_ context.Context, item orchestrator.WorkItem, _ *int) orchestrator.WorkerResult {
	return orchestrator.WorkerResult{
		WorkItemID: item.WorkItemID,
		Outcome:    orchestrator.OutcomeFailure,
		Error:      fmt.Errorf("github_api_status: 422: validation failed"),
	}
}

func (m *mockFailingRunner) FetchCandidates(_ context.Context) ([]orchestrator.WorkItem, error) {
	return nil, nil
}

func (m *mockFailingRunner) FetchStates(_ context.Context, _ []string) ([]orchestrator.WorkItem, error) {
	return nil, nil
}

func TestOrchestrator_WritebackFailureSchedulesRetry(t *testing.T) {
	source := &mockSource{
		items: []orchestrator.WorkItem{
			{
				WorkItemID:    "github:item1:issue1",
				ProjectItemID: "item1",
				ContentType:   "issue",
				Title:         "Writeback fail test",
				State:         "open",
				ProjectStatus: "Todo",
				IssueNumber:   intPtr(1),
				Repository:    &orchestrator.Repository{Owner: "org", Name: "repo", FullName: "org/repo"},
			},
		},
	}

	runner := &mockFailingRunner{}

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      100,
		MaxConcurrentAgents: 5,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        []string{"Todo"},
			ExecutableItemTypes: []string{"issue"},
		},
	}, source, runner)

	ctx := context.Background()
	orch.RunOnce(ctx)

	// Wait for the failing worker to complete
	time.Sleep(100 * time.Millisecond)
	orch.ProcessResults()

	// Verify the failure was recorded as a retry
	state := orch.GetState()

	if len(state.Running) != 0 {
		t.Errorf("expected 0 running after failure, got %d", len(state.Running))
	}

	retry, ok := state.RetryAttempts["github:item1:issue1"]
	if !ok {
		t.Fatal("expected retry entry for failed work item")
	}
	if retry.Attempt != 1 {
		t.Errorf("expected retry attempt=1, got %d", retry.Attempt)
	}
	if retry.Error == "" {
		t.Error("expected retry error to contain failure reason")
	}
}
