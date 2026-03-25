package github

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

// Source fetches project items from GitHub and normalizes them.
type Source struct {
	client *GraphQLClient
	cfg    SourceConfig
}

// NewSource creates a GitHub source adapter.
func NewSource(client *GraphQLClient, cfg SourceConfig) *Source {
	return &Source{client: client, cfg: cfg}
}

// FetchCandidateRaw fetches project items with two-pass enrichment, returns raw items.
func (s *Source) FetchCandidateRaw(ctx context.Context) ([]WorkItemRaw, error) {
	// Pass 1: lightweight fetch
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

	// Pass 2: enrich issues
	rawItems, err = s.client.FetchIssueDetails(ctx, rawItems)
	if err != nil {
		slog.Warn("github source: pass 2 partial failure", "error", err)
	}

	slog.Info("github source: fetched candidates", "count", len(rawItems))
	return rawItems, nil
}

// FetchStateRaw fetches current state for specific issue IDs.
func (s *Source) FetchStateRaw(ctx context.Context) ([]WorkItemRaw, error) {
	return s.FetchCandidateRaw(ctx)
}

// FetchTerminalRaw fetches items in terminal statuses.
func (s *Source) FetchTerminalRaw(ctx context.Context, terminalValues []string) ([]WorkItemRaw, error) {
	all, err := s.FetchCandidateRaw(ctx)
	if err != nil {
		return nil, err
	}
	var terminal []WorkItemRaw
	for _, item := range all {
		for _, tv := range terminalValues {
			if strings.EqualFold(item.ProjectStatus, tv) {
				terminal = append(terminal, item)
				break
			}
		}
	}
	return terminal, nil
}

// NormalizeWorkItem converts a raw GitHub item to a NormalizedItem with all derived fields.
func NormalizeWorkItem(raw WorkItemRaw, priorityMap map[string]int) NormalizedItem {
	item := NormalizedItem{
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
		Pass2Failed:   raw.Pass2Failed,
	}

	// Derive work_item_id
	if raw.IssueID != "" {
		item.WorkItemID = fmt.Sprintf("github:%s:%s", raw.ProjectItemID, raw.IssueID)
	} else {
		item.WorkItemID = fmt.Sprintf("github:%s:%s", raw.ProjectItemID, raw.ContentType)
	}

	// Derive issue_identifier
	if raw.Repository != nil && raw.IssueNumber != nil {
		item.IssueIdentifier = fmt.Sprintf("%s#%d", raw.Repository.FullName, *raw.IssueNumber)
	}

	// Lowercase labels
	for i, l := range item.Labels {
		item.Labels[i] = strings.ToLower(l)
	}

	// Priority from value map
	if raw.Priority != "" && priorityMap != nil {
		if p, ok := priorityMap[raw.Priority]; ok {
			item.Priority = &p
		}
	}

	// Repository with derived clone URL
	if raw.Repository != nil {
		cloneURL := raw.Repository.CloneURLHTTPS
		if cloneURL == "" && raw.Repository.FullName != "" {
			cloneURL = fmt.Sprintf("https://github.com/%s.git", raw.Repository.FullName)
		}
		item.Repository = &NormalizedRepo{
			Owner:         raw.Repository.Owner,
			Name:          raw.Repository.Name,
			FullName:      raw.Repository.FullName,
			DefaultBranch: raw.Repository.DefaultBranch,
			CloneURLHTTPS: cloneURL,
		}
	}

	// Blockers
	for _, b := range raw.BlockedBy {
		item.BlockedBy = append(item.BlockedBy, NormalizedBlocker(b))
	}
	if len(raw.BlockedBy) > 0 {
		slog.Debug("normalized blockers", "work_item_id", item.WorkItemID, "blocked_by_count", len(raw.BlockedBy))
	}
	for _, s := range raw.SubIssues {
		item.SubIssues = append(item.SubIssues, NormalizedChild(s))
	}
	for _, p := range raw.LinkedPRs {
		item.LinkedPRs = append(item.LinkedPRs, NormalizedPR(p))
	}

	return item
}

// NormalizedItem is the fully-normalized work item (github-package local, no import cycle).
type NormalizedItem struct {
	WorkItemID      string
	ProjectItemID   string
	ContentType     string
	IssueID         string
	IssueNumber     *int
	IssueIdentifier string
	Title           string
	Description     string
	State           string
	ProjectStatus   string
	Priority        *int
	Labels          []string
	Assignees       []string
	Milestone       string
	URL             string
	CreatedAt       string
	UpdatedAt       string
	Pass2Failed     bool
	Repository      *NormalizedRepo
	BlockedBy       []NormalizedBlocker
	SubIssues       []NormalizedChild
	LinkedPRs       []NormalizedPR
}

type NormalizedRepo struct {
	Owner, Name, FullName, DefaultBranch, CloneURLHTTPS string
}

type NormalizedBlocker struct {
	ID, Identifier, State string
}

type NormalizedChild struct {
	ID, Identifier, State string
}

type NormalizedPR struct {
	ID      string
	Number  int
	State   string
	IsDraft bool
	URL     string
}
