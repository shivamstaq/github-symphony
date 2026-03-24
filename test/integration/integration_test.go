//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/shivamstaq/github-symphony/internal/config"
	ghub "github.com/shivamstaq/github-symphony/internal/github"
	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

func init() {
	// Load .env from project root (two levels up from test/integration/)
	_ = godotenv.Load("../../.env")
}

func TestIntegration_FetchRealProjectItems(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set")
	}

	// Load test workflow
	wf, err := config.LoadWorkflow("WORKFLOW_test.md")
	if err != nil {
		t.Fatalf("load workflow: %v", err)
	}

	cfg, err := config.NewServiceConfig(wf.Config)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	// Create GitHub client
	gqlClient := ghub.NewGraphQLClient("https://api.github.com/graphql", token)
	source := ghub.NewSource(gqlClient, ghub.SourceConfig{
		Owner:           cfg.Tracker.Owner,
		ProjectNumber:   cfg.Tracker.ProjectNumber,
		ProjectScope:    cfg.Tracker.ProjectScope,
		StatusFieldName: cfg.Tracker.StatusFieldName,
		PageSize:        cfg.GitHub.GraphQLPageSize,
	})

	// Fetch candidates
	rawItems, err := source.FetchCandidateRaw(context.Background())
	if err != nil {
		t.Fatalf("fetch candidates: %v", err)
	}

	t.Logf("fetched %d raw items from GitHub Project #%d", len(rawItems), cfg.Tracker.ProjectNumber)

	if len(rawItems) == 0 {
		t.Fatal("expected at least 1 project item — verify project has items in active status")
	}

	// Verify normalization
	for _, raw := range rawItems {
		item := ghub.NormalizeWorkItem(raw, nil)
		t.Logf("  item: %s — %q (status=%q, state=%q, type=%q)",
			item.WorkItemID, item.Title, raw.ProjectStatus, item.State, item.ContentType)

		if item.WorkItemID == "" {
			t.Error("work_item_id should not be empty")
		}
		if item.Title == "" {
			t.Error("title should not be empty")
		}
		if item.ContentType == "" {
			t.Error("content_type should not be empty")
		}
	}
}

func TestIntegration_SourceBridgeFetchCandidates(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set")
	}

	wf, err := config.LoadWorkflow("WORKFLOW_test.md")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewServiceConfig(wf.Config)
	if err != nil {
		t.Fatal(err)
	}

	gqlClient := ghub.NewGraphQLClient("https://api.github.com/graphql", token)
	source := ghub.NewSource(gqlClient, ghub.SourceConfig{
		Owner:           cfg.Tracker.Owner,
		ProjectNumber:   cfg.Tracker.ProjectNumber,
		ProjectScope:    cfg.Tracker.ProjectScope,
		StatusFieldName: cfg.Tracker.StatusFieldName,
		PageSize:        50,
	})

	bridge := orchestrator.NewSourceBridge(source, nil)
	items, err := bridge.FetchCandidates(context.Background())
	if err != nil {
		t.Fatalf("bridge fetch: %v", err)
	}

	t.Logf("bridge returned %d work items", len(items))

	for _, item := range items {
		t.Logf("  %s: %q (status=%q, repo=%s)",
			item.WorkItemID, item.Title, item.ProjectStatus,
			repoName(item.Repository))

		// Verify all fields are populated
		if item.WorkItemID == "" {
			t.Error("work_item_id empty")
		}
		if item.ProjectItemID == "" {
			t.Error("project_item_id empty")
		}
		if item.ContentType != "issue" && item.ContentType != "draft_issue" && item.ContentType != "pull_request" {
			t.Errorf("unexpected content_type: %q", item.ContentType)
		}
		if item.Repository != nil && item.Repository.CloneURLHTTPS == "" {
			t.Error("clone URL should be derived")
		}
	}
}

func TestIntegration_DoctorConnectivity(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set")
	}

	// Verify PAT can authenticate
	provider := ghub.NewPATProvider(token)
	tok, err := provider.Token(context.Background(), ghub.RepoRef{})
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}

	// Verify API connectivity
	client, err := provider.HTTPClient(context.Background(), ghub.RepoRef{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		t.Fatalf("API call: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	t.Logf("GitHub API authenticated successfully (status %d)", resp.StatusCode)
}

func repoName(r *orchestrator.Repository) string {
	if r == nil {
		return "<nil>"
	}
	return r.FullName
}
