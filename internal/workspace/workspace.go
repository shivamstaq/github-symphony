package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var sanitizeRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// SanitizeKey replaces any character not in [A-Za-z0-9._-] with underscore.
func SanitizeKey(s string) string {
	return sanitizeRe.ReplaceAllString(s, "_")
}

// WorkspaceKey produces a deterministic workspace key from repo + issue.
func WorkspaceKey(owner, repo string, issueNumber int) string {
	return SanitizeKey(owner + "/" + repo + "#" + strconv.Itoa(issueNumber))
}

// BranchName produces the deterministic branch name.
func BranchName(prefix, owner, repo string, issueNumber int) string {
	return prefix + WorkspaceKey(owner, repo, issueNumber)
}

// ValidatePathContainment checks that child is under root after resolving symlinks/traversals.
func ValidatePathContainment(root, child string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("invalid_workspace_cwd: cannot resolve root: %w", err)
	}
	absChild, err := filepath.Abs(child)
	if err != nil {
		return fmt.Errorf("invalid_workspace_cwd: cannot resolve child: %w", err)
	}

	// Ensure child starts with root + separator
	if !strings.HasPrefix(absChild, absRoot+string(filepath.Separator)) && absChild != absRoot {
		return fmt.Errorf("invalid_workspace_cwd: path %q is not under root %q", absChild, absRoot)
	}
	return nil
}

// WorkItemRef contains the info needed to create a workspace for a work item.
type WorkItemRef struct {
	Owner       string
	Repo        string
	IssueNumber int
	CloneURL    string
	BaseBranch  string
}

// Workspace represents a created workspace directory.
type Workspace struct {
	Path           string
	WorkspaceKey   string
	RepoCachePath  string
	BranchName     string
	BaseBranch     string
	CreatedNow     bool
	CreatedFromCache bool
}

// ManagerConfig configures the workspace manager.
type ManagerConfig struct {
	WorktreeDir  string
	RepoCacheDir string
	BranchPrefix string
	UseWorktrees bool
	FetchDepth   int
}

// Manager handles workspace creation, reuse, and cleanup.
type Manager struct {
	cfg ManagerConfig
}

// NewManager creates a workspace manager.
func NewManager(cfg ManagerConfig) *Manager {
	return &Manager{cfg: cfg}
}

// CreateForWorkItem creates or reuses a workspace for the given work item.
func (m *Manager) CreateForWorkItem(_ context.Context, ref WorkItemRef) (*Workspace, error) {
	key := WorkspaceKey(ref.Owner, ref.Repo, ref.IssueNumber)
	branch := BranchName(m.cfg.BranchPrefix, ref.Owner, ref.Repo, ref.IssueNumber)
	wsPath := filepath.Join(m.cfg.WorktreeDir, key)

	// Validate path containment
	if err := ValidatePathContainment(m.cfg.WorktreeDir, wsPath); err != nil {
		return nil, err
	}

	// Check if workspace already exists
	if info, err := os.Stat(wsPath); err == nil && info.IsDir() {
		slog.Info("reusing existing workspace", "path", wsPath, "key", key)
		// Fetch latest changes
		m.gitFetch(wsPath)
		return &Workspace{
			Path:         wsPath,
			WorkspaceKey: key,
			BranchName:   branch,
			BaseBranch:   ref.BaseBranch,
			CreatedNow:   false,
		}, nil
	}

	// Ensure directories exist
	if err := os.MkdirAll(m.cfg.WorktreeDir, 0755); err != nil {
		return nil, fmt.Errorf("workspace creation: mkdir worktree dir: %w", err)
	}
	if err := os.MkdirAll(m.cfg.RepoCacheDir, 0755); err != nil {
		return nil, fmt.Errorf("workspace creation: mkdir repo cache dir: %w", err)
	}

	if m.cfg.UseWorktrees {
		return m.createWithWorktree(ref, key, branch, wsPath)
	}
	return m.createWithClone(ref, key, branch, wsPath)
}

func (m *Manager) createWithClone(ref WorkItemRef, key, branch, wsPath string) (*Workspace, error) {
	slog.Info("cloning repository for workspace", "url", ref.CloneURL, "path", wsPath, "branch", branch)

	args := []string{"clone"}
	if m.cfg.FetchDepth > 0 {
		args = append(args, "--depth", strconv.Itoa(m.cfg.FetchDepth))
	}
	args = append(args, "--branch", ref.BaseBranch, ref.CloneURL, wsPath)

	if err := m.runGit("", args...); err != nil {
		return nil, fmt.Errorf("workspace creation: clone: %w", err)
	}

	// Create and checkout the work branch
	if err := m.runGit(wsPath, "checkout", "-B", branch); err != nil {
		return nil, fmt.Errorf("workspace creation: checkout branch: %w", err)
	}

	return &Workspace{
		Path:         wsPath,
		WorkspaceKey: key,
		BranchName:   branch,
		BaseBranch:   ref.BaseBranch,
		CreatedNow:   true,
	}, nil
}

func (m *Manager) createWithWorktree(ref WorkItemRef, key, branch, wsPath string) (*Workspace, error) {
	cachePath := filepath.Join(m.cfg.RepoCacheDir, SanitizeKey(ref.Owner), SanitizeKey(ref.Repo))

	// Ensure repo cache exists
	if _, err := os.Stat(filepath.Join(cachePath, "HEAD")); os.IsNotExist(err) {
		slog.Info("cloning repo cache", "url", ref.CloneURL, "path", cachePath)
		if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
			return nil, fmt.Errorf("workspace creation: mkdir cache: %w", err)
		}
		args := []string{"clone", "--bare"}
		if m.cfg.FetchDepth > 0 {
			args = append(args, "--depth", strconv.Itoa(m.cfg.FetchDepth))
		}
		args = append(args, ref.CloneURL, cachePath)
		if err := m.runGit("", args...); err != nil {
			return nil, fmt.Errorf("workspace creation: clone cache: %w", err)
		}
	} else {
		slog.Info("fetching repo cache", "path", cachePath)
		m.gitFetch(cachePath)
	}

	// Create worktree
	slog.Info("creating worktree", "cache", cachePath, "path", wsPath, "branch", branch)
	if err := m.runGit(cachePath, "worktree", "add", "-B", branch, wsPath, ref.BaseBranch); err != nil {
		return nil, fmt.Errorf("workspace creation: worktree add: %w", err)
	}

	return &Workspace{
		Path:           wsPath,
		WorkspaceKey:   key,
		RepoCachePath:  cachePath,
		BranchName:     branch,
		BaseBranch:     ref.BaseBranch,
		CreatedNow:     true,
		CreatedFromCache: true,
	}, nil
}

func (m *Manager) gitFetch(dir string) {
	if err := m.runGit(dir, "fetch", "--all"); err != nil {
		slog.Warn("git fetch failed", "dir", dir, "error", err)
	}
}

func (m *Manager) runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

// RemoveWorkspace removes a workspace directory.
func (m *Manager) RemoveWorkspace(wsPath string) error {
	if err := ValidatePathContainment(m.cfg.WorktreeDir, wsPath); err != nil {
		return err
	}
	slog.Info("removing workspace", "path", wsPath)
	return os.RemoveAll(wsPath)
}
