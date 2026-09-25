package libro

import (
	"os/exec"
	"strconv"
)

func applicationPortOwners(port int) ([]byte, error) {
	return exec.Command("powershell", "-NoProfile", "-Command", "$ErrorActionPreference = 'Stop'; Get-NetTCPConnection -State Listen | Where-Object LocalPort -eq "+strconv.Itoa(port)+" | Select-Object -ExpandProperty OwningProcess -Unique").Output()
}
