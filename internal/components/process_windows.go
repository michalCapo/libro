package components

import (
	"os"
	"os/exec"
	"strconv"
)

func terminalHasChildren(pid int) bool {
	output, err := exec.Command("powershell", "-NoProfile", "-Command", "Get-CimInstance Win32_Process -Filter 'ParentProcessId = "+strconv.Itoa(pid)+"' | Select-Object -First 1 -ExpandProperty ProcessId").Output()
	return err == nil && len(output) > 0
}

func killTerminalProcess(process *os.Process) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(process.Pid), "/T", "/F").Run()
	_ = process.Kill()
}

// setChatProcessGroup is not needed: taskkill /T stops the whole tree.
func setChatProcessGroup(*exec.Cmd) {}
