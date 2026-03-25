package orchestrator_test

import (
	"testing"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

func baseCfg() orchestrator.EligibilityConfig {
	return orchestrator.EligibilityConfig{
		ActiveValues:        []string{"Todo"},
		ExecutableItemTypes: []string{"issue"},
	}
}

func baseState() *orchestrator.State {
	return &orchestrator.State{
		Running: make(map[string]*orchestrator.RunningEntry),
		Claimed: make(map[string]bool),
	}
}

func baseItem(id string) orchestrator.WorkItem {
	num := 1
	return orchestrator.WorkItem{
		WorkItemID:    id,
		ProjectItemID: "p1",
		ContentType:   "issue",
		Title:         "Test",
		State:         "open",
		ProjectStatus: "Todo",
		IssueNumber:   &num,
		Repository:    &orchestrator.Repository{FullName: "org/repo"},
	}
}

// --- Blocking dependency tests ---

func TestIsEligible_BlockedByOpenDependency(t *testing.T) {
	item := baseItem("github:p1:i1")
	item.BlockedBy = []orchestrator.BlockerRef{
		{Identifier: "org/repo#10", State: "open"},
	}

	eligible, reason := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if eligible {
		t.Error("should be ineligible: blocked by open dependency")
	}
	if reason == "" {
		t.Error("expected reason")
	}
}

func TestIsEligible_BlockedByClosedDependency(t *testing.T) {
	item := baseItem("github:p1:i1")
	item.BlockedBy = []orchestrator.BlockerRef{
		{Identifier: "org/repo#10", State: "closed"},
	}

	eligible, _ := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if !eligible {
		t.Error("should be eligible: blocker is closed")
	}
}

func TestIsEligible_MultipleBlockers_OneOpen(t *testing.T) {
	item := baseItem("github:p1:i1")
	item.BlockedBy = []orchestrator.BlockerRef{
		{Identifier: "org/repo#10", State: "closed"},
		{Identifier: "org/repo#11", State: "open"},
	}

	eligible, _ := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if eligible {
		t.Error("should be ineligible: one blocker still open")
	}
}

func TestIsEligible_MultipleBlockers_AllClosed(t *testing.T) {
	item := baseItem("github:p1:i1")
	item.BlockedBy = []orchestrator.BlockerRef{
		{Identifier: "org/repo#10", State: "closed"},
		{Identifier: "org/repo#11", State: "closed"},
	}

	eligible, _ := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if !eligible {
		t.Error("should be eligible: all blockers closed")
	}
}

// --- Parent/sub-issue tests ---

func TestIsEligible_ParentWithOpenSubIssues(t *testing.T) {
	item := baseItem("github:p1:parent")
	item.SubIssues = []orchestrator.ChildRef{
		{Identifier: "org/repo#20", State: "open"},
		{Identifier: "org/repo#21", State: "closed"},
	}

	eligible, reason := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if eligible {
		t.Error("parent with open sub-issues should be ineligible")
	}
	if reason == "" {
		t.Error("expected reason mentioning sub-issues")
	}
}

func TestIsEligible_ParentWithAllClosedSubIssues(t *testing.T) {
	item := baseItem("github:p1:parent")
	item.SubIssues = []orchestrator.ChildRef{
		{Identifier: "org/repo#20", State: "closed"},
		{Identifier: "org/repo#21", State: "closed"},
	}

	eligible, _ := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if !eligible {
		t.Error("parent with all sub-issues closed should be eligible")
	}
}

func TestIsEligible_ParentWithNoSubIssues(t *testing.T) {
	item := baseItem("github:p1:parent")
	// No SubIssues — normal issue dispatch

	eligible, _ := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if !eligible {
		t.Error("item with no sub-issues should be eligible for normal dispatch")
	}
}

func TestIsEligible_SubIssueBlockedByExternal(t *testing.T) {
	// A sub-issue that itself has a blocking dependency
	item := baseItem("github:p1:sub")
	item.BlockedBy = []orchestrator.BlockerRef{
		{Identifier: "org/other-repo#99", State: "open"},
	}
	// No SubIssues of its own

	eligible, _ := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if eligible {
		t.Error("sub-issue blocked by external issue should be ineligible")
	}
}

func TestIsEligible_ParentBlockedAndHasSubIssues(t *testing.T) {
	// Parent is blocked AND has open sub-issues — blocked takes precedence
	item := baseItem("github:p1:parent")
	item.BlockedBy = []orchestrator.BlockerRef{
		{Identifier: "org/repo#5", State: "open"},
	}
	item.SubIssues = []orchestrator.ChildRef{
		{Identifier: "org/repo#20", State: "open"},
	}

	eligible, reason := orchestrator.IsEligible(item, baseCfg(), baseState(), 10)
	if eligible {
		t.Error("should be ineligible")
	}
	// Blocker check comes before sub-issue check, so reason should mention blocker
	if reason == "" {
		t.Error("expected reason")
	}
}
