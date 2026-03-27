package engine

import (
	"context"
	"testing"
	"time"

	agentmock "github.com/shivamstaq/github-symphony/internal/agent/mock"
	"github.com/shivamstaq/github-symphony/internal/domain"
)

func TestStallDetection_StalledWorkerNeedsHuman(t *testing.T) {
	item := makeItem("50", 50)
	// Use a slow mock that takes 5 seconds per turn
	mockAgent := &agentmock.MockAgent{
		StopReason: "completed",
		NumTurns:   1,
		HasCommits: true,
		Delay:      5 * time.Second,
	}
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)
	eng.cfg.Agent.StallTimeoutMs = 50 // 50ms stall timeout

	ctx := context.Background()
	eng.handlePollTick(ctx)

	// Wait for dispatch, then backdate LastActivityAt
	time.Sleep(20 * time.Millisecond)
	if entry, ok := eng.state.Running["50"]; ok {
		entry.LastActivityAt = time.Now().Add(-200 * time.Millisecond)
	}

	// Run stall detection
	eng.detectStalls()

	// Process stall event
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case evt := <-eng.eventCh:
			eng.handleEvent(ctx, evt)
		default:
		}
		if eng.state.ItemState("50") == domain.StateNeedsHuman {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if eng.state.ItemState("50") != domain.StateNeedsHuman {
		t.Errorf("expected needs_human after stall, got %q", eng.state.ItemState("50"))
	}

	// Worker should be removed from running
	if _, running := eng.state.Running["50"]; running {
		t.Error("stalled worker should be removed from running")
	}
}

func TestStallDetection_ActiveWorkerNotStalled(t *testing.T) {
	item := makeItem("51", 51)
	mockAgent := agentmock.NewSuccessAgent()
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)
	eng.cfg.Agent.StallTimeoutMs = 60000 // 60s timeout

	ctx := context.Background()
	eng.handlePollTick(ctx)

	// Worker just started — should not be stalled
	eng.detectStalls()

	select {
	case evt := <-eng.eventCh:
		if evt.Type == EvtStallDetected {
			t.Error("active worker should not trigger stall detection")
		}
		eng.handleEvent(ctx, evt)
	default:
		// Good — no stall event
	}
}

func TestStallDetection_DisabledWhenZero(t *testing.T) {
	item := makeItem("52", 52)
	mockAgent := agentmock.NewSuccessAgent()
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)
	eng.cfg.Agent.StallTimeoutMs = 0 // disabled

	ctx := context.Background()
	eng.handlePollTick(ctx)

	// Backdate activity
	if entry, ok := eng.state.Running["52"]; ok {
		entry.LastActivityAt = time.Now().Add(-time.Hour)
	}

	eng.detectStalls()

	select {
	case evt := <-eng.eventCh:
		if evt.Type == EvtStallDetected {
			t.Error("stall detection should be disabled when timeout=0")
		}
		eng.handleEvent(ctx, evt)
	default:
		// Good
	}
}
