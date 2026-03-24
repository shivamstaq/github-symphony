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

func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"myorg/myrepo#42", "myorg_myrepo_42"},
		{"org/repo#123", "org_repo_123"},
		{"simple", "simple"},
		{"with spaces and $pecial!", "with_spaces_and__pecial_"},
		{"UPPER.lower-dash_under", "UPPER.lower-dash_under"},
		{"", ""},
	}

	for _, tt := range tests {
		got := workspace.SanitizeKey(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeKey(%q) = %q, want %q", tt.input, got, tt.want)
		}
		for _, c := range got {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
				t.Errorf("SanitizeKey(%q) contains invalid char %q", tt.input, string(c))
			}
		}
	}
}

func TestWorkspaceKey(t *testing.T) {
	key := workspace.WorkspaceKey("myorg", "myrepo", 42)
	if key != "myorg_myrepo_42" {
		t.Errorf("expected myorg_myrepo_42, got %q", key)
	}
}

func TestBranchName(t *testing.T) {
	name := workspace.BranchName("symphony/", "myorg", "myrepo", 42)
	if name != "symphony/myorg_myrepo_42" {
		t.Errorf("expected symphony/myorg_myrepo_42, got %q", name)
	}
}

func TestPathContainment_Valid(t *testing.T) {
	root := "/tmp/workspaces"
	if err := workspace.ValidatePathContainment(root, "/tmp/workspaces/org_repo_42"); err != nil {
		t.Errorf("expected valid path, got: %v", err)
	}
}

func TestPathContainment_Escape(t *testing.T) {
	root := "/tmp/workspaces"
	if err := workspace.ValidatePathContainment(root, "/tmp/other/org_repo_42"); err == nil {
		t.Error("expected error for path outside root")
	}
}

func TestPathContainment_Traversal(t *testing.T) {
	root := "/tmp/workspaces"
	if err := workspace.ValidatePathContainment(root, "/tmp/workspaces/../other/bad"); err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestManager_CreateWorkspace_NewAndReuse(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	root := t.TempDir()
	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "repo_cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: false, // simple clone for test
	})

	// Create a bare repo to clone from
	bareRepo := filepath.Join(root, "bare.git")
	runGit(t, root, "init", "--bare", bareRepo)

	// Create a working copy, add a commit, push
	srcRepo := filepath.Join(root, "src")
	runGit(t, root, "clone", bareRepo, srcRepo)
	os.WriteFile(filepath.Join(srcRepo, "README.md"), []byte("hello"), 0644)
	runGit(t, srcRepo, "add", ".")
	runGit(t, srcRepo, "-c", "user.email=test@test.com", "-c", "user.name=Test", "commit", "-m", "init")
	// Get default branch name
	out := runGitOutput(t, srcRepo, "branch", "--show-current")
	defaultBranch := strings.TrimSpace(out)
	runGit(t, srcRepo, "push", "origin", defaultBranch)

	ws, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner:       "testorg",
		Repo:        "testrepo",
		IssueNumber: 1,
		CloneURL:    bareRepo,
		BaseBranch:  defaultBranch,
	})
	if err != nil {
		t.Fatalf("CreateForWorkItem failed: %v", err)
	}

	if _, err := os.Stat(ws.Path); err != nil {
		t.Fatalf("workspace path doesn't exist: %v", err)
	}

	if !strings.HasPrefix(ws.Path, filepath.Join(root, "worktrees")) {
		t.Errorf("workspace path %q not under worktree dir", ws.Path)
	}

	if ws.BranchName != "symphony/testorg_testrepo_1" {
		t.Errorf("expected branch symphony/testorg_testrepo_1, got %q", ws.BranchName)
	}

	if !ws.CreatedNow {
		t.Error("expected CreatedNow=true for new workspace")
	}

	// Second call should reuse
	ws2, err := mgr.CreateForWorkItem(context.Background(), workspace.WorkItemRef{
		Owner:       "testorg",
		Repo:        "testrepo",
		IssueNumber: 1,
		CloneURL:    bareRepo,
		BaseBranch:  defaultBranch,
	})
	if err != nil {
		t.Fatalf("second CreateForWorkItem failed: %v", err)
	}
	if ws2.CreatedNow {
		t.Error("expected CreatedNow=false for reused workspace")
	}
	if ws2.Path != ws.Path {
		t.Errorf("expected same path on reuse, got %q vs %q", ws2.Path, ws.Path)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}
