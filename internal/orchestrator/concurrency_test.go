package orchestrator_test

import (
	"testing"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

func TestIsEligible_PerStatusLimit(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:new:issue",
		ProjectItemID: "new",
		ContentType:   "issue",
		Title:         "New item",
		State:         "open",
		ProjectStatus: "Todo",
		IssueNumber:   intPtr(5),
		Repository:    &orchestrator.Repository{FullName: "org/repo"},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo"},
		ExecutableItemTypes: []string{"issue"},
		MaxPerStatus:        map[string]int{"todo": 1}, // only 1 "Todo" at a time
	}

	state := &orchestrator.State{
		Running: map[string]*orchestrator.RunningEntry{
			"existing": {WorkItem: orchestrator.WorkItem{ProjectStatus: "Todo"}},
		},
		Claimed: make(map[string]bool),
	}

	eligible, reason := orchestrator.IsEligible(item, cfg, state, 10)
	if eligible {
		t.Error("should be ineligible: per-status limit for Todo is 1 and 1 already running")
	}
	if reason == "" {
		t.Error("expected a reason")
	}
}

func TestIsEligible_PerStatusLimit_DifferentStatus(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:new:issue",
		ProjectItemID: "new",
		ContentType:   "issue",
		Title:         "New item",
		State:         "open",
		ProjectStatus: "In Progress",
		IssueNumber:   intPtr(5),
		Repository:    &orchestrator.Repository{FullName: "org/repo"},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo", "In Progress"},
		ExecutableItemTypes: []string{"issue"},
		MaxPerStatus:        map[string]int{"todo": 1}, // limit only on Todo
	}

	state := &orchestrator.State{
		Running: map[string]*orchestrator.RunningEntry{
			"existing": {WorkItem: orchestrator.WorkItem{ProjectStatus: "Todo"}},
		},
		Claimed: make(map[string]bool),
	}

	eligible, _ := orchestrator.IsEligible(item, cfg, state, 10)
	if !eligible {
		t.Error("should be eligible: per-status limit only applies to Todo, this is In Progress")
	}
}

func TestIsEligible_PerRepoLimit(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:new:issue",
		ProjectItemID: "new",
		ContentType:   "issue",
		Title:         "New item",
		State:         "open",
		ProjectStatus: "Todo",
		IssueNumber:   intPtr(5),
		Repository:    &orchestrator.Repository{FullName: "org/busy-repo"},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo"},
		ExecutableItemTypes: []string{"issue"},
		MaxPerRepo:          map[string]int{"org/busy-repo": 1},
	}

	state := &orchestrator.State{
		Running: map[string]*orchestrator.RunningEntry{
			"existing": {WorkItem: orchestrator.WorkItem{
				Repository: &orchestrator.Repository{FullName: "org/busy-repo"},
			}},
		},
		Claimed: make(map[string]bool),
	}

	eligible, reason := orchestrator.IsEligible(item, cfg, state, 10)
	if eligible {
		t.Error("should be ineligible: per-repo limit for org/busy-repo is 1")
	}
	if reason == "" {
		t.Error("expected a reason")
	}
}
