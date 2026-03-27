// Package scenario provides a test harness for driving the Symphony engine
// through scripted event sequences and asserting FSM state transitions.
package scenario

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
	"github.com/shivamstaq/github-symphony/internal/engine"
	trackermock "github.com/shivamstaq/github-symphony/internal/tracker/mock"
)

// Harness drives the engine through scenarios with mock adapters.
type Harness struct {
	t       *testing.T
	eng     *engine.Engine
	tracker *trackermock.Tracker
	ctx     context.Context
	cancel  context.CancelFunc
	tmpDir  string
}

// HarnessConfig configures a test harness.
type HarnessConfig struct {
	Items          []domain.WorkItem
	Agent          agent.Agent
	MaxConcurrent  int
	MaxRetries     int
	StallTimeoutMs int
	Budget         config.BudgetConfig
	ActiveValues   []string
	TerminalValues []string
	HandoffStatus  string
}

// NewHarness creates a test harness with the given configuration.
func NewHarness(t *testing.T, hcfg HarnessConfig) *Harness {
	t.Helper()
	tmpDir := t.TempDir()

	if hcfg.MaxConcurrent == 0 {
		hcfg.MaxConcurrent = 5
	}
	if hcfg.MaxRetries == 0 {
		hcfg.MaxRetries = 3
	}
	if len(hcfg.ActiveValues) == 0 {
		hcfg.ActiveValues = []string{"Todo", "In Progress"}
	}
	if len(hcfg.TerminalValues) == 0 {
		hcfg.TerminalValues = []string{"Done", "Closed"}
	}

	mockAgent := hcfg.Agent
	if mockAgent == nil {
		mockAgent = agentmock.NewSuccessAgent()
	}

	tracker := trackermock.New(hcfg.Items)

	evtLogPath := filepath.Join(tmpDir, "events.jsonl")
	evtLog, err := engine.NewEventLog(evtLogPath)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.SymphonyConfig{}
	cfg.Tracker.ActiveValues = hcfg.ActiveValues
	cfg.Tracker.TerminalValues = hcfg.TerminalValues
	cfg.Tracker.ExecutableItemTypes = []string{"issue"}
	cfg.Agent.MaxConcurrent = hcfg.MaxConcurrent
	cfg.Agent.MaxTurns = 20
	cfg.Agent.MaxContinuationRetries = hcfg.MaxRetries
	cfg.Agent.MaxRetryBackoffMs = 100 // fast retries for tests
	cfg.Agent.StallTimeoutMs = hcfg.StallTimeoutMs
	cfg.Agent.Budget = hcfg.Budget
	cfg.Polling.IntervalMs = 100
	cfg.PullRequest.HandoffStatus = hcfg.HandoffStatus

	eng := engine.New(engine.Deps{
		Config:   cfg,
		Tracker:  tracker,
		Agent:    mockAgent,
		EventLog: evtLog,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	ctx, cancel := context.WithCancel(context.Background())

	return &Harness{
		t:       t,
		eng:     eng,
		tracker: tracker,
		ctx:     ctx,
		cancel:  cancel,
		tmpDir:  tmpDir,
	}
}

// PollOnce triggers a single poll cycle and processes all resulting events.
func (h *Harness) PollOnce() {
	h.t.Helper()
	h.eng.HandlePollTick(h.ctx)
	h.DrainEvents(time.Second)
}

// DrainEvents processes events from the engine channel until timeout.
func (h *Harness) DrainEvents(timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !h.eng.ProcessOneEvent(h.ctx) {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// WaitForState waits until the given item reaches the expected state, or fails.
func (h *Harness) WaitForState(itemID string, expected domain.ItemState, timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h.eng.ProcessOneEvent(h.ctx)
		if h.State(itemID) == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for item %q to reach state %q (current: %q)",
		itemID, expected, h.State(itemID))
}

// State returns the current FSM state of an item.
func (h *Harness) State(itemID string) domain.ItemState {
	return h.eng.GetState().ItemState(itemID)
}

// IsHandedOff checks if an item is marked as handed off.
func (h *Harness) IsHandedOff(itemID string) bool {
	return h.eng.GetState().HandedOff[itemID]
}

// IsRunning checks if an item has a running worker.
func (h *Harness) IsRunning(itemID string) bool {
	_, ok := h.eng.GetState().Running[itemID]
	return ok
}

// HasRetry checks if an item is in the retry queue.
func (h *Harness) HasRetry(itemID string) bool {
	_, ok := h.eng.GetState().RetryQueue[itemID]
	return ok
}

// RetryAttempt returns the retry attempt count for an item.
func (h *Harness) RetryAttempt(itemID string) int {
	if re, ok := h.eng.GetState().RetryQueue[itemID]; ok {
		return re.Attempt
	}
	return 0
}

// DispatchTotal returns the total dispatch count.
func (h *Harness) DispatchTotal() int64 {
	return h.eng.GetState().DispatchTotal
}

// SetTrackerItems updates the mock tracker's items (simulate external changes).
func (h *Harness) SetTrackerItems(items []domain.WorkItem) {
	h.tracker.SetItems(items)
}

// Emit sends an event to the engine.
func (h *Harness) Emit(evt engine.EngineEvent) {
	h.eng.Emit(evt)
}

// RunningEntry returns the running entry for an item, or nil.
func (h *Harness) RunningEntry(itemID string) *engine.RunningEntry {
	return h.eng.GetState().Running[itemID]
}

// AssertState fails if the item is not in the expected state.
func (h *Harness) AssertState(itemID string, expected domain.ItemState) {
	h.t.Helper()
	got := h.State(itemID)
	if got != expected {
		h.t.Errorf("item %q: expected state %q, got %q", itemID, expected, got)
	}
}

// AssertNotDispatched fails if the dispatch total increased.
func (h *Harness) AssertNotDispatched(prevTotal int64) {
	h.t.Helper()
	if h.DispatchTotal() != prevTotal {
		h.t.Error("unexpected dispatch — item should not have been dispatched")
	}
}

// Cleanup cancels the context.
func (h *Harness) Cleanup() {
	h.cancel()
}
