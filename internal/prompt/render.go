package prompt

import (
	"bytes"
	"strings"
	"text/template"
)

// DefaultPrompt is used when the workflow prompt body is empty.
const DefaultPrompt = "You are working on a GitHub issue from a GitHub Project."

// RenderInput contains the variables available to the prompt template.
type RenderInput struct {
	WorkItem      map[string]any
	Issue         map[string]any
	Repository    map[string]any
	Attempt       *int
	BranchName    string
	BaseBranch    string
	ProjectFields map[string]any
}

// Render executes a prompt template with the given input.
// An empty template returns the default prompt.
// Unknown variables cause an error (missingkey=error).
func Render(tmplStr string, input RenderInput) (string, error) {
	if strings.TrimSpace(tmplStr) == "" {
		return DefaultPrompt, nil
	}

	data := map[string]any{
		"work_item":      input.WorkItem,
		"issue":          input.Issue,
		"repository":     input.Repository,
		"attempt":        input.Attempt,
		"branch_name":    input.BranchName,
		"base_branch":    input.BaseBranch,
		"project_fields": input.ProjectFields,
	}

	tmpl, err := template.New("prompt").Option("missingkey=error").Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
