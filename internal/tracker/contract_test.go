package tracker_test

import (
	"context"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/domain"
	"github.com/shivamstaq/github-symphony/internal/tracker"
	trackermock "github.com/shivamstaq/github-symphony/internal/tracker/mock"
)

// contractSuite runs shared behavioral tests against any Tracker implementation.
// These tests verify the interface contract, not implementation details.
func contractSuite(t *testing.T, name string, tr tracker.Tracker) {
	t.Run(name+"/FetchCandidates_returns_items", func(t *testing.T) {
		items, err := tr.FetchCandidates(context.Background())
		if err != nil {
			t.Fatalf("FetchCandidates error: %v", err)
		}
		if len(items) == 0 {
			t.Skip("no items to test — tracker may not be configured")
		}
		// Every item must have a WorkItemID
		for _, item := range items {
			if item.WorkItemID == "" {
				t.Error("FetchCandidates returned item with empty WorkItemID")
			}
		}
	})

	t.Run(name+"/FetchCandidates_normalized_fields", func(t *testing.T) {
		items, err := tr.FetchCandidates(context.Background())
		if err != nil {
			t.Fatalf("FetchCandidates error: %v", err)
		}
		for _, item := range items {
			// Every item must have a title
			if item.Title == "" {
				t.Errorf("item %s has empty title", item.WorkItemID)
			}
			// State must be open or closed
			if item.State != "" && item.State != "open" && item.State != "closed" {
				t.Errorf("item %s has unexpected state %q (want open/closed)", item.WorkItemID, item.State)
			}
			// ProjectStatus must not be empty
			if item.ProjectStatus == "" {
				t.Errorf("item %s has empty ProjectStatus", item.WorkItemID)
			}
		}
	})

	t.Run(name+"/FetchStates_returns_matching_items", func(t *testing.T) {
		items, err := tr.FetchCandidates(context.Background())
		if err != nil || len(items) == 0 {
			t.Skip("no items available")
		}
		ids := []string{items[0].WorkItemID}
		states, err := tr.FetchStates(context.Background(), ids)
		if err != nil {
			t.Fatalf("FetchStates error: %v", err)
		}
		if len(states) == 0 {
			t.Error("FetchStates returned no items for known ID")
		}
		if len(states) > 0 && states[0].WorkItemID != ids[0] {
			t.Errorf("FetchStates returned wrong item: got %q, want %q", states[0].WorkItemID, ids[0])
		}
	})

	t.Run(name+"/FetchStates_unknown_id_returns_empty", func(t *testing.T) {
		states, err := tr.FetchStates(context.Background(), []string{"nonexistent-id-xyz"})
		if err != nil {
			t.Fatalf("FetchStates error: %v", err)
		}
		if len(states) != 0 {
			t.Errorf("FetchStates for unknown ID should return empty, got %d items", len(states))
		}
	})

	t.Run(name+"/ValidateConfig_returns_no_error", func(t *testing.T) {
		_, err := tr.ValidateConfig(context.Background(), tracker.ValidationInput{
			ActiveValues:   []string{"Todo"},
			TerminalValues: []string{"Done"},
		})
		if err != nil {
			t.Fatalf("ValidateConfig error: %v", err)
		}
	})
}

// TestContract_MockTracker runs the contract suite against the mock tracker.
func TestContract_MockTracker(t *testing.T) {
	issueNum := 1
	items := []domain.WorkItem{
		{
			WorkItemID:    "test-1",
			ProjectItemID: "proj-1",
			ContentType:   "issue",
			IssueNumber:   &issueNum,
			Title:         "Test issue",
			State:         "open",
			ProjectStatus: "Todo",
		},
		{
			WorkItemID:    "test-2",
			ProjectItemID: "proj-2",
			ContentType:   "issue",
			IssueNumber:   &issueNum,
			Title:         "Another issue",
			State:         "closed",
			ProjectStatus: "Done",
		},
	}

	tr := trackermock.New(items)
	contractSuite(t, "mock", tr)
}

// TestContract_LinearTracker would run against a real Linear instance.
// Skipped by default — requires LINEAR_API_KEY and LINEAR_TEAM_ID env vars.
// func TestContract_LinearTracker(t *testing.T) {
//     apiKey := os.Getenv("LINEAR_API_KEY")
//     teamID := os.Getenv("LINEAR_TEAM_ID")
//     if apiKey == "" || teamID == "" {
//         t.Skip("set LINEAR_API_KEY and LINEAR_TEAM_ID for Linear contract tests")
//     }
//     tr := linear.NewSource(apiKey, teamID)
//     contractSuite(t, "linear", tr)
// }
