package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/config"
	"github.com/shivamstaq/github-symphony/internal/domain"
)

func setupPromptDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "prompts")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "default.md"), []byte("Default prompt for {{.work_item.title}}"), 0644)
	os.WriteFile(filepath.Join(dir, "bug_fix.md"), []byte("Fix the bug: {{.work_item.title}}"), 0644)
	os.WriteFile(filepath.Join(dir, "feature.md"), []byte("Implement feature: {{.work_item.title}}"), 0644)
	return dir
}

func TestRouter_DefaultTemplate(t *testing.T) {
	dir := setupPromptDir(t)
	r := NewRouter(config.PromptRoutingConfig{
		Default: "default.md",
	}, dir)

	item := domain.WorkItem{Title: "Test"}
	tmpl, err := r.SelectTemplate(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl, "Default prompt") {
		t.Errorf("expected default template, got %q", tmpl)
	}
}

func TestRouter_RouteByField(t *testing.T) {
	dir := setupPromptDir(t)
	r := NewRouter(config.PromptRoutingConfig{
		FieldName: "Type",
		Routes: map[string]string{
			"Bug":     "bug_fix.md",
			"Feature": "feature.md",
		},
		Default: "default.md",
	}, dir)

	// Bug route
	item := domain.WorkItem{
		Title:         "Fix login",
		ProjectFields: map[string]any{"Type": "Bug"},
	}
	tmpl, err := r.SelectTemplate(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl, "Fix the bug") {
		t.Errorf("expected bug template, got %q", tmpl)
	}

	// Feature route
	item.ProjectFields["Type"] = "Feature"
	tmpl, err = r.SelectTemplate(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl, "Implement feature") {
		t.Errorf("expected feature template, got %q", tmpl)
	}

	// Unknown value -> default
	item.ProjectFields["Type"] = "Unknown"
	tmpl, err = r.SelectTemplate(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl, "Default prompt") {
		t.Errorf("expected default for unknown type, got %q", tmpl)
	}
}

func TestRouter_CaseInsensitive(t *testing.T) {
	dir := setupPromptDir(t)
	r := NewRouter(config.PromptRoutingConfig{
		FieldName: "Type",
		Routes:    map[string]string{"bug": "bug_fix.md"},
		Default:   "default.md",
	}, dir)

	item := domain.WorkItem{
		Title:         "Test",
		ProjectFields: map[string]any{"Type": "BUG"},
	}
	tmpl, err := r.SelectTemplate(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl, "Fix the bug") {
		t.Errorf("expected case-insensitive match for bug, got %q", tmpl)
	}
}

func TestRouter_NoProjectFields(t *testing.T) {
	dir := setupPromptDir(t)
	r := NewRouter(config.PromptRoutingConfig{
		FieldName: "Type",
		Routes:    map[string]string{"bug": "bug_fix.md"},
		Default:   "default.md",
	}, dir)

	item := domain.WorkItem{Title: "Test"} // no ProjectFields
	tmpl, err := r.SelectTemplate(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl, "Default prompt") {
		t.Errorf("expected default when no fields, got %q", tmpl)
	}
}
