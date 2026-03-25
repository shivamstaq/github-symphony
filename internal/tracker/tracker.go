// Package tracker defines the abstract interfaces for work item tracking systems.
//
// Symphony supports multiple tracker backends (GitHub Projects, Linear, Jira, GitLab)
// through these interfaces. Each backend implements the three core contracts:
//
//   - WorkItemSource: fetch eligible work items and refresh their states
//   - WriteBackService: create PRs, post comments, update status fields
//   - TrackerAuth: provide authentication tokens for the tracker
//
// The orchestrator only interacts with these interfaces, never with
// tracker-specific types. Each backend converts its native format to the
// shared WorkItem domain model.
//
// Currently implemented: GitHub Projects (internal/github/)
// Planned: Linear, Jira, GitLab
package tracker

import "context"

// WorkItemSource fetches work items from a project tracker.
// This interface is the primary input to the orchestrator's poll loop.
//
// The WorkItem type used by implementations is orchestrator.WorkItem.
// Each tracker backend normalizes its native types into that struct.
// This interface is defined identically in orchestrator.WorkItemSource —
// tracker backends implement orchestrator.WorkItemSource directly.
//
// type WorkItemSource interface {
//     FetchCandidates(ctx context.Context) ([]orchestrator.WorkItem, error)
//     FetchStates(ctx context.Context, workItemIDs []string) ([]orchestrator.WorkItem, error)
// }

// WriteBackService performs write-back operations after agent work completes.
// Each tracker backend maps these operations to its native API.
type WriteBackService interface {
	// CreateReviewArtifact pushes a branch and creates/updates a review artifact (PR, MR, etc.).
	// For trackers without code review (Jira), this may just update the ticket status.
	CreateReviewArtifact(ctx context.Context, params ReviewArtifactParams) (*ReviewArtifactResult, error)

	// CommentOnItem posts a comment or update on the work item.
	CommentOnItem(ctx context.Context, owner, repo string, itemNumber int, body string) (string, error)

	// MoveToStatus transitions the work item to a new status in the project.
	// This is used for handoff (e.g., "Todo" → "Human Review").
	MoveToStatus(ctx context.Context, projectID, itemID, fieldID, optionID string) error
}

// ReviewArtifactParams for creating a review artifact (PR/MR).
type ReviewArtifactParams struct {
	Owner      string
	Repo       string
	Title      string
	Body       string
	HeadBranch string
	BaseBranch string
	Draft      bool
}

// ReviewArtifactResult from a review artifact creation.
type ReviewArtifactResult struct {
	Number  int
	URL     string
	State   string
	IsDraft bool
	Created bool // true if newly created, false if existing was updated
}

// TrackerAuth provides authentication for tracker API operations.
type TrackerAuth interface {
	// Token returns a valid authentication token.
	Token(ctx context.Context) (string, error)

	// Mode returns the authentication mode identifier (e.g., "pat", "app", "oauth").
	Mode() string
}

// WorkItem is the normalized domain model shared across all tracker backends.
// Each backend converts its native types into this struct.
// This is the same as orchestrator.WorkItem — aliased here to define
// the canonical location for the domain model.
//
// Note: Currently orchestrator.WorkItem is the authoritative type.
// This package documents the contract that all tracker backends must produce.
// A future refactoring may move WorkItem here and have orchestrator import it.

// TrackerKind identifies which tracker backend to use.
type TrackerKind string

const (
	TrackerGitHub TrackerKind = "github"
	TrackerLinear TrackerKind = "linear"
	TrackerJira   TrackerKind = "jira"
	TrackerGitLab TrackerKind = "gitlab"
)
