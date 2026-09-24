package libro

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// desktopCommand sends application and issue commands through the local bridge.
func desktopCommand(command json.RawMessage) (json.RawMessage, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(instanceDir(filepath.Join(dir, "libro")), "desktop-control-"+Port()+".json"))
	if err != nil {
		return nil, errors.New("desktop control unavailable; open Libro desktop first")
	}
	var connection struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	if err = json.Unmarshal(data, &connection); err != nil {
		return nil, errors.New("invalid Libro connection")
	}
	if connection.Port < 1 || connection.Port > 65535 || len(connection.Token) != 64 {
		return nil, errors.New("invalid Libro connection")
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/", connection.Port), bytes.NewReader(command))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+connection.Token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("desktop control disconnected; reopen Libro desktop")
	}
	defer func() { _ = response.Body.Close() }()
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(&reply); err != nil {
		return nil, err
	}
	if reply.Error != "" {
		return nil, errors.New(reply.Error)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("desktop returned HTTP %d", response.StatusCode)
	}
	return reply.Result, nil
}

// RunMCP exposes application and issue tools over stdio.
func RunMCP(in io.Reader, out io.Writer) error {
	applicationScope := os.Getenv("LIBRO_APPLICATION_PATH")
	if applicationScope == "" {
		applicationScope, _ = os.Getwd()
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			if err = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32700, "message": "Parse error"}}); err != nil {
				return err
			}
			continue
		}
		if len(request.ID) == 0 {
			continue
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "initialize":
			reply["result"] = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "libro", "version": "1.0.0"}, "instructions": applicationHelp + "\n" + issuesHelp}
		case "ping":
			reply["result"] = map[string]any{}
		case "tools/list":
			reply["result"] = map[string]any{"tools": []any{map[string]any{"name": "application", "description": applicationHelp, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"action": map[string]any{"type": "string", "enum": []string{"status", "start", "restart", "stop"}}}, "required": []string{"action"}, "additionalProperties": false}}, issuesTool()}}
		case "tools/call":
			var result json.RawMessage
			var err error
			switch request.Params.Name {
			case "issues":
				result, err = IssuesCommand(request.Params.Arguments)
			case "application":
				result, err = scopedApplicationCommand(request.Params.Arguments, applicationScope)
			default:
				err = errors.New("unknown tool")
			}
			content := map[string]any{"type": "text", "text": string(result)}
			if err != nil {
				content["text"] = err.Error()
			}
			reply["result"] = map[string]any{"content": []any{content}, "isError": err != nil}
		default:
			reply["error"] = map[string]any{"code": -32601, "message": "Method not found"}
		}
		if err := encoder.Encode(reply); err != nil {
			return err
		}
	}
	return scanner.Err()
}
