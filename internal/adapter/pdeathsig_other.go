//go:build !linux

package adapter

import "os/exec"

// setPdeathsig is a no-op on non-Linux platforms.
func setPdeathsig(_ *exec.Cmd) {}
