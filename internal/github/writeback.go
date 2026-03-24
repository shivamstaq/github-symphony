package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// WriteBack handles deterministic GitHub write-back operations.
type WriteBack struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewWriteBack creates a new write-back client.
func NewWriteBack(baseURL, token string) *WriteBack {
	return &WriteBack{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{},
	}
}

// PRParams for creating/updating a pull request.
type PRParams struct {
	Owner      string
	Repo       string
	Title      string
	Body       string
	HeadBranch string
	BaseBranch string
	Draft      bool
}

// PRResult from a PR upsert operation.
type PRResult struct {
	Number  int
	URL     string
	State   string
	IsDraft bool
	Created bool
}

// UpsertPR creates or updates a pull request.
func (wb *WriteBack) UpsertPR(ctx context.Context, params PRParams) (*PRResult, error) {
	// Try to create a new PR
	body := map[string]any{
		"title": params.Title,
		"body":  params.Body,
		"head":  params.HeadBranch,
		"base":  params.BaseBranch,
		"draft": params.Draft,
	}

	resp, err := wb.restPost(ctx, fmt.Sprintf("/repos/%s/%s/pulls", params.Owner, params.Repo), body)
	if err != nil {
		return nil, fmt.Errorf("pr upsert: %w", err)
	}

	return &PRResult{
		Number:  getIntFromJSON(resp, "number"),
		URL:     getStrFromJSON(resp, "html_url"),
		State:   getStrFromJSON(resp, "state"),
		IsDraft: getBoolFromJSON(resp, "draft"),
		Created: true,
	}, nil
}

// CommentOnIssue posts a comment on an issue.
func (wb *WriteBack) CommentOnIssue(ctx context.Context, owner, repo string, issueNumber int, body string) (string, error) {
	payload := map[string]any{"body": body}

	resp, err := wb.restPost(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, issueNumber), payload)
	if err != nil {
		return "", fmt.Errorf("issue comment: %w", err)
	}

	return getStrFromJSON(resp, "html_url"), nil
}

// UpdateProjectField updates a project field value via GraphQL.
// This is a placeholder — the actual mutation depends on field type.
func (wb *WriteBack) UpdateProjectField(ctx context.Context, projectID, itemID, fieldID, value string) error {
	// TODO: implement GraphQL mutation for project field update
	_ = ctx
	return nil
}

func (wb *WriteBack) restPost(ctx context.Context, path string, body any) (map[string]any, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", wb.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+wb.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := wb.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github_api_status: %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func getStrFromJSON(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getIntFromJSON(m map[string]any, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func getBoolFromJSON(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}
