//go:build !windows

package components

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func terminalHasChildren(pid int) bool {
	return exec.Command("pgrep", "-P", strconv.Itoa(pid)).Run() == nil
}

// killTerminalProcess includes jobs with their own process groups.
func killTerminalProcess(process *os.Process) {
	output, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err == nil {
		children := make(map[int][]int)
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			pid, pidErr := strconv.Atoi(fields[0])
			parent, parentErr := strconv.Atoi(fields[1])
			if pidErr == nil && parentErr == nil && pid > 0 {
				children[parent] = append(children[parent], pid)
			}
		}
		var killChildren func(int)
		killChildren = func(parent int) {
			for _, pid := range children[parent] {
				killChildren(pid)
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		killChildren(process.Pid)
	}
	// PTY shells are session leaders. Also stop remaining jobs in their group.
	_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
	_ = process.Kill()
}
