package agent

import (
	"os"
	"testing"
)

func TestHasNewCommits_EmptyDir(t *testing.T) {
	if HasNewCommits("") {
		t.Error("empty dir should return false")
	}
}

func TestHasNewCommits_NonExistentDir(t *testing.T) {
	if HasNewCommits("/nonexistent/dir/that/does/not/exist") {
		t.Error("nonexistent dir should return false")
	}
}

func TestHasNewCommits_TempDirNoGit(t *testing.T) {
	dir, err := os.MkdirTemp("", "symphony-git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if HasNewCommits(dir) {
		t.Error("dir without git repo should return false")
	}
}

func TestHasUnpushedCommits_EmptyDir(t *testing.T) {
	if HasUnpushedCommits("") {
		t.Error("empty dir should return false")
	}
}

func TestHasUnpushedCommits_NonExistentDir(t *testing.T) {
	if HasUnpushedCommits("/nonexistent/dir/that/does/not/exist") {
		t.Error("nonexistent dir should return false")
	}
}
