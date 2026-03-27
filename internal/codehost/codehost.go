package codehost

import "context"

// CodeHost handles git operations and pull request management.
// Implementations: codehost/github (future: codehost/gitlab)
type CodeHost interface {
	// UpsertPR creates or updates a pull request.
	UpsertPR(ctx context.Context, params PRParams) (*PRResult, error)

	// CommentOnItem posts a comment on an issue/PR.
	CommentOnItem(ctx context.Context, ref ItemRef, body string) (string, error)

	// UpdateProjectStatus moves a project item to a new status.
	UpdateProjectStatus(ctx context.Context, params StatusUpdateParams) error

	// FetchProjectMeta retrieves project field IDs for status updates.
	FetchProjectMeta(ctx context.Context, params ProjectMetaParams) (*ProjectMeta, error)
}

// PRParams contains parameters for creating/updating a PR.
type PRParams struct {
	Owner      string
	Repo       string
	Title      string
	Body       string
	HeadBranch string
	BaseBranch string
	Draft      bool
}

// PRResult contains the result of a PR create/update.
type PRResult struct {
	Number  int
	URL     string
	State   string
	IsDraft bool
	Created bool // true if newly created, false if updated
}

// ItemRef identifies an issue or PR.
type ItemRef struct {
	Owner  string
	Repo   string
	Number int
}

// StatusUpdateParams for moving a project item status.
type StatusUpdateParams struct {
	ProjectID string
	ItemID    string
	FieldID   string
	OptionID  string
}

// ProjectMetaParams for looking up project field metadata.
type ProjectMetaParams struct {
	Owner         string
	ProjectNumber int
	Scope         string // "organization" or "user"
	FieldName     string
}

// ProjectMeta contains resolved project field IDs.
type ProjectMeta struct {
	ProjectID string
	FieldID   string
	Options   map[string]string // option name -> option ID
}
