package scenario

import (
	"fmt"
	"testing"
	"time"

	agentmock "github.com/shivamstaq/github-symphony/internal/agent/mock"
	"github.com/shivamstaq/github-symphony/internal/config"
	"github.com/shivamstaq/github-symphony/internal/domain"
	"github.com/shivamstaq/github-symphony/internal/engine"
)

func makeItem(id string, num int) domain.WorkItem {
	return domain.WorkItem{
		WorkItemID:      id,
		ProjectItemID:   "proj-" + id,
		ContentType:     "issue",
		IssueNumber:     &num,
		IssueIdentifier: "org/repo#" + id,
		Title:           "Issue " + id,
		Description:     "Fix something in " + id,
		State:           "open",
		ProjectStatus:   "Todo",
	}
}

// Scenario 1: Happy path — dispatch → agent completes with commits → handed off
func TestScenario_HappyPath(t *testing.T) {
	item := makeItem("1", 1)
	h := NewHarness(t, HarnessConfig{Items: []domain.WorkItem{item}})
	defer h.Cleanup()

	h.PollOnce()
	h.WaitForState("1", domain.StateHandedOff, 3*time.Second)

	h.AssertState("1", domain.StateHandedOff)
	if !h.IsHandedOff("1") {
		t.Error("item should be in HandedOff map")
	}
	if h.DispatchTotal() != 1 {
		t.Errorf("expected 1 dispatch, got %d", h.DispatchTotal())
	}
}

// Scenario 2: Retry exhaustion — agent fails repeatedly → failed state
func TestScenario_RetryExhaustion(t *testing.T) {
	item := makeItem("2", 2)
	h := NewHarness(t, HarnessConfig{
		Items:      []domain.WorkItem{item},
		Agent:      agentmock.NewFailAgent(fmt.Errorf("build failed")),
		MaxRetries: 2,
	})
	defer h.Cleanup()

	// First attempt
	h.PollOnce()
	h.WaitForState("2", domain.StateQueued, 2*time.Second)

	// Should have retry scheduled
	if !h.HasRetry("2") {
		t.Fatal("expected retry after first failure")
	}
	if h.RetryAttempt("2") != 1 {
		t.Errorf("expected attempt=1, got %d", h.RetryAttempt("2"))
	}
}

// Scenario 3: No commits → needs_human (NOT retry)
func TestScenario_NoCommitsEscalation(t *testing.T) {
	item := makeItem("3", 3)
	h := NewHarness(t, HarnessConfig{
		Items: []domain.WorkItem{item},
		Agent: agentmock.NewNoCommitsAgent(),
	})
	defer h.Cleanup()

	h.PollOnce()
	h.WaitForState("3", domain.StateNeedsHuman, 3*time.Second)

	h.AssertState("3", domain.StateNeedsHuman)

	// Must NOT be in retry queue
	if h.HasRetry("3") {
		t.Error("no-commits exit must NOT schedule a retry — this is the cost leak bug")
	}
}

// Scenario 4: Stall recovery — agent stalls → needs_human
func TestScenario_StallRecovery(t *testing.T) {
	item := makeItem("4", 4)
	h := NewHarness(t, HarnessConfig{
		Items: []domain.WorkItem{item},
		Agent: &agentmock.MockAgent{
			StopReason: "completed",
			NumTurns:   1,
			HasCommits: true,
			Delay:      10 * time.Second, // very slow agent
		},
		StallTimeoutMs: 50, // 50ms timeout
	})
	defer h.Cleanup()

	h.PollOnce()

	// Wait for dispatch
	time.Sleep(30 * time.Millisecond)

	// Backdate activity to trigger stall
	if entry := h.RunningEntry("4"); entry != nil {
		entry.LastActivityAt = time.Now().Add(-200 * time.Millisecond)
	}

	// Trigger another poll (which runs stall detection)
	h.PollOnce()
	h.WaitForState("4", domain.StateNeedsHuman, 2*time.Second)

	h.AssertState("4", domain.StateNeedsHuman)
	if h.IsRunning("4") {
		t.Error("stalled worker should be removed from running")
	}
}

// Scenario 5: Reconcile closed — issue closed while agent running → cancelled
func TestScenario_ReconcileClosed(t *testing.T) {
	item := makeItem("5", 5)
	h := NewHarness(t, HarnessConfig{
		Items: []domain.WorkItem{item},
		Agent: &agentmock.MockAgent{
			StopReason: "completed",
			NumTurns:   1,
			HasCommits: true,
			Delay:      5 * time.Second, // slow agent
		},
	})
	defer h.Cleanup()

	h.PollOnce()
	time.Sleep(30 * time.Millisecond)

	// Simulate issue being closed externally
	closedItem := item
	closedItem.State = "closed"
	h.SetTrackerItems([]domain.WorkItem{closedItem})

	// Next poll triggers reconciliation
	h.PollOnce()
	h.WaitForState("5", domain.StateOpen, 2*time.Second)

	// Item should be released (back to open = not tracked)
	h.AssertState("5", domain.StateOpen)
	if h.IsRunning("5") {
		t.Error("cancelled worker should be removed from running")
	}
}

// Scenario 6: Budget exceeded — tokens over limit → needs_human
func TestScenario_BudgetExceeded(t *testing.T) {
	item := makeItem("6", 6)
	h := NewHarness(t, HarnessConfig{
		Items: []domain.WorkItem{item},
		Agent: &agentmock.MockAgent{
			StopReason: "completed",
			NumTurns:   1,
			HasCommits: true,
			Delay:      5 * time.Second,
		},
		Budget: config.BudgetConfig{
			MaxTokensPerItem: 100,
		},
	})
	defer h.Cleanup()

	h.PollOnce()
	time.Sleep(30 * time.Millisecond)

	// Simulate token usage exceeding budget
	if entry := h.RunningEntry("6"); entry != nil {
		entry.TotalTokens = 200
	}

	// Budget check happens on agent updates — simulate one
	h.Emit(engine.NewEvent(engine.EvtAgentUpdate, "6", engine.AgentUpdatePayload{
		Update: agentmock.MakeUpdate("tokens", 200),
	}))
	h.DrainEvents(time.Second)

	h.AssertState("6", domain.StateNeedsHuman)
}

// Scenario 7: Handed-off item is NOT re-dispatched
func TestScenario_HandedOffNotRedispatched(t *testing.T) {
	item := makeItem("7", 7)
	h := NewHarness(t, HarnessConfig{Items: []domain.WorkItem{item}})
	defer h.Cleanup()

	// First poll — dispatch and handoff
	h.PollOnce()
	h.WaitForState("7", domain.StateHandedOff, 3*time.Second)

	prevDispatches := h.DispatchTotal()

	// Second poll — item still in tracker but should NOT be re-dispatched
	h.PollOnce()

	h.AssertNotDispatched(prevDispatches)
	h.AssertState("7", domain.StateHandedOff)
}
