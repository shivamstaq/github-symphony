package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shivamstaq/github-symphony/internal/domain"
)

// EligibilityConfig holds the config values needed for eligibility checks.
type EligibilityConfig struct {
	ActiveValues        []string
	TerminalValues      []string
	ExecutableItemTypes []string
	RequireIssueBacking bool
	RepoAllowlist       []string
	RepoDenylist        []string
	RequiredLabels      []string
	BlockedStatusValues []string
	MaxPerStatus        map[string]int
	MaxPerRepo          map[string]int
}

// IsEligible checks whether a work item should be dispatched.
// Returns (eligible, reason) where reason explains why it's ineligible.
func IsEligible(item domain.WorkItem, cfg EligibilityConfig, state *State, maxConcurrent int) (bool, string) {
	if item.ProjectItemID == "" {
		return false, "missing project_item_id"
	}
	if item.Title == "" {
		return false, "missing title"
	}
	if item.Pass2Failed {
		return false, "incomplete data (dependency fetch failed)"
	}
	if !containsCI(cfg.ExecutableItemTypes, item.ContentType) {
		return false, "content_type not executable: " + item.ContentType
	}
	if !containsCI(cfg.ActiveValues, item.ProjectStatus) {
		return false, "project_status not active: " + item.ProjectStatus
	}
	if containsCI(cfg.TerminalValues, item.ProjectStatus) {
		return false, "project_status is terminal: " + item.ProjectStatus
	}
	if containsCI(cfg.BlockedStatusValues, item.ProjectStatus) {
		return false, "project_status is blocked: " + item.ProjectStatus
	}
	if cfg.RequireIssueBacking && item.ContentType == "issue" && item.IssueNumber == nil {
		return false, "require_issue_backing: no issue number"
	}
	if item.State != "" && strings.ToLower(item.State) != "open" {
		return false, "issue state not open: " + item.State
	}

	// Repository allowlist/denylist
	if item.Repository != nil {
		fullName := item.Repository.FullName
		if len(cfg.RepoAllowlist) > 0 && !containsExact(cfg.RepoAllowlist, fullName) {
			return false, "repo not in allowlist: " + fullName
		}
		if containsExact(cfg.RepoDenylist, fullName) {
			return false, "repo in denylist: " + fullName
		}
	}

	// Required labels
	for _, req := range cfg.RequiredLabels {
		if !containsCI(item.Labels, req) {
			return false, "missing required label: " + req
		}
	}

	// FSM state checks (replaces old map checks)
	if state.IsClaimedOrRunning(item.WorkItemID) {
		return false, "already claimed or running"
	}
	if state.HandedOff[item.WorkItemID] {
		return false, "already handed off (PR created)"
	}

	// Global concurrency
	if state.RunningCount() >= maxConcurrent {
		return false, "no available slots"
	}

	// Per-status concurrency
	if limit, ok := cfg.MaxPerStatus[strings.ToLower(item.ProjectStatus)]; ok {
		count := 0
		for _, entry := range state.Running {
			if strings.EqualFold(entry.WorkItem.ProjectStatus, item.ProjectStatus) {
				count++
			}
		}
		if count >= limit {
			return false, fmt.Sprintf("per-status limit reached for %q (%d/%d)", item.ProjectStatus, count, limit)
		}
	}

	// Per-repo concurrency
	if item.Repository != nil {
		if limit, ok := cfg.MaxPerRepo[item.Repository.FullName]; ok {
			count := 0
			for _, entry := range state.Running {
				if entry.WorkItem.Repository != nil && entry.WorkItem.Repository.FullName == item.Repository.FullName {
					count++
				}
			}
			if count >= limit {
				return false, fmt.Sprintf("per-repo limit reached for %q (%d/%d)", item.Repository.FullName, count, limit)
			}
		}
	}

	// Blocker check
	for _, b := range item.BlockedBy {
		if b.State != "" && strings.ToLower(b.State) != "closed" {
			return false, "blocked by " + b.Identifier + " (state: " + b.State + ")"
		}
	}

	// Sub-issue check
	for _, child := range item.SubIssues {
		if child.State != "" && strings.ToLower(child.State) != "closed" {
			return false, "has open sub-issues: " + child.Identifier
		}
	}

	return true, ""
}

// SortForDispatch sorts work items by priority ascending, then created_at oldest first.
func SortForDispatch(items []domain.WorkItem) {
	sort.SliceStable(items, func(i, j int) bool {
		pi := priorityVal(items[i].Priority)
		pj := priorityVal(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt < items[j].CreatedAt
		}
		return items[i].WorkItemID < items[j].WorkItemID
	})
}

func priorityVal(p *int) int {
	if p == nil {
		return 999999
	}
	return *p
}

func containsCI(list []string, val string) bool {
	lower := strings.ToLower(val)
	for _, v := range list {
		if strings.ToLower(v) == lower {
			return true
		}
	}
	return false
}

func containsExact(list []string, val string) bool {
	for _, v := range list {
		if v == val {
			return true
		}
	}
	return false
}
