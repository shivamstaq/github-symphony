package linear

import (
	"fmt"

	"github.com/shivamstaq/github-symphony/internal/config"
	"github.com/shivamstaq/github-symphony/internal/tracker"
)

func init() {
	tracker.Register("linear", func(cfg *config.SymphonyConfig) (tracker.Tracker, error) {
		apiKey := cfg.Auth.Linear.APIKey
		if apiKey == "" {
			apiKey = cfg.Tracker.LinearAPIKey
		}
		if apiKey == "" {
			return nil, fmt.Errorf("linear API key required: set auth.linear.api_key or tracker.linear_api_key")
		}
		teamID := cfg.Tracker.LinearTeamID
		if teamID == "" {
			return nil, fmt.Errorf("linear team ID required: set tracker.linear_team_id")
		}
		return NewSource(apiKey, teamID), nil
	})
}
