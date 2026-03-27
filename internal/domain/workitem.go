package domain

// WorkItem is the canonical domain model for a project item.
// All packages import this type — no conversion layers needed.
type WorkItem struct {
	WorkItemID      string
	ProjectItemID   string
	ContentType     string // "issue", "draft_issue", "pull_request"
	IssueID         string
	IssueNumber     *int
	IssueIdentifier string // "owner/repo#number"
	Title           string
	Description     string
	State           string // "open", "closed"
	ProjectStatus   string
	Priority        *int
	Labels          []string
	Assignees       []string
	Milestone       string
	ProjectFields   map[string]any
	BlockedBy       []BlockerRef
	SubIssues       []ChildRef
	ParentIssue     *ParentRef
	LinkedPRs       []PRRef
	Repository      *Repository
	URL             string
	CreatedAt       string
	UpdatedAt       string
	Pass2Failed     bool // true if dependency data is incomplete — do not dispatch
}

type BlockerRef struct {
	ID         string
	Identifier string
	State      string
}

type ChildRef struct {
	ID         string
	Identifier string
	State      string
}

type ParentRef struct {
	ID         string
	Identifier string
}

type PRRef struct {
	ID      string
	Number  int
	State   string
	IsDraft bool
	URL     string
}

type Repository struct {
	Owner         string
	Name          string
	FullName      string
	DefaultBranch string
	CloneURLHTTPS string
}
