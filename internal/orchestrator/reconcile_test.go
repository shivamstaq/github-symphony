package orchestrator_test

import (
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

func TestReconcileStalled_KillsStalled(t *testing.T) {
	now := time.Now()
	state := &orchestrator.State{
		Running: map[string]*orchestrator.RunningEntry{
			"stalled": {
				WorkItem:  orchestrator.WorkItem{WorkItemID: "stalled"},
				StartedAt: now.Add(-10 * time.Minute), // started 10 min ago
				// No LastAgentTimestamp — never heard from agent
			},
			"fresh": {
				WorkItem:          orchestrator.WorkItem{WorkItemID: "fresh"},
				StartedAt:         now.Add(-1 * time.Minute),
				LastAgentTimestamp: timePtr(now.Add(-30 * time.Second)), // heard 30s ago
			},
		},
		Claimed:       map[string]bool{"stalled": true, "fresh": true},
		RetryAttempts: make(map[string]*orchestrator.RetryEntry),
	}

	stalled := orchestrator.DetectStalled(state, 300000) // 5 min stall timeout

	if len(stalled) != 1 {
		t.Fatalf("expected 1 stalled, got %d", len(stalled))
	}
	if stalled[0] != "stalled" {
		t.Errorf("expected 'stalled', got %q", stalled[0])
	}
}

func TestReconcileStalled_DisabledWhenZero(t *testing.T) {
	state := &orchestrator.State{
		Running: map[string]*orchestrator.RunningEntry{
			"old": {
				WorkItem:  orchestrator.WorkItem{WorkItemID: "old"},
				StartedAt: time.Now().Add(-1 * time.Hour),
			},
		},
	}

	stalled := orchestrator.DetectStalled(state, 0) // disabled
	if len(stalled) != 0 {
		t.Errorf("stall detection disabled but got %d stalled", len(stalled))
	}
}

func TestClassifyRefreshedItem_Terminal(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "i1",
		ProjectStatus: "Done",
		State:         "closed",
	}

	action := orchestrator.ClassifyRefreshed(item, []string{"Todo", "In Progress"}, []string{"Done", "Closed"})
	if action != orchestrator.ActionTerminate {
		t.Errorf("expected ActionTerminate, got %v", action)
	}
}

func TestClassifyRefreshedItem_StillActive(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "i1",
		ProjectStatus: "In Progress",
		State:         "open",
	}

	action := orchestrator.ClassifyRefreshed(item, []string{"Todo", "In Progress"}, []string{"Done"})
	if action != orchestrator.ActionKeep {
		t.Errorf("expected ActionKeep, got %v", action)
	}
}

func TestClassifyRefreshedItem_NeitherActiveNorTerminal(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "i1",
		ProjectStatus: "Blocked",
		State:         "open",
	}

	action := orchestrator.ClassifyRefreshed(item, []string{"Todo", "In Progress"}, []string{"Done"})
	if action != orchestrator.ActionStop {
		t.Errorf("expected ActionStop (non-active, non-terminal), got %v", action)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
