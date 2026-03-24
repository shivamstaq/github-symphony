package github

// WorkItemRaw is a partially-parsed project item from the GraphQL response (pass 1).
type WorkItemRaw struct {
	ProjectItemID string
	ContentType   string // "issue", "draft_issue", "pull_request"
	IssueID       string
	IssueNumber   *int
	Title         string
	State         string
	ProjectStatus string
	Repository    *RepositoryInfo
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
