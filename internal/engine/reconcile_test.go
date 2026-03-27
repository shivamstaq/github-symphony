package engine

import (
	"testing"

	"github.com/shivamstaq/github-symphony/internal/domain"
)

func TestClassifyRefreshed_ClosedIssue(t *testing.T) {
	item := domain.WorkItem{State: "closed", ProjectStatus: "Todo"}
	action := ClassifyRefreshed(item, []string{"Todo"}, []string{"Done"})
	if action != ActionTerminate {
		t.Errorf("closed issue should terminate, got %q", action)
	}
}

func TestClassifyRefreshed_TerminalStatus(t *testing.T) {
	item := domain.WorkItem{State: "open", ProjectStatus: "Done"}
	action := ClassifyRefreshed(item, []string{"Todo"}, []string{"Done"})
	if action != ActionTerminate {
		t.Errorf("terminal status should terminate, got %q", action)
	}
}

func TestClassifyRefreshed_ActiveAndOpen(t *testing.T) {
	item := domain.WorkItem{State: "open", ProjectStatus: "Todo"}
	action := ClassifyRefreshed(item, []string{"Todo", "In Progress"}, []string{"Done"})
	if action != ActionKeep {
		t.Errorf("active + open should keep, got %q", action)
	}
}

func TestClassifyRefreshed_NonActiveStatus(t *testing.T) {
	item := domain.WorkItem{State: "open", ProjectStatus: "Backlog"}
	action := ClassifyRefreshed(item, []string{"Todo"}, []string{"Done"})
	if action != ActionStop {
		t.Errorf("non-active status should stop, got %q", action)
	}
}

func TestClassifyRefreshed_CaseInsensitive(t *testing.T) {
	item := domain.WorkItem{State: "OPEN", ProjectStatus: "todo"}
	action := ClassifyRefreshed(item, []string{"Todo"}, []string{"Done"})
	if action != ActionKeep {
		t.Errorf("case-insensitive match should keep, got %q", action)
	}
}
