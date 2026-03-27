package linear

// Linear API response models.

// IssueNode is a single issue from the Linear API.
type IssueNode struct {
	ID          string       `json:"id"`
	Identifier  string       `json:"identifier"` // e.g., "ENG-123"
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Priority    int          `json:"priority"` // 0=none, 1=urgent, 2=high, 3=medium, 4=low
	State       StateNode    `json:"state"`
	Labels      LabelsConn   `json:"labels"`
	Assignee    *AssigneeNode `json:"assignee"`
	CreatedAt   string       `json:"createdAt"`
	UpdatedAt   string       `json:"updatedAt"`
	URL         string       `json:"url"`
	Team        TeamNode     `json:"team"`
	Project     *ProjectNode `json:"project"`
	Relations   RelationsConn `json:"relations"`
	BranchName  string       `json:"branchName"`
}

type StateNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "backlog", "unstarted", "started", "completed", "cancelled"
}

type LabelsConn struct {
	Nodes []LabelNode `json:"nodes"`
}

type LabelNode struct {
	Name string `json:"name"`
}

type AssigneeNode struct {
	Name string `json:"name"`
}

type TeamNode struct {
	ID   string `json:"id"`
	Key  string `json:"key"` // e.g., "ENG"
	Name string `json:"name"`
}

type ProjectNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RelationsConn struct {
	Nodes []RelationNode `json:"nodes"`
}

type RelationNode struct {
	Type         string    `json:"type"` // "blocks", "blocked_by", "related", "duplicate"
	RelatedIssue IssueRef  `json:"relatedIssue"`
}

type IssueRef struct {
	ID         string    `json:"id"`
	Identifier string    `json:"identifier"`
	State      StateNode `json:"state"`
}

// IssuesResponse is the top-level response for team issues queries.
type IssuesResponse struct {
	Team struct {
		Issues struct {
			Nodes    []IssueNode `json:"nodes"`
			PageInfo PageInfo    `json:"pageInfo"`
		} `json:"issues"`
	} `json:"team"`
}

// IssuesByIDsResponse is the response for fetching specific issues by ID.
type IssuesByIDsResponse struct {
	Issues struct {
		Nodes []IssueNode `json:"nodes"`
	} `json:"issues"`
}

type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// WorkflowStatesResponse is the response for fetching team workflow states.
type WorkflowStatesResponse struct {
	Team struct {
		States struct {
			Nodes []StateNode `json:"nodes"`
		} `json:"states"`
	} `json:"team"`
}
