package orchestrator

import (
	"context"

	ghub "github.com/shivamstaq/github-symphony/internal/github"
)

// SourceBridge wraps github.Source to implement WorkItemSource.
type SourceBridge struct {
	source      *ghub.Source
	priorityMap map[string]int
}

// NewSourceBridge creates a bridge from github.Source to orchestrator.WorkItemSource.
func NewSourceBridge(source *ghub.Source, priorityMap map[string]int) *SourceBridge {
	return &SourceBridge{source: source, priorityMap: priorityMap}
}

func (b *SourceBridge) FetchCandidates(ctx context.Context) ([]WorkItem, error) {
	raw, err := b.source.FetchCandidateRaw(ctx)
	if err != nil {
		return nil, err
	}
	var items []WorkItem
	for _, r := range raw {
		n := ghub.NormalizeWorkItem(r, b.priorityMap)
		items = append(items, ConvertNormalizedItem(n))
	}
	return items, nil
}

func (b *SourceBridge) FetchStates(ctx context.Context, workItemIDs []string) ([]WorkItem, error) {
	raw, err := b.source.FetchStateRaw(ctx)
	if err != nil {
		return nil, err
	}

	idSet := make(map[string]bool, len(workItemIDs))
	for _, id := range workItemIDs {
		idSet[id] = true
	}

	var matched []WorkItem
	for _, r := range raw {
		n := ghub.NormalizeWorkItem(r, b.priorityMap)
		item := ConvertNormalizedItem(n)
		if idSet[item.WorkItemID] {
			matched = append(matched, item)
		}
	}
	return matched, nil
}
