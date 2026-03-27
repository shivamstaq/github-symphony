package opencode

import (
	"os/exec"
	"testing"
)

func TestNew_DefaultBinary(t *testing.T) {
	a := New(Config{})
	if a.cfg.Binary != "opencode" {
		t.Errorf("expected default binary 'opencode', got %q", a.cfg.Binary)
	}
}

func TestNew_CustomBinary(t *testing.T) {
	a := New(Config{Binary: "/usr/local/bin/opencode"})
	if a.cfg.Binary != "/usr/local/bin/opencode" {
		t.Errorf("expected custom binary, got %q", a.cfg.Binary)
	}
}

func TestNew_PreservesConfig(t *testing.T) {
	a := New(Config{
		Binary:     "oc",
		Model:      "gpt-4",
		ConfigFile: "/etc/opencode.yaml",
		LogDir:     "/tmp/logs",
		SocketDir:  "/tmp/sockets",
	})
	if a.cfg.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", a.cfg.Model)
	}
	if a.cfg.ConfigFile != "/etc/opencode.yaml" {
		t.Errorf("expected config file, got %q", a.cfg.ConfigFile)
	}
}

func TestCheckDependencies_BinaryNotOnPath(t *testing.T) {
	// Only run if opencode is NOT installed
	if _, err := exec.LookPath("opencode"); err == nil {
		t.Skip("opencode is installed, skipping not-on-path test")
	}

	err := CheckDependencies()
	if err == nil {
		t.Error("expected error when opencode not on PATH")
	}
}
