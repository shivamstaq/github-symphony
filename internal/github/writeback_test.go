package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ghub "github.com/shivamstaq/github-symphony/internal/github"
)

func TestWriteBack_CreatePR(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/org/repo/pulls"):
			// No existing PRs
			json.NewEncoder(w).Encode([]any{})
		case r.Method == "POST" && r.URL.Path == "/repos/org/repo/pulls":
			json.NewDecoder(r.Body).Decode(&receivedBody)
			json.NewEncoder(w).Encode(map[string]any{
				"id":       1,
				"number":   99,
				"html_url": "https://github.com/org/repo/pull/99",
				"state":    "open",
				"draft":    true,
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	wb := ghub.NewWriteBack(server.URL, server.URL+"/graphql", "ghp_test")

	result, err := wb.UpsertPR(context.Background(), ghub.PRParams{
		Owner:      "org",
		Repo:       "repo",
		Title:      "Fix: resolve flaky test",
		Body:       "Automated fix by Symphony",
		HeadBranch: "symphony/org_repo_42",
		BaseBranch: "main",
		Draft:      true,
	})
	if err != nil {
		t.Fatalf("UpsertPR failed: %v", err)
	}

	if result.Number != 99 {
		t.Errorf("expected PR number=99, got %d", result.Number)
	}
	if !result.Created {
		t.Error("expected Created=true for new PR")
	}
}

func TestWriteBack_CommentOnIssue(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/repos/org/repo/issues/42/comments" {
			json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id":       1,
				"html_url": "https://github.com/org/repo/issues/42#issuecomment-1",
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	wb := ghub.NewWriteBack(server.URL, server.URL+"/graphql", "ghp_test")

	url, err := wb.CommentOnIssue(context.Background(), "org", "repo", 42, "PR created: https://github.com/org/repo/pull/99")
	if err != nil {
		t.Fatalf("CommentOnIssue failed: %v", err)
	}

	if url == "" {
		t.Error("expected non-empty comment URL")
	}

	if receivedBody["body"] != "PR created: https://github.com/org/repo/pull/99" {
		t.Errorf("unexpected comment body: %v", receivedBody["body"])
	}
}
