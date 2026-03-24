package workspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/workspace"
)

func TestManager_Worktree_CreateAndReuse(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	root := t.TempDir()
	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "repo_cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: true,
	})

	// Setup: bare repo with one commit
	bareRepo := filepath.Join(root, "bare.git")
	runGit(t, root, "init", "--bare", bareRepo)
	srcRepo := filepath.Join(root, "src")
	runGit(t, root, "clone", bareRepo, srcRepo)
	os.WriteFile(filepath.Join(srcRepo, "file.txt"), []byte("data"), 0644)
	runGit(t, srcRepo, "add", ".")
	runGit(t, srcRepo, "-c", "user.email=t@t", "-c", "user.name=T", "commit", "-m", "init")
	defaultBranch := strings.TrimSpace(runGitOutput(t, srcRepo, "branch", "--show-current"))
	runGit(t, srcRepo, "push", "origin", defaultBranch)

	ws, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner:       "org",
		Repo:        "repo",
		IssueNumber: 99,
		CloneURL:    bareRepo,
		BaseBranch:  defaultBranch,
	})
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	if !ws.CreatedNow {
		t.Error("expected CreatedNow=true")
	}
	if !ws.CreatedFromCache {
		t.Error("expected CreatedFromCache=true for worktree mode")
	}

	// Verify repo cache was created
	cachePath := filepath.Join(root, "repo_cache", "org", "repo")
	if _, err := os.Stat(filepath.Join(cachePath, "HEAD")); err != nil {
		t.Fatalf("repo cache not created at %s", cachePath)
	}

	// Verify worktree has the file
	if _, err := os.Stat(filepath.Join(ws.Path, "file.txt")); err != nil {
		t.Error("worktree missing file.txt from repo")
	}

	// Verify we're on the right branch
	branchOut := strings.TrimSpace(runGitOutput(t, ws.Path, "branch", "--show-current"))
	if branchOut != "symphony/org_repo_99" {
		t.Errorf("expected branch symphony/org_repo_99, got %q", branchOut)
	}

	// Second call reuses
	ws2, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner:       "org",
		Repo:        "repo",
		IssueNumber: 99,
		CloneURL:    bareRepo,
		BaseBranch:  defaultBranch,
	})
	if err != nil {
		t.Fatalf("reuse failed: %v", err)
	}
	if ws2.CreatedNow {
		t.Error("expected CreatedNow=false on reuse")
	}
}

func TestManager_Worktree_MultiplIssues(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	root := t.TempDir()
	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "repo_cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: true,
	})

	bareRepo := filepath.Join(root, "bare.git")
	runGit(t, root, "init", "--bare", bareRepo)
	srcRepo := filepath.Join(root, "src")
	runGit(t, root, "clone", bareRepo, srcRepo)
	os.WriteFile(filepath.Join(srcRepo, "f.txt"), []byte("x"), 0644)
	runGit(t, srcRepo, "add", ".")
	runGit(t, srcRepo, "-c", "user.email=t@t", "-c", "user.name=T", "commit", "-m", "init")
	branch := strings.TrimSpace(runGitOutput(t, srcRepo, "branch", "--show-current"))
	runGit(t, srcRepo, "push", "origin", branch)

	// Create workspaces for two different issues
	ws1, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner: "org", Repo: "repo", IssueNumber: 1, CloneURL: bareRepo, BaseBranch: branch,
	})
	if err != nil {
		t.Fatalf("issue 1 failed: %v", err)
	}

	ws2, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner: "org", Repo: "repo", IssueNumber: 2, CloneURL: bareRepo, BaseBranch: branch,
	})
	if err != nil {
		t.Fatalf("issue 2 failed: %v", err)
	}

	// They should be different paths
	if ws1.Path == ws2.Path {
		t.Error("two issues should have different workspace paths")
	}

	// But share the same repo cache
	if ws1.RepoCachePath != ws2.RepoCachePath {
		t.Error("two issues from same repo should share cache")
	}
}
