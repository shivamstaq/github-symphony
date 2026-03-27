package engine

import (
	"testing"

	"github.com/shivamstaq/github-symphony/internal/domain"
)

func makeEligItem(id string, status string, repo *domain.Repository) domain.WorkItem {
	num := 1
	return domain.WorkItem{
		WorkItemID:    id,
		ProjectItemID: "proj-" + id,
		ContentType:   "issue",
		IssueNumber:   &num,
		Title:         "Issue " + id,
		State:         "open",
		ProjectStatus: status,
		Repository:    repo,
	}
}

func baseEligCfg() EligibilityConfig {
	return EligibilityConfig{
		ActiveValues:        []string{"Todo", "In Progress"},
		TerminalValues:      []string{"Done"},
		ExecutableItemTypes: []string{"issue"},
	}
}

func TestIsEligible_PerStatusConcurrency(t *testing.T) {
	cfg := baseEligCfg()
	cfg.MaxPerStatus = map[string]int{"todo": 1}

	state := NewState()
	state.Running["existing"] = &RunningEntry{
		WorkItem: makeEligItem("existing", "Todo", nil),
	}

	item := makeEligItem("new", "Todo", nil)
	eligible, reason := IsEligible(item, cfg, state, 10)

	if eligible {
		t.Errorf("should be ineligible due to per-status limit, got eligible")
	}
	if reason == "" {
		t.Error("expected a reason string for ineligibility")
	}
}

func TestIsEligible_PerStatusDifferentStatusAllowed(t *testing.T) {
	cfg := baseEligCfg()
	cfg.MaxPerStatus = map[string]int{"todo": 1}

	state := NewState()
	state.Running["existing"] = &RunningEntry{
		WorkItem: makeEligItem("existing", "Todo", nil),
	}

	item := makeEligItem("new2", "In Progress", nil)
	eligible, _ := IsEligible(item, cfg, state, 10)
	if !eligible {
		t.Error("item with different status should be eligible")
	}
}

func TestIsEligible_PerRepoConcurrency(t *testing.T) {
	repo := &domain.Repository{Owner: "org", Name: "repo", FullName: "org/repo"}
	cfg := baseEligCfg()
	cfg.MaxPerRepo = map[string]int{"org/repo": 1}

	state := NewState()
	state.Running["existing"] = &RunningEntry{
		WorkItem: makeEligItem("existing", "Todo", repo),
	}

	item := makeEligItem("new", "Todo", repo)
	eligible, reason := IsEligible(item, cfg, state, 10)

	if eligible {
		t.Errorf("should be ineligible due to per-repo limit, got eligible")
	}
	if reason == "" {
		t.Error("expected a reason string")
	}
}

func TestIsEligible_PerRepoDifferentRepoAllowed(t *testing.T) {
	repo := &domain.Repository{Owner: "org", Name: "repo", FullName: "org/repo"}
	otherRepo := &domain.Repository{Owner: "org", Name: "other", FullName: "org/other"}
	cfg := baseEligCfg()
	cfg.MaxPerRepo = map[string]int{"org/repo": 1}

	state := NewState()
	state.Running["existing"] = &RunningEntry{
		WorkItem: makeEligItem("existing", "Todo", repo),
	}

	item := makeEligItem("new2", "Todo", otherRepo)
	eligible, _ := IsEligible(item, cfg, state, 10)
	if !eligible {
		t.Error("item in different repo should be eligible")
	}
}

func TestIsEligible_PerStatusNotConfigured(t *testing.T) {
	cfg := baseEligCfg()
	// No MaxPerStatus set

	state := NewState()
	state.Running["existing"] = &RunningEntry{
		WorkItem: makeEligItem("existing", "Todo", nil),
	}

	item := makeEligItem("new", "Todo", nil)
	eligible, _ := IsEligible(item, cfg, state, 10)
	if !eligible {
		t.Error("should be eligible when per-status limit is not configured")
	}
}

func TestIsEligible_PerRepoNotConfigured(t *testing.T) {
	repo := &domain.Repository{Owner: "org", Name: "repo", FullName: "org/repo"}
	cfg := baseEligCfg()
	// No MaxPerRepo set

	state := NewState()
	state.Running["existing"] = &RunningEntry{
		WorkItem: makeEligItem("existing", "Todo", repo),
	}

	item := makeEligItem("new", "Todo", repo)
	eligible, _ := IsEligible(item, cfg, state, 10)
	if !eligible {
		t.Error("should be eligible when per-repo limit is not configured")
	}
}
