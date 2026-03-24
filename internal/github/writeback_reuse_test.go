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

func TestWriteBack_ReusesExistingPR(t *testing.T) {
	var patchCalled bool
	var patchBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// GET /repos/org/repo/pulls?head=...&state=open -> return existing PR
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/org/repo/pulls") {
			_ = json.NewEncoder(w).Encode([]any{
				map[string]any{
					"id":       1,
					"number":   50,
					"html_url": "https://github.com/org/repo/pull/50",
					"state":    "open",
					"draft":    true,
				},
			})
			return
		}

		// PATCH /repos/org/repo/pulls/50 -> update existing
		if r.Method == "PATCH" && r.URL.Path == "/repos/org/repo/pulls/50" {
			patchCalled = true
			json.NewDecoder(r.Body).Decode(&patchBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       1,
				"number":   50,
				"html_url": "https://github.com/org/repo/pull/50",
				"state":    "open",
				"draft":    true,
			})
			return
		}

		w.WriteHeader(404)
	}))
	defer server.Close()

	wb := ghub.NewWriteBack(server.URL, "ghp_test")
	result, err := wb.UpsertPR(context.Background(), ghub.PRParams{
		Owner:      "org",
		Repo:       "repo",
		Title:      "Updated title",
		Body:       "Updated body",
		HeadBranch: "symphony/org_repo_42",
		BaseBranch: "main",
		Draft:      true,
	})
	if err != nil {
		t.Fatalf("UpsertPR failed: %v", err)
	}

	if result.Created {
		t.Error("expected Created=false when reusing existing PR")
	}
	if result.Number != 50 {
		t.Errorf("expected PR number=50, got %d", result.Number)
	}
	if !patchCalled {
		t.Error("expected PATCH to be called for existing PR")
	}
	if patchBody["title"] != "Updated title" {
		t.Errorf("expected title update, got %v", patchBody["title"])
	}
}

func TestWriteBack_CreatesNewPRWhenNoneExists(t *testing.T) {
	var postCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// GET returns empty array — no existing PR
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/org/repo/pulls") {
			_ = json.NewEncoder(w).Encode([]any{})
			return
		}

		// POST creates new PR
		if r.Method == "POST" && r.URL.Path == "/repos/org/repo/pulls" {
			postCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       2,
				"number":   99,
				"html_url": "https://github.com/org/repo/pull/99",
				"state":    "open",
				"draft":    true,
			})
			return
		}

		w.WriteHeader(404)
	}))
	defer server.Close()

	wb := ghub.NewWriteBack(server.URL, "ghp_test")
	result, err := wb.UpsertPR(context.Background(), ghub.PRParams{
		Owner: "org", Repo: "repo", Title: "New PR",
		HeadBranch: "symphony/org_repo_1", BaseBranch: "main", Draft: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !result.Created {
		t.Error("expected Created=true for new PR")
	}
	if !postCalled {
		t.Error("expected POST to be called for new PR")
	}
	if result.Number != 99 {
		t.Errorf("expected number=99, got %d", result.Number)
	}
}
