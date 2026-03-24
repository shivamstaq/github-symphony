package github

import (
	"context"
	"fmt"
	"net/http"
)

// RepoRef identifies a repository for auth scoping.
type RepoRef struct {
	Owner string
	Name  string
}

// AuthProvider abstracts GitHub authentication.
// PAT implementation returns the same token for all repos.
// App implementation resolves installation tokens per repo.
type AuthProvider interface {
	Token(ctx context.Context, repo RepoRef) (string, error)
	HTTPClient(ctx context.Context, repo RepoRef) (*http.Client, error)
	Mode() string
}

// PATProvider implements AuthProvider using a Personal Access Token.
type PATProvider struct {
	token string
}

// NewPATProvider creates a PAT-based auth provider.
func NewPATProvider(token string) *PATProvider {
	return &PATProvider{token: token}
}

func (p *PATProvider) Token(_ context.Context, _ RepoRef) (string, error) {
	if p.token == "" {
		return "", fmt.Errorf("github_auth_error: PAT token is empty")
	}
	return p.token, nil
}

func (p *PATProvider) HTTPClient(ctx context.Context, repo RepoRef) (*http.Client, error) {
	token, err := p.Token(ctx, repo)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: &AuthTransport{
			Token: token,
			Base:  http.DefaultTransport,
		},
	}, nil
}

func (p *PATProvider) Mode() string {
	return "pat"
}

// AuthTransport adds Bearer token auth to HTTP requests.
type AuthTransport struct {
	Token string
	Base  http.RoundTripper
}

func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// AppProvider is a stub for GitHub App auth.
// It satisfies the AuthProvider interface but returns an error directing users to PAT mode.
type AppProvider struct{}

func NewAppProvider() *AppProvider {
	return &AppProvider{}
}

func (p *AppProvider) Token(_ context.Context, _ RepoRef) (string, error) {
	return "", fmt.Errorf("github_auth_error: GitHub App auth not yet implemented, use PAT mode (set GITHUB_TOKEN)")
}

func (p *AppProvider) HTTPClient(_ context.Context, _ RepoRef) (*http.Client, error) {
	return nil, fmt.Errorf("github_auth_error: GitHub App auth not yet implemented, use PAT mode")
}

func (p *AppProvider) Mode() string {
	return "app"
}
