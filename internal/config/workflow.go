package config

import (
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// WorkflowDefinition is the parsed WORKFLOW.md payload.
type WorkflowDefinition struct {
	Config         map[string]any
	PromptTemplate string
}

// LoadWorkflow reads a WORKFLOW.md file and splits it into config (YAML front matter)
// and prompt template (Markdown body).
func LoadWorkflow(path string) (*WorkflowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &WorkflowError{
				Kind:    ErrMissingWorkflowFile,
				Message: path,
				Cause:   err,
			}
		}
		return nil, &WorkflowError{
			Kind:    ErrMissingWorkflowFile,
			Message: "cannot read " + path,
			Cause:   err,
		}
	}

	content := string(data)
	frontMatter, body := splitFrontMatter(content)

	wf := &WorkflowDefinition{
		Config:         make(map[string]any),
		PromptTemplate: strings.TrimSpace(body),
	}

	if frontMatter == "" {
		return wf, nil
	}

	var raw any
	if err := yaml.Unmarshal([]byte(frontMatter), &raw); err != nil {
		return nil, &WorkflowError{
			Kind:    ErrWorkflowParseError,
			Message: "invalid YAML front matter",
			Cause:   err,
		}
	}

	m, ok := raw.(map[string]any)
	if !ok {
		return nil, &WorkflowError{
			Kind:    ErrFrontMatterNotAMap,
			Message: "front matter must be a YAML map/object",
		}
	}

	wf.Config = m

	// Validate template syntax at parse time
	if wf.PromptTemplate != "" {
		if _, err := template.New("prompt").Option("missingkey=error").Parse(wf.PromptTemplate); err != nil {
			return nil, &WorkflowError{
				Kind:    ErrTemplateParseError,
				Message: "invalid prompt template",
				Cause:   err,
			}
		}
	}

	return wf, nil
}

// splitFrontMatter splits content into YAML front matter and body.
// Front matter is delimited by leading "---" lines.
func splitFrontMatter(content string) (frontMatter, body string) {
	if !strings.HasPrefix(content, "---") {
		return "", content
	}

	// Find the closing "---"
	rest := content[3:]
	// Skip the newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		// No closing delimiter — treat entire file as body
		return "", content
	}

	frontMatter = rest[:idx]
	remaining := rest[idx+4:] // skip "\n---"

	// Skip newline after closing ---
	if len(remaining) > 0 && remaining[0] == '\n' {
		remaining = remaining[1:]
	} else if len(remaining) > 1 && remaining[0] == '\r' && remaining[1] == '\n' {
		remaining = remaining[2:]
	}

	return frontMatter, remaining
}
