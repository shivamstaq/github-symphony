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

// GraphQLClient handles GitHub GraphQL API requests.
type GraphQLClient struct {
	endpoint string
	token    string
	client   *http.Client
}

// NewGraphQLClient creates a new GraphQL client.
func NewGraphQLClient(endpoint, token string) *GraphQLClient {
	return &GraphQLClient{
		endpoint: endpoint,
		token:    token,
		client:   &http.Client{},
	}
}

// FetchProjectItems fetches project items with status field values (pass 1 of two-pass query).
func (c *GraphQLClient) FetchProjectItems(ctx context.Context, q ProjectQuery) ([]WorkItemRaw, error) {
	var allItems []WorkItemRaw
	var cursor *string

	for {
		items, pageInfo, err := c.fetchProjectItemsPage(ctx, q, cursor)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, items...)

		if !pageInfo.HasNextPage {
			break
		}
		cursor = pageInfo.EndCursor
	}

	return allItems, nil
}

type pageInfo struct {
	HasNextPage bool
	EndCursor   *string
}

func (c *GraphQLClient) fetchProjectItemsPage(ctx context.Context, q ProjectQuery, cursor *string) ([]WorkItemRaw, pageInfo, error) {
	// Build the query depending on scope
	ownerField := "organization"
	if q.ProjectScope == "user" {
		ownerField = "user"
	}

	afterClause := ""
	if cursor != nil {
		afterClause = fmt.Sprintf(`, after: %q`, *cursor)
	}

	query := fmt.Sprintf(`query {
  %s(login: %q) {
    projectV2(number: %d) {
      items(first: %d%s) {
        nodes {
          id
          fieldValueByName(name: %q) {
            ... on ProjectV2ItemFieldSingleSelectValue { name }
          }
          content {
            __typename
            ... on Issue {
              id
              number
              title
              state
              repository {
                owner { login }
                name
                nameWithOwner
                defaultBranchRef { name }
              }
            }
            ... on DraftIssue {
              title
            }
            ... on PullRequest {
              id
              number
              title
              state
              repository {
                owner { login }
                name
                nameWithOwner
                defaultBranchRef { name }
              }
            }
          }
        }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
  }
}`, ownerField, q.Owner, q.ProjectNumber, q.PageSize, afterClause, q.StatusFieldName)

	respData, err := c.doGraphQL(ctx, query, nil)
	if err != nil {
		return nil, pageInfo{}, err
	}

	// Navigate the response
	orgOrUser, ok := respData[ownerField].(map[string]any)
	if !ok {
		return nil, pageInfo{}, fmt.Errorf("github_graphql_errors: missing %s in response", ownerField)
	}
	project, ok := orgOrUser["projectV2"].(map[string]any)
	if !ok {
		return nil, pageInfo{}, fmt.Errorf("github_graphql_errors: missing projectV2 in response")
	}
	itemsObj, ok := project["items"].(map[string]any)
	if !ok {
		return nil, pageInfo{}, fmt.Errorf("github_graphql_errors: missing items in response")
	}

	nodes, _ := itemsObj["nodes"].([]any)
	pi := parsePageInfo(itemsObj["pageInfo"])

	var items []WorkItemRaw
	for _, node := range nodes {
		n, ok := node.(map[string]any)
		if !ok {
			continue
		}
		item := parseProjectItem(n)
		items = append(items, item)
	}

	return items, pi, nil
}

func (c *GraphQLClient) doGraphQL(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("github_api_request: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("github_api_request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github_api_request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("github_api_request: read body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github_api_status: %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data   map[string]any `json:"data"`
		Errors []any          `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("github_api_request: unmarshal: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("github_graphql_errors: %v", result.Errors)
	}
	if result.Data == nil {
		return nil, fmt.Errorf("github_graphql_errors: null data in response")
	}

	return result.Data, nil
}

func parseProjectItem(n map[string]any) WorkItemRaw {
	item := WorkItemRaw{
		ProjectItemID: getString(n, "id"),
	}

	// Parse status field
	if fv, ok := n["fieldValueByName"].(map[string]any); ok {
		item.ProjectStatus = getString(fv, "name")
	}

	// Parse content
	content, ok := n["content"].(map[string]any)
	if !ok {
		return item
	}

	typename := getString(content, "__typename")
	switch typename {
	case "Issue":
		item.ContentType = "issue"
		item.IssueID = getString(content, "id")
		if num, ok := content["number"].(float64); ok {
			n := int(num)
			item.IssueNumber = &n
		}
		item.Title = getString(content, "title")
		item.State = strings.ToLower(getString(content, "state"))
		item.Repository = parseRepository(content)
	case "DraftIssue":
		item.ContentType = "draft_issue"
		item.Title = getString(content, "title")
	case "PullRequest":
		item.ContentType = "pull_request"
		item.IssueID = getString(content, "id")
		if num, ok := content["number"].(float64); ok {
			n := int(num)
			item.IssueNumber = &n
		}
		item.Title = getString(content, "title")
		item.State = strings.ToLower(getString(content, "state"))
		item.Repository = parseRepository(content)
	}

	return item
}

func parseRepository(content map[string]any) *RepositoryInfo {
	repo, ok := content["repository"].(map[string]any)
	if !ok {
		return nil
	}

	info := &RepositoryInfo{
		Name:     getString(repo, "name"),
		FullName: getString(repo, "nameWithOwner"),
	}

	if owner, ok := repo["owner"].(map[string]any); ok {
		info.Owner = getString(owner, "login")
	}
	if defBranch, ok := repo["defaultBranchRef"].(map[string]any); ok {
		info.DefaultBranch = getString(defBranch, "name")
	}

	return info
}

func parsePageInfo(raw any) pageInfo {
	pi := pageInfo{}
	m, ok := raw.(map[string]any)
	if !ok {
		return pi
	}
	if v, ok := m["hasNextPage"].(bool); ok {
		pi.HasNextPage = v
	}
	if v, ok := m["endCursor"].(string); ok {
		pi.EndCursor = &v
	}
	return pi
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
