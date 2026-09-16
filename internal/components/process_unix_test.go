//go:build !windows

package components

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestKillTerminalProcessStopsChildJobs(t *testing.T) {
	cmd := exec.Command("bash", "-c", "set -m; sleep 60 & echo $!; wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	killTerminalProcess(cmd.Process)
	_ = cmd.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for {
		output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if err != nil || strings.TrimSpace(string(output)) == "" || strings.HasPrefix(strings.TrimSpace(string(output)), "Z") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d is still running: %s", pid, output)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
