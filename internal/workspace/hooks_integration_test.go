package workspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/workspace"
)

func TestManager_AfterCreateHook_Success(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	if runtime.GOOS == "windows" {
		t.Skip("hooks require bash")
	}

	root := t.TempDir()
	markerFile := filepath.Join(root, "hook_ran")

	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "repo_cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: false,
		Hooks: workspace.HooksConfig{
			AfterCreate: "touch " + markerFile,
			TimeoutMs:   5000,
		},
	})

	bareRepo := setupBareRepo(t, root)
	branch := getDefaultBranch(t, root, bareRepo)

	ws, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner: "org", Repo: "repo", IssueNumber: 1,
		CloneURL: bareRepo, BaseBranch: branch,
	})
	if err != nil {
		t.Fatalf("CreateForWorkItem failed: %v", err)
	}

	if !ws.CreatedNow {
		t.Error("expected CreatedNow=true")
	}

	// Verify hook ran
	if _, err := os.Stat(markerFile); err != nil {
		t.Error("after_create hook did not run — marker file missing")
	}
}

func TestManager_AfterCreateHook_Failure_IsFatal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	if runtime.GOOS == "windows" {
		t.Skip("hooks require bash")
	}

	root := t.TempDir()
	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "repo_cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: false,
		Hooks: workspace.HooksConfig{
			AfterCreate: "exit 1", // fail on purpose
			TimeoutMs:   5000,
		},
	})

	bareRepo := setupBareRepo(t, root)
	branch := getDefaultBranch(t, root, bareRepo)

	_, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner: "org", Repo: "repo", IssueNumber: 1,
		CloneURL: bareRepo, BaseBranch: branch,
	})
	if err == nil {
		t.Fatal("expected error when after_create hook fails")
	}
	if !strings.Contains(err.Error(), "after_create") {
		t.Errorf("error should mention after_create, got: %v", err)
	}

	// Workspace directory should have been cleaned up
	wsPath := filepath.Join(root, "worktrees", "org_repo_1")
	if _, err := os.Stat(wsPath); err == nil {
		t.Error("workspace should have been removed after after_create failure")
	}
}

func TestManager_PushBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	root := t.TempDir()
	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "repo_cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: false,
	})

	bareRepo := setupBareRepo(t, root)
	branch := getDefaultBranch(t, root, bareRepo)

	ws, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner: "org", Repo: "repo", IssueNumber: 7,
		CloneURL: bareRepo, BaseBranch: branch,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make a commit in the workspace
	os.WriteFile(filepath.Join(ws.Path, "fix.txt"), []byte("fixed"), 0644)
	runGit(t, ws.Path, "add", ".")
	runGit(t, ws.Path, "-c", "user.email=t@t", "-c", "user.name=T", "commit", "-m", "fix")

	// Push
	if err := mgr.PushBranch(ws.Path, "origin", ws.BranchName); err != nil {
		t.Fatalf("PushBranch failed: %v", err)
	}

	// Verify branch exists in bare repo
	out := runGitOutput(t, bareRepo, "branch", "--list", ws.BranchName)
	if !strings.Contains(out, "symphony/org_repo_7") {
		t.Errorf("pushed branch not found in remote, branches: %q", out)
	}
}

// Helper: create a bare repo with one commit
func setupBareRepo(t *testing.T, root string) string {
	t.Helper()
	bareRepo := filepath.Join(root, "bare.git")
	runGit(t, root, "init", "--bare", bareRepo)
	srcRepo := filepath.Join(root, "src")
	runGit(t, root, "clone", bareRepo, srcRepo)
	os.WriteFile(filepath.Join(srcRepo, "README.md"), []byte("init"), 0644)
	runGit(t, srcRepo, "add", ".")
	runGit(t, srcRepo, "-c", "user.email=t@t", "-c", "user.name=T", "commit", "-m", "init")
	branch := getDefaultBranch(t, root, srcRepo)
	runGit(t, srcRepo, "push", "origin", branch)
	return bareRepo
}

func getDefaultBranch(t *testing.T, _ string, repo string) string {
	t.Helper()
	return strings.TrimSpace(runGitOutput(t, repo, "branch", "--show-current"))
}
