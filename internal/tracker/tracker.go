// Package tracker defines the abstract interface for work item tracking systems.
//
// Symphony supports multiple tracker backends (GitHub Projects, Linear) through
// this interface. Each backend implements Tracker and converts its native format
// to the shared domain.WorkItem model.
//
// Currently implemented: GitHub Projects (tracker/github/)
// Planned: Linear (tracker/linear/)
package tracker

import (
	"context"

	"github.com/shivamstaq/github-symphony/internal/domain"
)

// Tracker is the unified interface for issue/project tracking systems.
// Implementations: tracker/github, tracker/linear
type Tracker interface {
	// FetchCandidates returns all work items eligible for dispatch.
	FetchCandidates(ctx context.Context) ([]domain.WorkItem, error)

	// FetchStates returns current state for specific work item IDs.
	FetchStates(ctx context.Context, ids []string) ([]domain.WorkItem, error)

	// ValidateConfig checks that configured fields, statuses, and labels
	// exist on the remote tracker. Returns a list of problems found.
	ValidateConfig(ctx context.Context, input ValidationInput) ([]ValidationProblem, error)

	// CreateMissingFields creates fields/statuses/labels that don't exist.
	CreateMissingFields(ctx context.Context, problems []ValidationProblem) error
}

// ValidationInput contains the fields to validate against the remote tracker.
type ValidationInput struct {
	StatusFieldName string
	ActiveValues    []string
	TerminalValues  []string
	RequiredLabels  []string
	CustomFields    []string
}

// ValidationProblem describes a single config/tracker mismatch.
type ValidationProblem struct {
	Kind   ProblemKind
	Name   string
	CanFix bool
}

// ProblemKind classifies a validation problem.
type ProblemKind string

const (
	ProblemMissingStatus ProblemKind = "missing_status"
	ProblemMissingLabel  ProblemKind = "missing_label"
	ProblemMissingField  ProblemKind = "missing_field"
)
