//go:build !windows

package libro

import (
	"os/exec"
	"syscall"
)

func prepareNavigationProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// The TypeScript server can spawn tsserver. Cancel the entire owned group.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
