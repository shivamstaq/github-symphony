package github

import "fmt"

// GitHubErrorKind identifies the category of GitHub error.
type GitHubErrorKind string

const (
	ErrUnsupportedTrackerKind         GitHubErrorKind = "unsupported_tracker_kind"
	ErrMissingGitHubCredentials       GitHubErrorKind = "missing_github_credentials"
	ErrMissingGitHubAppID             GitHubErrorKind = "missing_github_app_id"
	ErrMissingGitHubPrivateKey        GitHubErrorKind = "missing_github_private_key"
	ErrMissingGitHubToken             GitHubErrorKind = "missing_github_token"
	ErrGitHubAuthError                GitHubErrorKind = "github_auth_error"
	ErrGitHubPATInsufficientPerms     GitHubErrorKind = "github_pat_insufficient_permissions"
	ErrGitHubInstallationResolution   GitHubErrorKind = "github_installation_resolution_error"
	ErrGitHubAPIRequest               GitHubErrorKind = "github_api_request"
	ErrGitHubAPIStatus                GitHubErrorKind = "github_api_status"
	ErrGitHubAPIRateLimited           GitHubErrorKind = "github_api_rate_limited"
	ErrGitHubGraphQLErrors            GitHubErrorKind = "github_graphql_errors"
	ErrGitHubUnknownPayload           GitHubErrorKind = "github_unknown_payload"
	ErrGitHubWebhookSignatureInvalid  GitHubErrorKind = "github_webhook_signature_invalid"
	ErrGitHubRepoNotAllowed           GitHubErrorKind = "github_repo_not_allowed"
)

// GitHubError is a typed error for GitHub operations.
type GitHubError struct {
	Kind    GitHubErrorKind
	Message string
	Cause   error
}

func (e *GitHubError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *GitHubError) Unwrap() error {
	return e.Cause
}
