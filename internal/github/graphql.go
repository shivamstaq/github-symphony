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
	req.Header.Set("GraphQL-Features", "sub_issues")

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

// FetchIssueDetails fetches full details for a batch of issues (Pass 2).
// Enriches the items with labels, description, assignees, linked PRs, etc.
func (c *GraphQLClient) FetchIssueDetails(ctx context.Context, items []WorkItemRaw) ([]WorkItemRaw, error) {
	for i, item := range items {
		if item.IssueID == "" || item.ContentType != "issue" {
			continue
		}

		query := `query($id: ID!) {
		  node(id: $id) {
		    ... on Issue {
		      body
		      url
		      createdAt
		      updatedAt
		      milestone { title }
		      assignees(first: 20) { nodes { login } }
		      labels(first: 50) { nodes { name } }
		      blockedBy(first: 50) {
		        nodes { id number state repository { nameWithOwner } }
		      }
		      subIssues(first: 50) {
		        nodes { id number state repository { nameWithOwner } }
		      }
		      closingIssuesReferences(first: 20) {
		        nodes { id number state isDraft url repository { nameWithOwner } }
		      }
		    }
		  }
		}`

		data, err := c.doGraphQL(ctx, query, map[string]any{"id": item.IssueID})
		if err != nil {
			// Non-fatal: continue with partial data
			continue
		}

		node, ok := data["node"].(map[string]any)
		if !ok {
			continue
		}

		items[i].Description = getString(node, "body")
		items[i].URL = getString(node, "url")
		items[i].CreatedAt = getString(node, "createdAt")
		items[i].UpdatedAt = getString(node, "updatedAt")

		if ms, ok := node["milestone"].(map[string]any); ok {
			items[i].Milestone = getString(ms, "title")
		}

		if assignees, ok := node["assignees"].(map[string]any); ok {
			if nodes, ok := assignees["nodes"].([]any); ok {
				for _, n := range nodes {
					if a, ok := n.(map[string]any); ok {
						items[i].Assignees = append(items[i].Assignees, getString(a, "login"))
					}
				}
			}
		}

		if labels, ok := node["labels"].(map[string]any); ok {
			if nodes, ok := labels["nodes"].([]any); ok {
				for _, n := range nodes {
					if l, ok := n.(map[string]any); ok {
						items[i].Labels = append(items[i].Labels, strings.ToLower(getString(l, "name")))
					}
				}
			}
		}

		// Blocking dependencies (issues that must close before this one can proceed)
		if blocked, ok := node["blockedBy"].(map[string]any); ok {
			if nodes, ok := blocked["nodes"].([]any); ok {
				for _, n := range nodes {
					if b, ok := n.(map[string]any); ok {
						ref := BlockerRefRaw{
							ID:    getString(b, "id"),
							State: strings.ToLower(getString(b, "state")),
						}
						if num, ok := b["number"].(float64); ok {
							if repo, ok := b["repository"].(map[string]any); ok {
								ref.Identifier = fmt.Sprintf("%s#%d", getString(repo, "nameWithOwner"), int(num))
							}
						}
						items[i].BlockedBy = append(items[i].BlockedBy, ref)
					}
				}
			}
		}

		// Sub-issues
		if subIssues, ok := node["subIssues"].(map[string]any); ok {
			if nodes, ok := subIssues["nodes"].([]any); ok {
				for _, n := range nodes {
					if s, ok := n.(map[string]any); ok {
						ref := ChildRefRaw{
							ID:    getString(s, "id"),
							State: strings.ToLower(getString(s, "state")),
						}
						if num, ok := s["number"].(float64); ok {
							if repo, ok := s["repository"].(map[string]any); ok {
								ref.Identifier = fmt.Sprintf("%s#%d", getString(repo, "nameWithOwner"), int(num))
							}
						}
						items[i].SubIssues = append(items[i].SubIssues, ref)
					}
				}
			}
		}

		// Linked PRs (closingIssuesReferences)
		if closing, ok := node["closingIssuesReferences"].(map[string]any); ok {
			if nodes, ok := closing["nodes"].([]any); ok {
				for _, n := range nodes {
					if p, ok := n.(map[string]any); ok {
						pr := PRRefRaw{
							ID:      getString(p, "id"),
							State:   strings.ToLower(getString(p, "state")),
							IsDraft: getBool(p, "isDraft"),
							URL:     getString(p, "url"),
						}
						if num, ok := p["number"].(float64); ok {
							pr.Number = int(num)
						}
						items[i].LinkedPRs = append(items[i].LinkedPRs, pr)
					}
				}
			}
		}

		// Derive clone URL if not set
		if items[i].Repository != nil && items[i].Repository.CloneURLHTTPS == "" {
			items[i].Repository.CloneURLHTTPS = fmt.Sprintf("https://github.com/%s.git", items[i].Repository.FullName)
		}
	}

	return items, nil
}

// ProjectFieldMeta holds the IDs needed to update a project status field.
type ProjectFieldMeta struct {
	ProjectID string
	FieldID   string
	Options   map[string]string // option name → option ID
}

// FetchProjectFieldMeta fetches the project ID, status field ID, and option IDs.
func (c *GraphQLClient) FetchProjectFieldMeta(ctx context.Context, owner string, projectNumber int, scope string, fieldName string) (*ProjectFieldMeta, error) {
	ownerField := "organization"
	if scope == "user" {
		ownerField = "user"
	}

	query := fmt.Sprintf(`query {
	  %s(login: %q) {
	    projectV2(number: %d) {
	      id
	      field(name: %q) {
	        ... on ProjectV2SingleSelectField {
	          id
	          options { id name }
	        }
	      }
	    }
	  }
	}`, ownerField, owner, projectNumber, fieldName)

	data, err := c.doGraphQL(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch project field meta: %w", err)
	}

	ownerData, ok := data[ownerField].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("fetch project field meta: missing %s in response", ownerField)
	}
	project, ok := ownerData["projectV2"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("fetch project field meta: missing projectV2")
	}
	field, ok := project["field"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("fetch project field meta: field %q not found or not single-select", fieldName)
	}

	meta := &ProjectFieldMeta{
		ProjectID: getString(project, "id"),
		FieldID:   getString(field, "id"),
		Options:   make(map[string]string),
	}

	if options, ok := field["options"].([]any); ok {
		for _, opt := range options {
			if o, ok := opt.(map[string]any); ok {
				name := getString(o, "name")
				id := getString(o, "id")
				if name != "" && id != "" {
					meta.Options[name] = id
				}
			}
		}
	}

	return meta, nil
}

// ConvertDraftIssue converts a draft project item to a real issue.
func (c *GraphQLClient) ConvertDraftIssue(ctx context.Context, itemID, repoID string) (string, error) {
	query := `mutation($itemId: ID!, $repositoryId: ID!) {
		convertProjectV2DraftIssueItemToIssue(input: {
			itemId: $itemId
			repositoryId: $repositoryId
		}) {
			item { id }
		}
	}`

	data, err := c.doGraphQL(ctx, query, map[string]any{
		"itemId":       itemID,
		"repositoryId": repoID,
	})
	if err != nil {
		return "", fmt.Errorf("convert draft issue: %w", err)
	}

	if convert, ok := data["convertProjectV2DraftIssueItemToIssue"].(map[string]any); ok {
		if item, ok := convert["item"].(map[string]any); ok {
			return getString(item, "id"), nil
		}
	}
	return "", nil
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
