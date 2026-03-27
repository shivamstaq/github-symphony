package engine

import (
	"context"
	"testing"
	"time"

	agentmock "github.com/shivamstaq/github-symphony/internal/agent/mock"
	"github.com/shivamstaq/github-symphony/internal/domain"
)

func TestBudget_TokenLimitExceeded(t *testing.T) {
	item := makeItem("60", 60)
	// Use slow agent so it doesn't complete before budget check
	mockAgent := &agentmock.MockAgent{
		StopReason: "completed",
		NumTurns:   1,
		HasCommits: true,
		Delay:      5 * time.Second,
	}
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)
	eng.cfg.Agent.Budget.MaxTokensPerItem = 500

	ctx := context.Background()
	eng.handlePollTick(ctx)

	// Wait for dispatch, then simulate high token usage
	time.Sleep(20 * time.Millisecond)
	if entry, ok := eng.state.Running["60"]; ok {
		entry.TotalTokens = 600 // over limit
	}

	// Budget check is synchronous — immediately transitions to needs_human
	eng.checkBudget("60")

	if eng.state.ItemState("60") != domain.StateNeedsHuman {
		t.Errorf("expected needs_human after budget exceeded, got %q", eng.state.ItemState("60"))
	}
}

func TestBudget_CostLimitExceeded(t *testing.T) {
	item := makeItem("61", 61)
	mockAgent := &agentmock.MockAgent{
		StopReason: "completed",
		NumTurns:   1,
		HasCommits: true,
		Delay:      5 * time.Second,
	}
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)
	eng.cfg.Agent.Budget.MaxCostPerItemUSD = 1.0

	ctx := context.Background()
	eng.handlePollTick(ctx)

	time.Sleep(20 * time.Millisecond)
	if entry, ok := eng.state.Running["61"]; ok {
		entry.CostUSD = 1.5
	}

	eng.checkBudget("61")

	if eng.state.ItemState("61") != domain.StateNeedsHuman {
		t.Errorf("expected needs_human after cost exceeded, got %q", eng.state.ItemState("61"))
	}
}

func TestBudget_UnderLimitNoAction(t *testing.T) {
	item := makeItem("62", 62)
	mockAgent := agentmock.NewSuccessAgent()
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)
	eng.cfg.Agent.Budget.MaxTokensPerItem = 100000

	ctx := context.Background()
	eng.handlePollTick(ctx)

	time.Sleep(20 * time.Millisecond)
	if entry, ok := eng.state.Running["62"]; ok {
		entry.TotalTokens = 500
	}

	eng.checkBudget("62")

	// No budget event should be emitted
	select {
	case evt := <-eng.eventCh:
		if evt.Type == EvtBudgetExceeded {
			t.Error("should not trigger budget exceeded when under limit")
		}
		eng.handleEvent(ctx, evt)
	default:
		// Good
	}
}

func TestBudget_ZeroMeansNoLimit(t *testing.T) {
	item := makeItem("63", 63)
	mockAgent := agentmock.NewSuccessAgent()
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)
	// All budget values default to 0 = no limit

	ctx := context.Background()
	eng.handlePollTick(ctx)

	time.Sleep(20 * time.Millisecond)
	if entry, ok := eng.state.Running["63"]; ok {
		entry.TotalTokens = 999999999
		entry.CostUSD = 999999.0
	}

	eng.checkBudget("63")

	select {
	case evt := <-eng.eventCh:
		if evt.Type == EvtBudgetExceeded {
			t.Error("budget with zero limits should never trigger")
		}
		eng.handleEvent(ctx, evt)
	default:
		// Good
	}
}
