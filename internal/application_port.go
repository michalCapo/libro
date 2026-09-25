package libro

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func killApplicationPort(port int) error {
	output, err := applicationPortOwners(port)
	if err != nil {
		return err
	}
	owners := make(map[int]bool)
	for field := range strings.FieldsSeq(string(output)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 1 || pid == os.Getpid() {
			return fmt.Errorf("refusing to kill port owner %q", field)
		}
		owners[pid] = true
	}
	for pid := range owners {
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		err = process.Kill()
		_ = process.Release()
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill process %d: %w", pid, err)
		}
	}
	return nil
}
