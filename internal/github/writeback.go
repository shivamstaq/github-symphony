package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WriteBack handles deterministic GitHub write-back operations.
type WriteBack struct {
	baseURL         string // REST API base (e.g., https://api.github.com)
	graphqlEndpoint string // GraphQL endpoint (e.g., https://api.github.com/graphql)
	token           string
	client          *http.Client
}

// NewWriteBack creates a new write-back client.
// graphqlEndpoint should be the full GraphQL URL (not derived from baseURL).
func NewWriteBack(baseURL, graphqlEndpoint, token string) *WriteBack {
	return &WriteBack{
		baseURL:         strings.TrimSuffix(baseURL, "/"),
		graphqlEndpoint: graphqlEndpoint,
		token:           token,
		client:          &http.Client{},
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
// If a PR already exists for the head branch, it updates the existing one.
func (wb *WriteBack) UpsertPR(ctx context.Context, params PRParams) (*PRResult, error) {
	// First, check for existing PR on this head branch
	existing, err := wb.findExistingPR(ctx, params.Owner, params.Repo, params.HeadBranch)
	if err != nil {
		return nil, fmt.Errorf("pr upsert: find existing: %w", err)
	}

	if existing != nil {
		// Update existing PR
		body := map[string]any{
			"title": params.Title,
			"body":  params.Body,
		}
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d", params.Owner, params.Repo, getIntFromJSON(existing, "number"))
		resp, err := wb.restPatch(ctx, path, body)
		if err != nil {
			return nil, fmt.Errorf("pr upsert: update: %w", err)
		}
		return &PRResult{
			Number:  getIntFromJSON(resp, "number"),
			URL:     getStrFromJSON(resp, "html_url"),
			State:   getStrFromJSON(resp, "state"),
			IsDraft: getBoolFromJSON(resp, "draft"),
			Created: false,
		}, nil
	}

	// Create new PR
	body := map[string]any{
		"title": params.Title,
		"body":  params.Body,
		"head":  params.HeadBranch,
		"base":  params.BaseBranch,
		"draft": params.Draft,
	}

	resp, err := wb.restPost(ctx, fmt.Sprintf("/repos/%s/%s/pulls", params.Owner, params.Repo), body)
	if err != nil {
		return nil, fmt.Errorf("pr upsert: create: %w", err)
	}

	return &PRResult{
		Number:  getIntFromJSON(resp, "number"),
		URL:     getStrFromJSON(resp, "html_url"),
		State:   getStrFromJSON(resp, "state"),
		IsDraft: getBoolFromJSON(resp, "draft"),
		Created: true,
	}, nil
}

func (wb *WriteBack) findExistingPR(ctx context.Context, owner, repo, headBranch string) (map[string]any, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls?head=%s:%s&state=open", owner, repo, owner, headBranch)
	resp, err := wb.restGet(ctx, path)
	if err != nil {
		return nil, err
	}
	prs, ok := resp.([]any)
	if !ok || len(prs) == 0 {
		return nil, nil
	}
	if first, ok := prs[0].(map[string]any); ok {
		return first, nil
	}
	return nil, nil
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

// UpdateProjectField updates a project single-select field via GraphQL.
func (wb *WriteBack) UpdateProjectField(ctx context.Context, projectID, itemID, fieldID, optionID string) error {
	query := `mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String!) {
		updateProjectV2ItemFieldValue(input: {
			projectId: $projectId
			itemId: $itemId
			fieldId: $fieldId
			value: { singleSelectOptionId: $optionId }
		}) {
			projectV2Item { id }
		}
	}`
	variables := map[string]any{
		"projectId": projectID,
		"itemId":    itemID,
		"fieldId":   fieldID,
		"optionId":  optionID,
	}
	_, err := wb.graphqlPost(ctx, query, variables)
	return err
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

func (wb *WriteBack) restGet(ctx context.Context, path string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", wb.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+wb.token)
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
	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (wb *WriteBack) restPatch(ctx context.Context, path string, body any) (map[string]any, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "PATCH", wb.baseURL+path, bytes.NewReader(jsonBody))
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

func (wb *WriteBack) graphqlPost(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	body := map[string]any{"query": query, "variables": variables}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := wb.graphqlEndpoint
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+wb.token)
	req.Header.Set("Content-Type", "application/json")

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
	var result struct {
		Data   map[string]any `json:"data"`
		Errors []any          `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("github_graphql_errors: %v", result.Errors)
	}
	return result.Data, nil
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
