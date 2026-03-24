package prompt_test

import (
	"testing"

	"github.com/shivamstaq/github-symphony/internal/prompt"
)

func TestRender_BasicTemplate(t *testing.T) {
	tmpl := `You are working on {{.work_item.issue_identifier}}: {{.work_item.title}}.
Branch: {{.branch_name}}`

	data := prompt.RenderInput{
		WorkItem: map[string]any{
			"issue_identifier": "org/repo#42",
			"title":            "Fix the flaky test",
		},
		BranchName: "symphony/org_repo_42",
		BaseBranch: "main",
	}

	result, err := prompt.Render(tmpl, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "You are working on org/repo#42: Fix the flaky test.\nBranch: symphony/org_repo_42"
	if result != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
	}
}

func TestRender_AttemptNilForFirstRun(t *testing.T) {
	tmpl := `{{if .attempt}}Retry {{.attempt}}{{else}}First run{{end}}`

	data := prompt.RenderInput{
		WorkItem: map[string]any{},
		Attempt:  nil,
	}

	result, err := prompt.Render(tmpl, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "First run" {
		t.Errorf("expected 'First run', got %q", result)
	}
}

func TestRender_AttemptSetForRetry(t *testing.T) {
	tmpl := `{{if .attempt}}Retry {{.attempt}}{{else}}First run{{end}}`

	attempt := 3
	data := prompt.RenderInput{
		WorkItem: map[string]any{},
		Attempt:  &attempt,
	}

	result, err := prompt.Render(tmpl, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Retry 3" {
		t.Errorf("expected 'Retry 3', got %q", result)
	}
}

func TestRender_UnknownVariableFails(t *testing.T) {
	tmpl := `Hello {{.nonexistent_var}}`

	data := prompt.RenderInput{
		WorkItem: map[string]any{},
	}

	_, err := prompt.Render(tmpl, data)
	if err == nil {
		t.Fatal("expected error for unknown variable")
	}
}

func TestRender_EmptyTemplateFallback(t *testing.T) {
	result, err := prompt.Render("", prompt.RenderInput{
		WorkItem: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != prompt.DefaultPrompt {
		t.Errorf("expected default prompt, got %q", result)
	}
}
