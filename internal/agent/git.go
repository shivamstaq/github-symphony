package agent

import (
	"os/exec"
	"strings"
)

// HasNewCommits checks if there are new commits or uncommitted changes
// in the given workspace directory relative to the remote tracking branch.
func HasNewCommits(dir string) bool {
	if dir == "" {
		return false
	}
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0 || HasUnpushedCommits(dir)
}

// HasUnpushedCommits checks for commits ahead of the upstream tracking branch.
func HasUnpushedCommits(dir string) bool {
	if dir == "" {
		return false
	}
	cmd := exec.Command("git", "log", "--oneline", "@{u}..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// No upstream tracking — check if there are any commits
		cmd2 := exec.Command("git", "log", "--oneline", "-1")
		cmd2.Dir = dir
		out2, err2 := cmd2.Output()
		return err2 == nil && len(strings.TrimSpace(string(out2))) > 0
	}
	return len(strings.TrimSpace(string(out))) > 0
}
