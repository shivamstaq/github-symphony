// Package mock provides a configurable mock tracker for testing.
package mock

import (
	"context"
	"sync"

	"github.com/shivamstaq/github-symphony/internal/domain"
	"github.com/shivamstaq/github-symphony/internal/tracker"
)

// Tracker is a configurable mock that stores items in memory.
type Tracker struct {
	mu    sync.Mutex
	items []domain.WorkItem

	// Tracking calls for assertions
	FetchCandidatesCalls int
	FetchStatesCalls     int
	ValidateCalls        int
	StatusUpdates        []StatusUpdate
}

// StatusUpdate records a status change made via the tracker.
type StatusUpdate struct {
	ItemID    string
	NewStatus string
}

// New creates a mock tracker with the given initial items.
func New(items []domain.WorkItem) *Tracker {
	return &Tracker{items: items}
}

func (t *Tracker) FetchCandidates(_ context.Context) ([]domain.WorkItem, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.FetchCandidatesCalls++
	cp := make([]domain.WorkItem, len(t.items))
	copy(cp, t.items)
	return cp, nil
}

func (t *Tracker) FetchStates(_ context.Context, ids []string) ([]domain.WorkItem, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.FetchStatesCalls++

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	var result []domain.WorkItem
	for _, item := range t.items {
		if idSet[item.WorkItemID] {
			result = append(result, item)
		}
	}
	return result, nil
}

func (t *Tracker) ValidateConfig(_ context.Context, _ tracker.ValidationInput) ([]tracker.ValidationProblem, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ValidateCalls++
	return nil, nil
}

func (t *Tracker) CreateMissingFields(_ context.Context, _ []tracker.ValidationProblem) error {
	return nil
}

// SetItems replaces the item list (for simulating state changes).
func (t *Tracker) SetItems(items []domain.WorkItem) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items = items
}

// AddItem adds a single item.
func (t *Tracker) AddItem(item domain.WorkItem) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items = append(t.items, item)
}

// Compile-time check.
var _ tracker.Tracker = (*Tracker)(nil)
