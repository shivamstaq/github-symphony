package github_test

import (
	"strings"
	"testing"

	ghub "github.com/shivamstaq/github-symphony/internal/github"
)

func TestNormalizeWorkItem_WorkItemID(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_abc",
		IssueID:       "I_def",
		ContentType:   "issue",
	}

	item := ghub.NormalizeWorkItem(raw, nil)

	expected := "github:PVTI_abc:I_def"
	if item.WorkItemID != expected {
		t.Errorf("expected work_item_id=%q, got %q", expected, item.WorkItemID)
	}
}

func TestNormalizeWorkItem_WorkItemID_DraftIssue(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_abc",
		ContentType:   "draft_issue",
		// No IssueID for drafts
	}

	item := ghub.NormalizeWorkItem(raw, nil)

	expected := "github:PVTI_abc:draft_issue"
	if item.WorkItemID != expected {
		t.Errorf("expected work_item_id=%q, got %q", expected, item.WorkItemID)
	}
}

func TestNormalizeWorkItem_IssueIdentifier(t *testing.T) {
	num := 42
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1",
		IssueID:       "I_1",
		ContentType:   "issue",
		IssueNumber:   &num,
		Repository: &ghub.RepositoryInfo{
			Owner:    "myorg",
			Name:     "myrepo",
			FullName: "myorg/myrepo",
		},
	}

	item := ghub.NormalizeWorkItem(raw, nil)

	if item.IssueIdentifier != "myorg/myrepo#42" {
		t.Errorf("expected issue_identifier=myorg/myrepo#42, got %q", item.IssueIdentifier)
	}
}

func TestNormalizeWorkItem_LabelsLowercase(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1",
		ContentType:   "issue",
		Labels:        []string{"Bug", "HIGH-PRIORITY", "Enhancement"},
	}

	item := ghub.NormalizeWorkItem(raw, nil)

	for _, l := range item.Labels {
		if l != strings.ToLower(l) {
			t.Errorf("label not lowercase: %q", l)
		}
	}
	if item.Labels[0] != "bug" {
		t.Errorf("expected first label=bug, got %q", item.Labels[0])
	}
}

func TestNormalizeWorkItem_PriorityFromMap(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1",
		ContentType:   "issue",
		Priority:      "High",
	}

	priorityMap := map[string]int{
		"Critical": 1,
		"High":     2,
		"Medium":   3,
		"Low":      4,
	}

	item := ghub.NormalizeWorkItem(raw, priorityMap)

	if item.Priority == nil {
		t.Fatal("expected priority to be set")
	}
	if *item.Priority != 2 {
		t.Errorf("expected priority=2, got %d", *item.Priority)
	}
}

func TestNormalizeWorkItem_CloneURL_Derived(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1",
		ContentType:   "issue",
		Repository: &ghub.RepositoryInfo{
			Owner:    "org",
			Name:     "repo",
			FullName: "org/repo",
		},
	}

	item := ghub.NormalizeWorkItem(raw, nil)

	if item.Repository == nil {
		t.Fatal("expected repository")
	}
	if item.Repository.CloneURLHTTPS != "https://github.com/org/repo.git" {
		t.Errorf("expected derived clone URL, got %q", item.Repository.CloneURLHTTPS)
	}
}

func TestNormalizeWorkItem_BlockersAndSubIssues(t *testing.T) {
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_1",
		ContentType:   "issue",
		BlockedBy: []ghub.BlockerRefRaw{
			{ID: "I_blocker", Identifier: "org/repo#10", State: "open"},
		},
		SubIssues: []ghub.ChildRefRaw{
			{ID: "I_child", Identifier: "org/repo#11", State: "closed"},
		},
		LinkedPRs: []ghub.PRRefRaw{
			{ID: "PR_1", Number: 99, State: "open", IsDraft: true, URL: "https://github.com/org/repo/pull/99"},
		},
	}

	item := ghub.NormalizeWorkItem(raw, nil)

	if len(item.BlockedBy) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(item.BlockedBy))
	}
	if item.BlockedBy[0].Identifier != "org/repo#10" {
		t.Errorf("blocker identifier: %q", item.BlockedBy[0].Identifier)
	}

	if len(item.SubIssues) != 1 {
		t.Fatalf("expected 1 sub-issue, got %d", len(item.SubIssues))
	}

	if len(item.LinkedPRs) != 1 {
		t.Fatalf("expected 1 linked PR, got %d", len(item.LinkedPRs))
	}
	if item.LinkedPRs[0].Number != 99 {
		t.Errorf("linked PR number: %d", item.LinkedPRs[0].Number)
	}
}

func TestNormalizeWorkItem_FullFields(t *testing.T) {
	num := 5
	raw := ghub.WorkItemRaw{
		ProjectItemID: "PVTI_full",
		ContentType:   "issue",
		IssueID:       "I_full",
		IssueNumber:   &num,
		Title:         "Full test issue",
		Description:   "Detailed description",
		State:         "open",
		ProjectStatus: "In Progress",
		Labels:        []string{"Bug"},
		Assignees:     []string{"alice", "bob"},
		Milestone:     "v1.0",
		URL:           "https://github.com/org/repo/issues/5",
		CreatedAt:     "2024-01-01T00:00:00Z",
		UpdatedAt:     "2024-01-02T00:00:00Z",
		Repository: &ghub.RepositoryInfo{
			Owner:         "org",
			Name:          "repo",
			FullName:      "org/repo",
			DefaultBranch: "main",
		},
	}

	item := ghub.NormalizeWorkItem(raw, nil)

	if item.WorkItemID != "github:PVTI_full:I_full" {
		t.Errorf("work_item_id: %q", item.WorkItemID)
	}
	if item.IssueIdentifier != "org/repo#5" {
		t.Errorf("issue_identifier: %q", item.IssueIdentifier)
	}
	if item.Description != "Detailed description" {
		t.Errorf("description: %q", item.Description)
	}
	if len(item.Assignees) != 2 {
		t.Errorf("assignees: %v", item.Assignees)
	}
	if item.Milestone != "v1.0" {
		t.Errorf("milestone: %q", item.Milestone)
	}
	if item.Repository.DefaultBranch != "main" {
		t.Errorf("default branch: %q", item.Repository.DefaultBranch)
	}
}
