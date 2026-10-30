package libro

import (
	"encoding/json"
	"fmt"
	"io"
)

const restartHelp = `Restart the Libro desktop app, for example after installing a new Libro build.
It restarts Libro itself, not the project application; use the application tool for that.
Every terminal and agent in Libro stops, including the caller, so finish and report your work first.
It works only while the Libro desktop window is open.
CLI: libro restart.
`

func restartTool() map[string]any {
	return map[string]any{"name": "restart", "description": restartHelp, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}
}

// RunRestartCLI restarts Libro for agents without MCP support.
func RunRestartCLI(args []string, out io.Writer) error {
	if len(args) > 0 {
		_, err := io.WriteString(out, restartHelp)
		return err
	}
	result, err := agentControlCommand(json.RawMessage(`{"action":"restart"}`))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}
