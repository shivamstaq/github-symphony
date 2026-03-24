package github

// WorkItemRaw is a partially-parsed project item from the GraphQL response.
// Pass 1 populates: ProjectItemID, ContentType, IssueID, IssueNumber, Title, State, ProjectStatus, Repository.
// Pass 2 adds: Description, Labels, Assignees, Milestone, BlockedBy, SubIssues, LinkedPRs, CreatedAt, UpdatedAt, URL.
type WorkItemRaw struct {
	ProjectItemID string
	ContentType   string // "issue", "draft_issue", "pull_request"
	IssueID       string
	IssueNumber   *int
	Title         string
	Description   string
	State         string
	ProjectStatus string
	Priority      string // raw priority field value
	Labels        []string
	Assignees     []string
	Milestone     string
	BlockedBy     []BlockerRefRaw
	SubIssues     []ChildRefRaw
	LinkedPRs     []PRRefRaw
	Repository    *RepositoryInfo
	URL           string
	CreatedAt     string
	UpdatedAt     string
}

// BlockerRefRaw is a dependency/blocker reference from GitHub.
type BlockerRefRaw struct {
	ID         string
	Identifier string // owner/repo#number
	State      string
}

// ChildRefRaw is a sub-issue reference from GitHub.
type ChildRefRaw struct {
	ID         string
	Identifier string
	State      string
}

// PRRefRaw is a linked PR reference from GitHub.
type PRRefRaw struct {
	ID      string
	Number  int
	State   string
	IsDraft bool
	URL     string
}

// RepositoryInfo holds repository metadata.
type RepositoryInfo struct {
	Owner         string
	Name          string
	FullName      string
	DefaultBranch string
	CloneURLHTTPS string
}

// ProjectQuery holds the parameters for fetching project items.
type ProjectQuery struct {
	Owner           string
	ProjectNumber   int
	ProjectScope    string // "organization" or "user"
	StatusFieldName string
	PageSize        int
}
