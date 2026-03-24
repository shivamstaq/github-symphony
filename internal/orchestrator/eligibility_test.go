package orchestrator_test

import (
	"testing"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

func TestIsEligible_BasicActive(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:item1:issue1",
		ProjectItemID: "item1",
		ContentType:   "issue",
		Title:         "Fix bug",
		State:         "open",
		ProjectStatus: "Todo",
		IssueNumber:   intPtr(1),
		Repository:    &orchestrator.Repository{Owner: "org", Name: "repo", FullName: "org/repo"},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo", "Ready", "In Progress"},
		TerminalValues:      []string{"Done", "Closed"},
		ExecutableItemTypes: []string{"issue"},
		RequireIssueBacking: true,
	}

	state := &orchestrator.State{
		Running: make(map[string]*orchestrator.RunningEntry),
		Claimed: make(map[string]bool),
	}

	eligible, reason := orchestrator.IsEligible(item, cfg, state, 10)
	if !eligible {
		t.Fatalf("expected eligible, got ineligible: %s", reason)
	}
}

func TestIsEligible_TerminalStatus(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:item1:issue1",
		ProjectItemID: "item1",
		ContentType:   "issue",
		Title:         "Done task",
		State:         "open",
		ProjectStatus: "Done",
		IssueNumber:   intPtr(1),
		Repository:    &orchestrator.Repository{Owner: "org", Name: "repo", FullName: "org/repo"},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo"},
		TerminalValues:      []string{"Done"},
		ExecutableItemTypes: []string{"issue"},
	}
	state := &orchestrator.State{Running: make(map[string]*orchestrator.RunningEntry), Claimed: make(map[string]bool)}

	eligible, _ := orchestrator.IsEligible(item, cfg, state, 10)
	if eligible {
		t.Error("terminal status should be ineligible")
	}
}

func TestIsEligible_ClosedIssue(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:item1:issue1",
		ProjectItemID: "item1",
		ContentType:   "issue",
		Title:         "Closed",
		State:         "closed",
		ProjectStatus: "Todo",
		IssueNumber:   intPtr(1),
		Repository:    &orchestrator.Repository{Owner: "org", Name: "repo", FullName: "org/repo"},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo"},
		ExecutableItemTypes: []string{"issue"},
		RequireIssueBacking: true,
	}
	state := &orchestrator.State{Running: make(map[string]*orchestrator.RunningEntry), Claimed: make(map[string]bool)}

	eligible, _ := orchestrator.IsEligible(item, cfg, state, 10)
	if eligible {
		t.Error("closed issue should be ineligible")
	}
}

func TestIsEligible_AlreadyClaimed(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:item1:issue1",
		ProjectItemID: "item1",
		ContentType:   "issue",
		Title:         "Claimed",
		State:         "open",
		ProjectStatus: "Todo",
		IssueNumber:   intPtr(1),
		Repository:    &orchestrator.Repository{Owner: "org", Name: "repo", FullName: "org/repo"},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo"},
		ExecutableItemTypes: []string{"issue"},
	}
	state := &orchestrator.State{
		Running: make(map[string]*orchestrator.RunningEntry),
		Claimed: map[string]bool{"github:item1:issue1": true},
	}

	eligible, _ := orchestrator.IsEligible(item, cfg, state, 10)
	if eligible {
		t.Error("already claimed should be ineligible")
	}
}

func TestIsEligible_NoSlots(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:item1:issue1",
		ProjectItemID: "item1",
		ContentType:   "issue",
		Title:         "No slots",
		State:         "open",
		ProjectStatus: "Todo",
		IssueNumber:   intPtr(1),
		Repository:    &orchestrator.Repository{Owner: "org", Name: "repo", FullName: "org/repo"},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo"},
		ExecutableItemTypes: []string{"issue"},
	}
	state := &orchestrator.State{
		Running: map[string]*orchestrator.RunningEntry{
			"other": {},
		},
		Claimed: make(map[string]bool),
	}

	eligible, _ := orchestrator.IsEligible(item, cfg, state, 1) // max=1, 1 running
	if eligible {
		t.Error("no available slots should be ineligible")
	}
}

func TestIsEligible_BlockedByDependency(t *testing.T) {
	item := orchestrator.WorkItem{
		WorkItemID:    "github:item1:issue1",
		ProjectItemID: "item1",
		ContentType:   "issue",
		Title:         "Blocked",
		State:         "open",
		ProjectStatus: "Todo",
		IssueNumber:   intPtr(1),
		Repository:    &orchestrator.Repository{Owner: "org", Name: "repo", FullName: "org/repo"},
		BlockedBy: []orchestrator.BlockerRef{
			{Identifier: "org/repo#2", State: "open"},
		},
	}

	cfg := orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo"},
		ExecutableItemTypes: []string{"issue"},
	}
	state := &orchestrator.State{Running: make(map[string]*orchestrator.RunningEntry), Claimed: make(map[string]bool)}

	eligible, _ := orchestrator.IsEligible(item, cfg, state, 10)
	if eligible {
		t.Error("blocked issue should be ineligible")
	}
}

func TestSortForDispatch(t *testing.T) {
	items := []orchestrator.WorkItem{
		{WorkItemID: "c", Priority: intPtr(3), CreatedAt: "2024-01-03"},
		{WorkItemID: "a", Priority: intPtr(1), CreatedAt: "2024-01-01"},
		{WorkItemID: "b", Priority: intPtr(1), CreatedAt: "2024-01-02"},
	}

	orchestrator.SortForDispatch(items)

	if items[0].WorkItemID != "a" {
		t.Errorf("expected first=a (priority 1, oldest), got %s", items[0].WorkItemID)
	}
	if items[1].WorkItemID != "b" {
		t.Errorf("expected second=b (priority 1, newer), got %s", items[1].WorkItemID)
	}
	if items[2].WorkItemID != "c" {
		t.Errorf("expected third=c (priority 3), got %s", items[2].WorkItemID)
	}
}

func intPtr(i int) *int { return &i }
