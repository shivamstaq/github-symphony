package github

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

// SourceConfig holds parameters for the GitHub source adapter.
type SourceConfig struct {
	Owner            string
	ProjectNumber    int
	ProjectScope     string
	StatusFieldName  string
	PageSize         int
	PriorityValueMap map[string]int
}

// Source implements orchestrator.WorkItemSource by bridging GraphQL to the domain model.
type Source struct {
	client *GraphQLClient
	cfg    SourceConfig
}

// NewSource creates a GitHub source adapter.
func NewSource(client *GraphQLClient, cfg SourceConfig) *Source {
	return &Source{client: client, cfg: cfg}
}

// FetchCandidates fetches project items and normalizes them to WorkItems (two-pass).
func (s *Source) FetchCandidates(ctx context.Context) ([]orchestrator.WorkItem, error) {
	// Pass 1: lightweight fetch of project items with status
	rawItems, err := s.client.FetchProjectItems(ctx, ProjectQuery{
		Owner:           s.cfg.Owner,
		ProjectNumber:   s.cfg.ProjectNumber,
		ProjectScope:    s.cfg.ProjectScope,
		StatusFieldName: s.cfg.StatusFieldName,
		PageSize:        s.cfg.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("github source: fetch candidates: %w", err)
	}

	if len(rawItems) == 0 {
		return nil, nil
	}

	// Pass 2: enrich issues with full details
	rawItems, err = s.client.FetchIssueDetails(ctx, rawItems)
	if err != nil {
		slog.Warn("github source: pass 2 partial failure", "error", err)
		// Continue with partial data
	}

	// Normalize to domain model
	var items []orchestrator.WorkItem
	for _, raw := range rawItems {
		item := NormalizeWorkItem(raw, s.cfg.PriorityValueMap)
		items = append(items, item)
	}

	slog.Info("github source: fetched candidates", "count", len(items))
	return items, nil
}

// FetchStates fetches current state for specific work items (used by reconciliation).
func (s *Source) FetchStates(ctx context.Context, workItemIDs []string) ([]orchestrator.WorkItem, error) {
	// Re-fetch all candidates and filter to the requested IDs
	// A more efficient implementation would query by node IDs directly
	allItems, err := s.FetchCandidates(ctx)
	if err != nil {
		return nil, err
	}

	idSet := make(map[string]bool, len(workItemIDs))
	for _, id := range workItemIDs {
		idSet[id] = true
	}

	var matched []orchestrator.WorkItem
	for _, item := range allItems {
		if idSet[item.WorkItemID] {
			matched = append(matched, item)
		}
	}
	return matched, nil
}

// FetchTerminalItems fetches items in terminal project statuses (for startup cleanup).
func (s *Source) FetchTerminalItems(ctx context.Context, terminalValues []string) ([]orchestrator.WorkItem, error) {
	allItems, err := s.FetchCandidates(ctx)
	if err != nil {
		return nil, err
	}

	var terminal []orchestrator.WorkItem
	for _, item := range allItems {
		for _, tv := range terminalValues {
			if strings.EqualFold(item.ProjectStatus, tv) {
				terminal = append(terminal, item)
				break
			}
		}
	}
	return terminal, nil
}

// NormalizeWorkItem converts a raw GitHub item to the orchestrator domain model.
func NormalizeWorkItem(raw WorkItemRaw, priorityMap map[string]int) orchestrator.WorkItem {
	item := orchestrator.WorkItem{
		ProjectItemID: raw.ProjectItemID,
		ContentType:   raw.ContentType,
		IssueID:       raw.IssueID,
		IssueNumber:   raw.IssueNumber,
		Title:         raw.Title,
		Description:   raw.Description,
		State:         raw.State,
		ProjectStatus: raw.ProjectStatus,
		Labels:        raw.Labels,
		Assignees:     raw.Assignees,
		Milestone:     raw.Milestone,
		URL:           raw.URL,
		CreatedAt:     raw.CreatedAt,
		UpdatedAt:     raw.UpdatedAt,
	}

	// Derive work_item_id: github:<project_item_id>:<issue_id or content_type>
	if raw.IssueID != "" {
		item.WorkItemID = fmt.Sprintf("github:%s:%s", raw.ProjectItemID, raw.IssueID)
	} else {
		item.WorkItemID = fmt.Sprintf("github:%s:%s", raw.ProjectItemID, raw.ContentType)
	}

	// Derive issue_identifier: owner/repo#number
	if raw.Repository != nil && raw.IssueNumber != nil {
		item.IssueIdentifier = fmt.Sprintf("%s#%d", raw.Repository.FullName, *raw.IssueNumber)
	}

	// Normalize labels to lowercase
	for i, l := range item.Labels {
		item.Labels[i] = strings.ToLower(l)
	}

	// Derive priority from field value map
	if raw.Priority != "" && priorityMap != nil {
		if p, ok := priorityMap[raw.Priority]; ok {
			item.Priority = &p
		}
	}

	// Map repository
	if raw.Repository != nil {
		cloneURL := raw.Repository.CloneURLHTTPS
		if cloneURL == "" && raw.Repository.FullName != "" {
			cloneURL = fmt.Sprintf("https://github.com/%s.git", raw.Repository.FullName)
		}
		item.Repository = &orchestrator.Repository{
			Owner:         raw.Repository.Owner,
			Name:          raw.Repository.Name,
			FullName:      raw.Repository.FullName,
			DefaultBranch: raw.Repository.DefaultBranch,
			CloneURLHTTPS: cloneURL,
		}
	}

	// Map blockers
	for _, b := range raw.BlockedBy {
		item.BlockedBy = append(item.BlockedBy, orchestrator.BlockerRef{
			ID:         b.ID,
			Identifier: b.Identifier,
			State:      b.State,
		})
	}

	// Map sub-issues
	for _, s := range raw.SubIssues {
		item.SubIssues = append(item.SubIssues, orchestrator.ChildRef{
			ID:         s.ID,
			Identifier: s.Identifier,
			State:      s.State,
		})
	}

	// Map linked PRs
	for _, p := range raw.LinkedPRs {
		item.LinkedPRs = append(item.LinkedPRs, orchestrator.PRRef{
			ID:      p.ID,
			Number:  p.Number,
			State:   p.State,
			IsDraft: p.IsDraft,
			URL:     p.URL,
		})
	}

	return item
}
