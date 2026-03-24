package github_test

import (
	"context"
	"testing"

	ghub "github.com/shivamstaq/github-symphony/internal/github"
)

func TestClientTools_ReturnsAllFive(t *testing.T) {
	tools := ghub.ClientTools()
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil input schema", tool.Name)
		}
	}

	expected := []string{
		"github_issue_read",
		"github_issue_comment",
		"github_project_update_field",
		"github_pull_request_upsert",
		"github_repo_read_file",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestAppProvider_ReturnsError(t *testing.T) {
	provider := ghub.NewAppProvider()

	_, err := provider.Token(context.Background(), ghub.RepoRef{})
	if err == nil {
		t.Fatal("expected error from App auth stub")
	}

	if provider.Mode() != "app" {
		t.Errorf("expected mode=app, got %q", provider.Mode())
	}
}

func TestGitHubError_Format(t *testing.T) {
	err := &ghub.GitHubError{
		Kind:    ghub.ErrGitHubAPIRateLimited,
		Message: "rate limit exceeded",
	}
	if err.Error() != "github_api_rate_limited: rate limit exceeded" {
		t.Errorf("unexpected error string: %q", err.Error())
	}
}
