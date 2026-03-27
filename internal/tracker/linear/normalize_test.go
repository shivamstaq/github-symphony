package linear

import (
	"testing"
)

func TestNormalize_BasicIssue(t *testing.T) {
	issue := IssueNode{
		ID:          "abc-123",
		Identifier:  "ENG-42",
		Title:       "Fix login bug",
		Description: "Login fails on mobile",
		Priority:    2,
		State:       StateNode{Name: "In Progress", Type: "started"},
		Labels:      LabelsConn{Nodes: []LabelNode{{Name: "bug"}, {Name: "urgent"}}},
		Assignee:    &AssigneeNode{Name: "Alice"},
		CreatedAt:   "2026-03-15T10:00:00Z",
		URL:         "https://linear.app/team/ENG-42",
		Team:        TeamNode{Key: "ENG", Name: "Engineering"},
	}

	item := Normalize(issue)

	if item.WorkItemID != "linear:abc-123" {
		t.Errorf("WorkItemID = %q, want 'linear:abc-123'", item.WorkItemID)
	}
	if item.IssueIdentifier != "ENG-42" {
		t.Errorf("IssueIdentifier = %q, want 'ENG-42'", item.IssueIdentifier)
	}
	if item.Title != "Fix login bug" {
		t.Errorf("Title = %q", item.Title)
	}
	if item.State != "open" {
		t.Errorf("State = %q, want 'open' for started type", item.State)
	}
	if item.ProjectStatus != "In Progress" {
		t.Errorf("ProjectStatus = %q", item.ProjectStatus)
	}
	if item.Priority == nil || *item.Priority != 2 {
		t.Errorf("Priority = %v, want 2", item.Priority)
	}
	if len(item.Labels) != 2 || item.Labels[0] != "bug" {
		t.Errorf("Labels = %v", item.Labels)
	}
	if len(item.Assignees) != 1 || item.Assignees[0] != "Alice" {
		t.Errorf("Assignees = %v", item.Assignees)
	}
}

func TestNormalize_CompletedIssueIsClosed(t *testing.T) {
	issue := IssueNode{
		ID:         "def-456",
		Identifier: "ENG-99",
		Title:      "Done issue",
		State:      StateNode{Name: "Done", Type: "completed"},
		Team:       TeamNode{Key: "ENG"},
	}

	item := Normalize(issue)
	if item.State != "closed" {
		t.Errorf("completed issue State = %q, want 'closed'", item.State)
	}
}

func TestNormalize_CancelledIssueIsClosed(t *testing.T) {
	issue := IssueNode{
		ID:         "ghi-789",
		Identifier: "ENG-100",
		Title:      "Cancelled",
		State:      StateNode{Name: "Cancelled", Type: "cancelled"},
		Team:       TeamNode{Key: "ENG"},
	}

	item := Normalize(issue)
	if item.State != "closed" {
		t.Errorf("cancelled issue State = %q, want 'closed'", item.State)
	}
}

func TestNormalize_BlockedByRelation(t *testing.T) {
	issue := IssueNode{
		ID:         "xyz-1",
		Identifier: "ENG-10",
		Title:      "Blocked task",
		State:      StateNode{Name: "Todo", Type: "unstarted"},
		Team:       TeamNode{Key: "ENG"},
		Relations: RelationsConn{
			Nodes: []RelationNode{
				{
					Type: "blocked_by",
					RelatedIssue: IssueRef{
						ID:         "xyz-2",
						Identifier: "ENG-5",
						State:      StateNode{Name: "In Progress"},
					},
				},
			},
		},
	}

	item := Normalize(issue)
	if len(item.BlockedBy) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(item.BlockedBy))
	}
	if item.BlockedBy[0].Identifier != "ENG-5" {
		t.Errorf("blocker identifier = %q", item.BlockedBy[0].Identifier)
	}
}

func TestNormalize_NoPriorityIsNil(t *testing.T) {
	issue := IssueNode{
		ID:         "nop-1",
		Identifier: "ENG-200",
		Title:      "No priority",
		Priority:   0,
		State:      StateNode{Name: "Todo", Type: "unstarted"},
		Team:       TeamNode{Key: "ENG"},
	}

	item := Normalize(issue)
	if item.Priority != nil {
		t.Errorf("zero priority should normalize to nil, got %v", *item.Priority)
	}
}
