package codehost

import (
	"fmt"

	"github.com/shivamstaq/github-symphony/internal/config"
)

// NewCodeHost creates a CodeHost based on the config.
// Currently only GitHub is supported as a code host.
func NewCodeHost(cfg *config.SymphonyConfig) (CodeHost, error) {
	// CodeHost is always GitHub for now (even when tracker is Linear,
	// the code still lives on GitHub).
	token := cfg.Auth.GitHub.Token
	if token == "" {
		return nil, fmt.Errorf("github token required for code host: set auth.github.token")
	}

	apiURL := cfg.Auth.GitHub.APIURL
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}

	// Import cycle prevention: we can't import codehost/github here.
	// Instead, the caller (cmd/symphony/run.go) creates the GitHub host directly.
	// This factory exists for future extension when we have multiple code hosts.
	return nil, fmt.Errorf("use codehostgithub.New() directly — factory registration coming in a future version")
}
