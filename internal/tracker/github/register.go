package github

import (
	"fmt"

	"github.com/shivamstaq/github-symphony/internal/config"
	gh "github.com/shivamstaq/github-symphony/internal/github"
	"github.com/shivamstaq/github-symphony/internal/tracker"
)

func init() {
	tracker.Register("github", func(cfg *config.SymphonyConfig) (tracker.Tracker, error) {
		token := cfg.Auth.GitHub.Token
		if token == "" {
			return nil, fmt.Errorf("github token required: set auth.github.token or $GITHUB_TOKEN")
		}

		apiURL := cfg.Auth.GitHub.APIURL
		if apiURL == "" {
			apiURL = "https://api.github.com"
		}
		graphqlEndpoint := apiURL + "/graphql"

		client := gh.NewGraphQLClient(graphqlEndpoint, token)

		pageSize := 100
		ghSource := gh.NewSource(client, gh.SourceConfig{
			Owner:            cfg.Tracker.Owner,
			ProjectNumber:    cfg.Tracker.ProjectNumber,
			ProjectScope:     cfg.Tracker.ProjectScope,
			StatusFieldName:  cfg.Tracker.StatusFieldName,
			PageSize:         pageSize,
			PriorityValueMap: cfg.Tracker.PriorityValueMap,
		})

		return NewSource(client, ghSource, cfg.Tracker.PriorityValueMap, SourceConfig{
			Owner:           cfg.Tracker.Owner,
			ProjectNumber:   cfg.Tracker.ProjectNumber,
			ProjectScope:    cfg.Tracker.ProjectScope,
			StatusFieldName: cfg.Tracker.StatusFieldName,
		}), nil
	})
}
