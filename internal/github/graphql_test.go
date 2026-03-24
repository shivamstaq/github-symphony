package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ghub "github.com/shivamstaq/github-symphony/internal/github"
)

func TestGraphQLClient_FetchProjectItems(t *testing.T) {
	// Mock GitHub GraphQL API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp_test" {
			t.Errorf("missing or wrong auth header: %q", r.Header.Get("Authorization"))
		}

		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		resp := map[string]any{
			"data": map[string]any{
				"organization": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []any{
								map[string]any{
									"id": "PVTI_item1",
									"fieldValueByName": map[string]any{
										"name": "Todo",
									},
									"content": map[string]any{
										"__typename": "Issue",
										"id":         "I_issue1",
										"number":     42,
										"title":      "Fix flaky test",
										"state":      "OPEN",
										"repository": map[string]any{
											"owner": map[string]any{
												"login": "myorg",
											},
											"name":          "myrepo",
											"nameWithOwner": "myorg/myrepo",
											"defaultBranchRef": map[string]any{
												"name": "main",
											},
										},
									},
								},
							},
							"pageInfo": map[string]any{
								"hasNextPage": false,
								"endCursor":   nil,
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := ghub.NewGraphQLClient(server.URL, "ghp_test")

	items, err := client.FetchProjectItems(context.Background(), ghub.ProjectQuery{
		Owner:           "myorg",
		ProjectNumber:   1,
		ProjectScope:    "organization",
		StatusFieldName: "Status",
		PageSize:        50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.ProjectItemID != "PVTI_item1" {
		t.Errorf("expected ProjectItemID=PVTI_item1, got %q", item.ProjectItemID)
	}
	if item.ProjectStatus != "Todo" {
		t.Errorf("expected ProjectStatus=Todo, got %q", item.ProjectStatus)
	}
	if item.ContentType != "issue" {
		t.Errorf("expected ContentType=issue, got %q", item.ContentType)
	}
	if item.Title != "Fix flaky test" {
		t.Errorf("expected Title='Fix flaky test', got %q", item.Title)
	}
	if item.IssueNumber == nil || *item.IssueNumber != 42 {
		t.Errorf("expected IssueNumber=42, got %v", item.IssueNumber)
	}
	if item.Repository == nil || item.Repository.FullName != "myorg/myrepo" {
		t.Errorf("expected repo=myorg/myrepo, got %v", item.Repository)
	}
}
