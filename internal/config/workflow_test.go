package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/config"
)

func TestLoadWorkflow_WithFrontMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: github
  owner: myorg
  project_number: 42
agent:
  kind: claude_code
---
You are working on {{.work_item.title}}.

Fix the bug described in the issue.
`
	os.WriteFile(path, []byte(content), 0644)

	wf, err := config.LoadWorkflow(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Config should have tracker.kind
	tracker, ok := wf.Config["tracker"].(map[string]any)
	if !ok {
		t.Fatal("expected tracker to be a map")
	}
	if tracker["kind"] != "github" {
		t.Errorf("expected tracker.kind=github, got %v", tracker["kind"])
	}
	if tracker["owner"] != "myorg" {
		t.Errorf("expected tracker.owner=myorg, got %v", tracker["owner"])
	}

	// Prompt template should be trimmed body
	if wf.PromptTemplate == "" {
		t.Fatal("expected non-empty prompt template")
	}
	if wf.PromptTemplate[:15] != "You are working" {
		t.Errorf("unexpected prompt start: %q", wf.PromptTemplate[:15])
	}
}

func TestLoadWorkflow_NoFrontMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `Just a prompt with no front matter.`
	os.WriteFile(path, []byte(content), 0644)

	wf, err := config.LoadWorkflow(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(wf.Config) != 0 {
		t.Errorf("expected empty config, got %v", wf.Config)
	}
	if wf.PromptTemplate != "Just a prompt with no front matter." {
		t.Errorf("unexpected prompt: %q", wf.PromptTemplate)
	}
}

func TestLoadWorkflow_MissingFile(t *testing.T) {
	_, err := config.LoadWorkflow("/nonexistent/WORKFLOW.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}

	var wfErr *config.WorkflowError
	if !config.AsWorkflowError(err, &wfErr) {
		t.Fatalf("expected WorkflowError, got %T: %v", err, err)
	}
	if wfErr.Kind != config.ErrMissingWorkflowFile {
		t.Errorf("expected ErrMissingWorkflowFile, got %v", wfErr.Kind)
	}
}

func TestLoadWorkflow_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
this is: [not: valid: yaml
---
prompt body
`
	os.WriteFile(path, []byte(content), 0644)

	_, err := config.LoadWorkflow(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}

	var wfErr *config.WorkflowError
	if !config.AsWorkflowError(err, &wfErr) {
		t.Fatalf("expected WorkflowError, got %T: %v", err, err)
	}
	if wfErr.Kind != config.ErrWorkflowParseError {
		t.Errorf("expected ErrWorkflowParseError, got %v", wfErr.Kind)
	}
}

func TestLoadWorkflow_NonMapYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
- this
- is
- a list
---
prompt body
`
	os.WriteFile(path, []byte(content), 0644)

	_, err := config.LoadWorkflow(path)
	if err == nil {
		t.Fatal("expected error for non-map YAML")
	}

	var wfErr *config.WorkflowError
	if !config.AsWorkflowError(err, &wfErr) {
		t.Fatalf("expected WorkflowError, got %T: %v", err, err)
	}
	if wfErr.Kind != config.ErrFrontMatterNotAMap {
		t.Errorf("expected ErrFrontMatterNotAMap, got %v", wfErr.Kind)
	}
}
