//go:build !windows

package libro

import (
	"errors"
	"os/exec"
	"strconv"
)

func applicationPortOwners(port int) ([]byte, error) {
	output, err := exec.Command("lsof", "-nP", "-t", "-a", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN").Output()
	var exit *exec.ExitError
	// lsof exits with 1 when the listener has already gone away.
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(output) == 0 && len(exit.Stderr) == 0 {
		err = nil
	}
	return output, err
}
