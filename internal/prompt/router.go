package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shivamstaq/github-symphony/internal/config"
	"github.com/shivamstaq/github-symphony/internal/domain"
)

// Router selects a prompt template based on a work item's custom field value.
type Router struct {
	cfg       config.PromptRoutingConfig
	promptDir string // directory containing prompt template files
}

// NewRouter creates a prompt router.
// promptDir is the absolute path to .symphony/prompts/.
func NewRouter(cfg config.PromptRoutingConfig, promptDir string) *Router {
	return &Router{cfg: cfg, promptDir: promptDir}
}

// SelectTemplate returns the prompt template content for the given work item.
// It checks the work item's ProjectFields for the configured routing field,
// matches the value against routes, and falls back to the default template.
func (r *Router) SelectTemplate(item domain.WorkItem) (string, error) {
	templateFile := r.cfg.Default

	if r.cfg.FieldName != "" && item.ProjectFields != nil {
		if val, ok := item.ProjectFields[r.cfg.FieldName]; ok {
			valStr := fmt.Sprintf("%v", val)
			for routeVal, tmplFile := range r.cfg.Routes {
				if strings.EqualFold(routeVal, valStr) {
					templateFile = tmplFile
					break
				}
			}
		}
	}

	path := filepath.Join(r.promptDir, templateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt template %q: %w", path, err)
	}
	return string(data), nil
}
