package components

import (
	"os"
	"os/exec"
	"strconv"
)

func killTerminalProcess(process *os.Process) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(process.Pid), "/T", "/F").Run()
	_ = process.Kill()
}
