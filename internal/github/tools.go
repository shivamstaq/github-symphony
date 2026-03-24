package github

// ToolDescriptor describes a client-side GitHub tool exposed to the agent.
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ClientTools returns the baseline set of typed GitHub tools per spec Section 11.7.
func ClientTools() []ToolDescriptor {
	return []ToolDescriptor{
		{
			Name:        "github_issue_read",
			Description: "Fetch canonical issue details for a repository issue.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository":   map[string]any{"type": "string", "description": "owner/repo"},
					"issue_number": map[string]any{"type": "integer"},
				},
				"required": []string{"repository", "issue_number"},
			},
		},
		{
			Name:        "github_issue_comment",
			Description: "Append a comment to a GitHub issue.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository":      map[string]any{"type": "string"},
					"issue_number":    map[string]any{"type": "integer"},
					"body":            map[string]any{"type": "string"},
					"idempotency_key": map[string]any{"type": "string"},
				},
				"required": []string{"repository", "issue_number", "body"},
			},
		},
		{
			Name:        "github_project_update_field",
			Description: "Update a project field value for the current project item.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_item_id": map[string]any{"type": "string"},
					"field_name":      map[string]any{"type": "string"},
					"value":           map[string]any{"type": "string"},
				},
				"required": []string{"project_item_id", "field_name", "value"},
			},
		},
		{
			Name:        "github_pull_request_upsert",
			Description: "Create or update a pull request for the current work branch.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository":  map[string]any{"type": "string"},
					"base_branch": map[string]any{"type": "string"},
					"head_branch": map[string]any{"type": "string"},
					"title":       map[string]any{"type": "string"},
					"body":        map[string]any{"type": "string"},
					"draft":       map[string]any{"type": "boolean"},
				},
				"required": []string{"repository", "base_branch", "head_branch", "title"},
			},
		},
		{
			Name:        "github_repo_read_file",
			Description: "Read a file from a repository via the GitHub API.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string"},
					"path":       map[string]any{"type": "string"},
					"ref":        map[string]any{"type": "string"},
				},
				"required": []string{"repository", "path"},
			},
		},
	}
}
