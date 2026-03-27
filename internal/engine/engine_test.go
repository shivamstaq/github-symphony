package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/agent"
	agentmock "github.com/shivamstaq/github-symphony/internal/agent/mock"
	"github.com/shivamstaq/github-symphony/internal/config"
	"github.com/shivamstaq/github-symphony/internal/domain"
	"github.com/shivamstaq/github-symphony/internal/tracker"
)

// mockTracker implements tracker.Tracker for testing.
type mockTracker struct {
	items []domain.WorkItem
}

func (m *mockTracker) FetchCandidates(ctx context.Context) ([]domain.WorkItem, error) {
	return m.items, nil
}
func (m *mockTracker) FetchStates(ctx context.Context, ids []string) ([]domain.WorkItem, error) {
	// Return items that match the requested IDs
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var result []domain.WorkItem
	for _, item := range m.items {
		if idSet[item.WorkItemID] {
			result = append(result, item)
		}
	}
	return result, nil
}
func (m *mockTracker) ValidateConfig(ctx context.Context, input tracker.ValidationInput) ([]tracker.ValidationProblem, error) {
	return nil, nil
}
func (m *mockTracker) CreateMissingFields(ctx context.Context, problems []tracker.ValidationProblem) error {
	return nil
}

func newTestEngine(t *testing.T, items []domain.WorkItem, mockAgent agent.Agent) *Engine {
	t.Helper()
	tmpDir := t.TempDir()
	evtLogPath := filepath.Join(tmpDir, "events.jsonl")
	evtLog, err := NewEventLog(evtLogPath)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.SymphonyConfig{}
	cfg.Tracker.ActiveValues = []string{"Todo"}
	cfg.Tracker.TerminalValues = []string{"Done"}
	cfg.Tracker.ExecutableItemTypes = []string{"issue"}
	cfg.Agent.MaxConcurrent = 5
	cfg.Agent.MaxTurns = 10
	cfg.Agent.MaxContinuationRetries = 3
	cfg.Agent.MaxRetryBackoffMs = 1000
	cfg.Polling.IntervalMs = 100

	return New(Deps{
		Config:   cfg,
		Tracker:  &mockTracker{items: items},
		Agent:    mockAgent,
		EventLog: evtLog,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})
}

func makeItem(id string, issueNum int) domain.WorkItem {
	return domain.WorkItem{
		WorkItemID:      id,
		ProjectItemID:   "proj-" + id,
		ContentType:     "issue",
		IssueNumber:     &issueNum,
		IssueIdentifier: "org/repo#" + id,
		Title:           "Test issue " + id,
		Description:     "Fix something",
		State:           "open",
		ProjectStatus:   "Todo",
	}
}

func TestEngine_DispatchAndHandoff(t *testing.T) {
	item := makeItem("42", 42)
	mockAgent := agentmock.NewSuccessAgent()
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)

	// Run one poll tick
	ctx := context.Background()
	eng.handlePollTick(ctx)

	// Wait for worker goroutine to complete and events to be processed
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		// Process any pending events
		select {
		case evt := <-eng.eventCh:
			eng.handleEvent(ctx, evt)
		default:
		}

		if eng.state.HandedOff["42"] {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !eng.state.HandedOff["42"] {
		t.Fatal("expected item 42 to be handed off")
	}

	st := eng.state.ItemState("42")
	if st != domain.StateHandedOff {
		t.Errorf("expected state handed_off, got %q", st)
	}

	if eng.state.HandoffTotal != 1 {
		t.Errorf("expected handoff_total=1, got %d", eng.state.HandoffTotal)
	}
}

func TestEngine_NoCommitsNeedsHuman(t *testing.T) {
	item := makeItem("43", 43)
	mockAgent := agentmock.NewNoCommitsAgent()
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)

	ctx := context.Background()
	eng.handlePollTick(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case evt := <-eng.eventCh:
			eng.handleEvent(ctx, evt)
		default:
		}

		st := eng.state.ItemState("43")
		if st == domain.StateNeedsHuman {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	st := eng.state.ItemState("43")
	if st != domain.StateNeedsHuman {
		t.Errorf("expected state needs_human, got %q", st)
	}

	// Must NOT be retried
	if _, inRetry := eng.state.RetryQueue["43"]; inRetry {
		t.Error("no-commits exit should NOT schedule a retry")
	}
}

func TestEngine_ErrorWithRetry(t *testing.T) {
	item := makeItem("44", 44)
	mockAgent := agentmock.NewFailAgent(context.DeadlineExceeded)
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)

	ctx := context.Background()
	eng.handlePollTick(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case evt := <-eng.eventCh:
			eng.handleEvent(ctx, evt)
		default:
		}

		st := eng.state.ItemState("44")
		if st == domain.StateQueued {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	st := eng.state.ItemState("44")
	if st != domain.StateQueued {
		t.Errorf("expected state queued (retry), got %q", st)
	}

	re, ok := eng.state.RetryQueue["44"]
	if !ok {
		t.Fatal("expected retry entry for item 44")
	}
	if re.Attempt != 1 {
		t.Errorf("expected attempt=1, got %d", re.Attempt)
	}
}

func TestEngine_HandedOffNotReDispatched(t *testing.T) {
	item := makeItem("45", 45)
	mockAgent := agentmock.NewSuccessAgent()
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)

	ctx := context.Background()

	// First dispatch -> handoff
	eng.handlePollTick(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case evt := <-eng.eventCh:
			eng.handleEvent(ctx, evt)
		default:
		}
		if eng.state.HandedOff["45"] {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !eng.state.HandedOff["45"] {
		t.Fatal("expected item 45 to be handed off")
	}

	// Second poll — item should NOT be dispatched again
	prevDispatch := eng.state.DispatchTotal
	eng.handlePollTick(ctx)

	// Drain events
	for {
		select {
		case evt := <-eng.eventCh:
			eng.handleEvent(ctx, evt)
		default:
			goto done
		}
	}
done:

	if eng.state.DispatchTotal != prevDispatch {
		t.Error("handed-off item was re-dispatched — this is the critical cost leak bug")
	}
}

func TestEngine_EligibilityBlocksIneligible(t *testing.T) {
	// Item with terminal status should not be dispatched
	item := domain.WorkItem{
		WorkItemID:    "99",
		ProjectItemID: "proj-99",
		ContentType:   "issue",
		Title:         "Done issue",
		State:         "open",
		ProjectStatus: "Done",
	}
	mockAgent := agentmock.NewSuccessAgent()
	eng := newTestEngine(t, []domain.WorkItem{item}, mockAgent)

	ctx := context.Background()
	eng.handlePollTick(ctx)

	// Should not dispatch
	if eng.state.DispatchTotal != 0 {
		t.Error("terminal-status item should not be dispatched")
	}
}
