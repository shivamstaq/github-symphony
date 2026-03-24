package github_test

import (
	"strings"
	"testing"

	ghub "github.com/shivamstaq/github-symphony/internal/github"
)

func TestNormalizeWorkItem_WorkItemID(t *testing.T) {
	raw := ghub.WorkItemRaw{ProjectItemID: "PVTI_abc", IssueID: "I_def", ContentType: "issue"}
	item := ghub.NormalizeWorkItem(raw, nil)
	if item.WorkItemID != "github:PVTI_abc:I_def" {
		t.Errorf("got %q", item.WorkItemID)
	}
}

func TestNormalizeWorkItem_WorkItemID_DraftIssue(t *testing.T) {
	raw := ghub.WorkItemRaw{ProjectItemID: "PVTI_abc", ContentType: "draft_issue"}
	item := ghub.NormalizeWorkItem(raw, nil)
	if item.WorkItemID != "github:PVTI_abc:draft_issue" {
		t.Errorf("got %q", item.WorkItemID)
	}
}

func TestNormalizeWorkItem_IssueIdentifier(t *testing.T) {
	num := 42
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1", IssueID: "I_1", ContentType: "issue",
		IssueNumber: &num,
		Repository:  &ghub.RepositoryInfo{Owner: "myorg", Name: "myrepo", FullName: "myorg/myrepo"},
	}
	item := ghub.NormalizeWorkItem(raw, nil)
	if item.IssueIdentifier != "myorg/myrepo#42" {
		t.Errorf("got %q", item.IssueIdentifier)
	}
}

func TestNormalizeWorkItem_LabelsLowercase(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1", ContentType: "issue",
		Labels: []string{"Bug", "HIGH-PRIORITY", "Enhancement"},
	}
	item := ghub.NormalizeWorkItem(raw, nil)
	for _, l := range item.Labels {
		if l != strings.ToLower(l) {
			t.Errorf("label not lowercase: %q", l)
		}
	}
}

func TestNormalizeWorkItem_PriorityFromMap(t *testing.T) {
	raw := ghub.WorkItemRaw{ProjectItemID: "PVTI_1", ContentType: "issue", Priority: "High"}
	item := ghub.NormalizeWorkItem(raw, map[string]int{"Critical": 1, "High": 2, "Medium": 3, "Low": 4})
	if item.Priority == nil || *item.Priority != 2 {
		t.Errorf("expected priority=2, got %v", item.Priority)
	}
}

func TestNormalizeWorkItem_CloneURL_Derived(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1", ContentType: "issue",
		Repository: &ghub.RepositoryInfo{Owner: "org", Name: "repo", FullName: "org/repo"},
	}
	item := ghub.NormalizeWorkItem(raw, nil)
	if item.Repository == nil || item.Repository.CloneURLHTTPS != "https://github.com/org/repo.git" {
		t.Errorf("clone URL: %v", item.Repository)
	}
}

func TestNormalizeWorkItem_BlockersAndSubIssues(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1", ContentType: "issue",
		BlockedBy: []ghub.BlockerRefRaw{{ID: "I_b", Identifier: "org/repo#10", State: "open"}},
		SubIssues: []ghub.ChildRefRaw{{ID: "I_c", Identifier: "org/repo#11", State: "closed"}},
		LinkedPRs: []ghub.PRRefRaw{{ID: "PR_1", Number: 99, State: "open", IsDraft: true, URL: "https://github.com/org/repo/pull/99"}},
	}
	item := ghub.NormalizeWorkItem(raw, nil)
	if len(item.BlockedBy) != 1 || item.BlockedBy[0].Identifier != "org/repo#10" {
		t.Errorf("blockers: %v", item.BlockedBy)
	}
	if len(item.SubIssues) != 1 {
		t.Errorf("sub-issues: %v", item.SubIssues)
	}
	if len(item.LinkedPRs) != 1 || item.LinkedPRs[0].Number != 99 {
		t.Errorf("linked PRs: %v", item.LinkedPRs)
	}
}

func TestNormalizeWorkItem_FullFields(t *testing.T) {
	num := 5
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_full", ContentType: "issue", IssueID: "I_full",
		IssueNumber: &num, Title: "Full test issue", Description: "Detailed",
		State: "open", ProjectStatus: "In Progress",
		Labels: []string{"Bug"}, Assignees: []string{"alice", "bob"}, Milestone: "v1.0",
		URL: "https://github.com/org/repo/issues/5", CreatedAt: "2024-01-01T00:00:00Z",
		Repository: &ghub.RepositoryInfo{Owner: "org", Name: "repo", FullName: "org/repo", DefaultBranch: "main"},
	}
	item := ghub.NormalizeWorkItem(raw, nil)
	if item.WorkItemID != "github:PVTI_full:I_full" {
		t.Errorf("work_item_id: %q", item.WorkItemID)
	}
	if item.IssueIdentifier != "org/repo#5" {
		t.Errorf("issue_identifier: %q", item.IssueIdentifier)
	}
	if item.Description != "Detailed" {
		t.Errorf("description: %q", item.Description)
	}
	if len(item.Assignees) != 2 {
		t.Errorf("assignees: %v", item.Assignees)
	}
	if item.Repository.DefaultBranch != "main" {
		t.Errorf("default branch: %q", item.Repository.DefaultBranch)
	}
}
