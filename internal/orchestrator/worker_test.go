package orchestrator_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/adapter"
	"github.com/shivamstaq/github-symphony/internal/orchestrator"
	"github.com/shivamstaq/github-symphony/internal/workspace"
)

const workerMockAgent = `
while IFS= read -r line; do
  id=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('id',0))" 2>/dev/null || echo "0")
  method=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('method',''))" 2>/dev/null || echo "")
  case "$method" in
    initialize) echo "{\"id\":${id},\"result\":{\"protocolVersion\":1,\"provider\":\"test\",\"capabilities\":{}}}" ;;
    session/new) echo "{\"id\":${id},\"result\":{\"sessionId\":\"s1\"}}" ;;
    session/prompt) echo "{\"id\":${id},\"result\":{\"stopReason\":\"completed\",\"summary\":\"done\"}}" ;;
    session/cancel) echo "{\"id\":${id},\"result\":{\"cancelled\":true}}" ;;
    session/close) echo "{\"id\":${id},\"result\":{\"closed\":true}}" ;;
    *) echo "{\"id\":${id},\"error\":{\"code\":-1,\"message\":\"unknown\"}}" ;;
  esac
done
`

func setupTestRepo(t *testing.T) (string, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	root := t.TempDir()
	bareRepo := filepath.Join(root, "bare.git")
	runGitCmd(t, root, "init", "--bare", bareRepo)
	srcRepo := filepath.Join(root, "src")
	runGitCmd(t, root, "clone", bareRepo, srcRepo)
	os.WriteFile(filepath.Join(srcRepo, "README.md"), []byte("init"), 0644)
	runGitCmd(t, srcRepo, "add", ".")
	runGitCmd(t, srcRepo, "-c", "user.email=t@t", "-c", "user.name=T", "commit", "-m", "init")
	branch := strings.TrimSpace(runGitOutputCmd(t, srcRepo, "branch", "--show-current"))
	runGitCmd(t, srcRepo, "push", "origin", branch)
	return root, bareRepo, branch
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func runGitOutputCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestWorker_BasicRun_CompletesNormally(t *testing.T) {
	root, bareRepo, branch := setupTestRepo(t)

	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: false,
	})

	runner := orchestrator.NewRunner(orchestrator.WorkerDeps{
		WorkspaceManager: mgr,
		AdapterFactory: func(cwd string) (adapter.AdapterClient, error) {
			return adapter.NewAdapter(adapter.AdapterConfig{
				Kind:    "opencode",
				Command: "bash",
				Args:    []string{"-c", workerMockAgent},
				Cwd:     cwd,
			})
		},
		PromptTemplate: "Fix the issue: {{.work_item.title}}",
		MaxTurns:       2,
	})

	num := 1
	item := orchestrator.WorkItem{
		WorkItemID:      "github:p1:i1",
		ProjectItemID:   "p1",
		IssueID:         "i1",
		IssueNumber:     &num,
		IssueIdentifier: "org/repo#1",
		ContentType:     "issue",
		Title:           "Fix flaky test",
		State:           "open",
		ProjectStatus:   "Todo",
		Repository: &orchestrator.Repository{
			Owner:         "org",
			Name:          "repo",
			FullName:      "org/repo",
			DefaultBranch: branch,
			CloneURLHTTPS: bareRepo,
		},
	}

	result := runner.Run(context.Background(), item, nil)

	if result.Outcome != orchestrator.OutcomeNormal {
		t.Errorf("expected OutcomeNormal, got %q (error: %v)", result.Outcome, result.Error)
	}
}

func TestWorker_MaxTurns_Enforced(t *testing.T) {
	root, bareRepo, branch := setupTestRepo(t)

	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: false,
	})

	turnCount := 0
	runner := orchestrator.NewRunner(orchestrator.WorkerDeps{
		WorkspaceManager: mgr,
		AdapterFactory: func(cwd string) (adapter.AdapterClient, error) {
			return adapter.NewAdapter(adapter.AdapterConfig{
				Kind:    "opencode", // use generic subprocess adapter for testing
				Command: "bash",
				Args:    []string{"-c", workerMockAgent},
				Cwd:     cwd,
			})
		},
		Source: &countingSourceForWorker{onFetch: func() { turnCount++ }},
		PromptTemplate: "Work on: {{.work_item.title}}",
		MaxTurns:       3,
	})

	num := 2
	item := orchestrator.WorkItem{
		WorkItemID: "github:p2:i2", ProjectItemID: "p2", IssueID: "i2",
		IssueNumber: &num, IssueIdentifier: "org/repo#2",
		ContentType: "issue", Title: "Multi-turn test", State: "open",
		ProjectStatus: "Todo",
		Repository: &orchestrator.Repository{
			Owner: "org", Name: "repo", FullName: "org/repo",
			DefaultBranch: branch, CloneURLHTTPS: bareRepo,
		},
	}

	result := runner.Run(context.Background(), item, nil)

	if result.Outcome != orchestrator.OutcomeNormal {
		t.Errorf("expected OutcomeNormal, got %q (error: %v)", result.Outcome, result.Error)
	}
	// Should have done 3 turns (max), with FetchStates called between turns 1-2 and 2-3
	if turnCount < 2 {
		t.Errorf("expected at least 2 state refreshes between turns, got %d", turnCount)
	}
}

func TestWorker_BeforeRunHook_Failure_Aborts(t *testing.T) {
	root, bareRepo, branch := setupTestRepo(t)

	mgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  filepath.Join(root, "worktrees"),
		RepoCacheDir: filepath.Join(root, "cache"),
		BranchPrefix: "symphony/",
		UseWorktrees: false,
	})

	runner := orchestrator.NewRunner(orchestrator.WorkerDeps{
		WorkspaceManager: mgr,
		AdapterFactory: func(cwd string) (adapter.AdapterClient, error) {
			t.Error("adapter should not be created when before_run fails")
			return nil, nil
		},
		PromptTemplate: "should not render",
		MaxTurns:       1,
		HooksBefore:    "exit 1", // fail
		HooksTimeoutMs: 5000,
	})

	num := 3
	item := orchestrator.WorkItem{
		WorkItemID: "github:p3:i3", ProjectItemID: "p3",
		IssueNumber: &num, IssueIdentifier: "org/repo#3",
		ContentType: "issue", Title: "Hook fail test", State: "open",
		ProjectStatus: "Todo",
		Repository: &orchestrator.Repository{
			Owner: "org", Name: "repo", FullName: "org/repo",
			DefaultBranch: branch, CloneURLHTTPS: bareRepo,
		},
	}

	result := runner.Run(context.Background(), item, nil)

	if result.Outcome != orchestrator.OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %q", result.Outcome)
	}
}

// countingSourceForWorker counts FetchStates calls and always returns the item as active.
type countingSourceForWorker struct {
	onFetch func()
}

func (s *countingSourceForWorker) FetchCandidates(_ context.Context) ([]orchestrator.WorkItem, error) {
	return nil, nil
}

func (s *countingSourceForWorker) FetchStates(_ context.Context, ids []string) ([]orchestrator.WorkItem, error) {
	if s.onFetch != nil {
		s.onFetch()
	}
	// Return items as still active
	var items []orchestrator.WorkItem
	for _, id := range ids {
		items = append(items, orchestrator.WorkItem{
			WorkItemID:    id,
			State:         "open",
			ProjectStatus: "Todo",
		})
	}
	return items, nil
}
