package codex

import (
	"os/exec"
	"testing"
)

func TestNew_DefaultBinary(t *testing.T) {
	a := New(Config{})
	if a.cfg.Binary != "codex" {
		t.Errorf("expected default binary 'codex', got %q", a.cfg.Binary)
	}
}

func TestNew_DefaultApprovalPolicy(t *testing.T) {
	a := New(Config{})
	if a.cfg.ApprovalPolicy != "full-auto" {
		t.Errorf("expected default approval policy 'full-auto', got %q", a.cfg.ApprovalPolicy)
	}
}

func TestNew_CustomBinary(t *testing.T) {
	a := New(Config{Binary: "/usr/local/bin/codex"})
	if a.cfg.Binary != "/usr/local/bin/codex" {
		t.Errorf("expected custom binary, got %q", a.cfg.Binary)
	}
}

func TestNew_CustomApprovalPolicy(t *testing.T) {
	a := New(Config{ApprovalPolicy: "suggest"})
	if a.cfg.ApprovalPolicy != "suggest" {
		t.Errorf("expected 'suggest', got %q", a.cfg.ApprovalPolicy)
	}
}

func TestCheckDependencies_BinaryNotOnPath(t *testing.T) {
	// Only run if codex is NOT installed
	if _, err := exec.LookPath("codex"); err == nil {
		t.Skip("codex is installed, skipping not-on-path test")
	}

	err := CheckDependencies()
	if err == nil {
		t.Error("expected error when codex not on PATH")
	}
}
