// Package github implements tracker.Tracker for GitHub Projects V2.
package github

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shivamstaq/github-symphony/internal/domain"
	gh "github.com/shivamstaq/github-symphony/internal/github"
	"github.com/shivamstaq/github-symphony/internal/tracker"
)

// Source implements tracker.Tracker by wrapping github.Source.
type Source struct {
	ghSource    *gh.Source
	ghClient    *gh.GraphQLClient
	priorityMap map[string]int
	cfg         SourceConfig
}

// SourceConfig holds config needed by the GitHub tracker adapter.
type SourceConfig struct {
	Owner           string
	ProjectNumber   int
	ProjectScope    string
	StatusFieldName string
}

// NewSource creates a GitHub tracker adapter.
func NewSource(client *gh.GraphQLClient, ghSource *gh.Source, priorityMap map[string]int, cfg SourceConfig) *Source {
	return &Source{
		ghSource:    ghSource,
		ghClient:    client,
		priorityMap: priorityMap,
		cfg:         cfg,
	}
}

func (s *Source) FetchCandidates(ctx context.Context) ([]domain.WorkItem, error) {
	rawItems, err := s.ghSource.FetchCandidateRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("github tracker: fetch candidates: %w", err)
	}

	items := make([]domain.WorkItem, 0, len(rawItems))
	for _, raw := range rawItems {
		normalized := gh.NormalizeWorkItem(raw, s.priorityMap)
		items = append(items, ToDomainWorkItem(normalized))
	}

	slog.Debug("github tracker: fetched candidates", "count", len(items))
	return items, nil
}

func (s *Source) FetchStates(ctx context.Context, ids []string) ([]domain.WorkItem, error) {
	// Fetch all items and filter to matching IDs.
	// GitHub API doesn't support fetching by arbitrary IDs directly —
	// we refetch the project and filter.
	rawItems, err := s.ghSource.FetchStateRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("github tracker: fetch states: %w", err)
	}

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	var items []domain.WorkItem
	for _, raw := range rawItems {
		normalized := gh.NormalizeWorkItem(raw, s.priorityMap)
		domainItem := ToDomainWorkItem(normalized)
		if idSet[domainItem.WorkItemID] {
			items = append(items, domainItem)
		}
	}

	return items, nil
}

func (s *Source) ValidateConfig(ctx context.Context, input tracker.ValidationInput) ([]tracker.ValidationProblem, error) {
	// Fetch project field metadata to verify the status field exists
	fieldName := input.StatusFieldName
	if fieldName == "" {
		fieldName = "Status"
	}
	meta, err := s.ghClient.FetchProjectFieldMeta(ctx, s.cfg.Owner, s.cfg.ProjectNumber, s.cfg.ProjectScope, fieldName)
	if err != nil {
		return nil, fmt.Errorf("github tracker: validate config: %w", err)
	}

	var problems []tracker.ValidationProblem

	// Check that configured status values exist as options
	for _, v := range input.ActiveValues {
		if _, ok := meta.Options[v]; !ok {
			problems = append(problems, tracker.ValidationProblem{
				Kind:   tracker.ProblemMissingStatus,
				Name:   v,
				CanFix: false,
			})
		}
	}
	for _, v := range input.TerminalValues {
		if _, ok := meta.Options[v]; !ok {
			problems = append(problems, tracker.ValidationProblem{
				Kind:   tracker.ProblemMissingStatus,
				Name:   v,
				CanFix: false,
			})
		}
	}

	return problems, nil
}

func (s *Source) CreateMissingFields(_ context.Context, _ []tracker.ValidationProblem) error {
	return fmt.Errorf("github does not support auto-creating project field options — configure them in the GitHub Project settings UI")
}

// Compile-time interface check.
var _ tracker.Tracker = (*Source)(nil)
