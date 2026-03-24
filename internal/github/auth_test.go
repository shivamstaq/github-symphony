package github_test

import (
	"context"
	"net/http"
	"testing"

	ghub "github.com/shivamstaq/github-symphony/internal/github"
)

func TestPATProvider_ReturnsToken(t *testing.T) {
	provider := ghub.NewPATProvider("ghp_test_token_123")

	token, err := provider.Token(context.Background(), ghub.RepoRef{Owner: "org", Name: "repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ghp_test_token_123" {
		t.Errorf("expected ghp_test_token_123, got %q", token)
	}
}

func TestPATProvider_Mode(t *testing.T) {
	provider := ghub.NewPATProvider("ghp_test")
	if provider.Mode() != "pat" {
		t.Errorf("expected mode=pat, got %q", provider.Mode())
	}
}

func TestPATProvider_HTTPClient(t *testing.T) {
	provider := ghub.NewPATProvider("ghp_test")

	client, err := provider.HTTPClient(context.Background(), ghub.RepoRef{Owner: "org", Name: "repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil HTTP client")
	}

	// Verify the client's transport adds the auth header
	transport, ok := client.Transport.(*ghub.AuthTransport)
	if !ok {
		t.Fatalf("expected AuthTransport, got %T", client.Transport)
	}

	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	transport.RoundTrip(req) // ignore error, just check header was set
	if req.Header.Get("Authorization") != "Bearer ghp_test" {
		t.Errorf("expected Bearer auth header, got %q", req.Header.Get("Authorization"))
	}
}

func TestPATProvider_EmptyTokenReturnsError(t *testing.T) {
	provider := ghub.NewPATProvider("")

	_, err := provider.Token(context.Background(), ghub.RepoRef{})
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}
