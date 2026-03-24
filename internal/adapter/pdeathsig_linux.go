//go:build linux

package adapter

import (
	"os/exec"
	"syscall"
)

// setPdeathsig sets the parent-death signal on Linux so the child process
// is killed if the parent dies unexpectedly, preventing orphaned agent processes.
func setPdeathsig(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
	cmd.SysProcAttr.Setpgid = true
}
