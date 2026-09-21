package components

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCodexActivityAcrossPTYChunks(t *testing.T) {
	a := &agentActivity{kind: "codex"}
	var got []string
	for _, chunk := range []string{"Ready in ordinary output", "\x1b", "]0;Work", "ing\x07", "\x1b]2;Thinking\x1b", "\\", "\x1b]0;Ready\x07", "\x1b]2;Working | Fix sidebar\x07", "\x1b]2;Ready | Fix sidebar\x07", "\x1b]777;libro;exited\x07"} {
		a.output([]byte(chunk), func(status string) { got = append(got, status) })
	}
	if want := []string{"working", "working", "done", "working", "done", "exited"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
}

func TestAgentActivityStartupAndExit(t *testing.T) {
	s := &TerminalSession{activity: &agentActivity{kind: "codex"}}
	for _, step := range []struct{ input, want string }{
		{"idle", "idle"}, {"done", "idle"}, {"working", "working"},
		{"done", "done"}, {"working", "working"}, {"exited", "idle"},
		{"working", "idle"}, // A stale file read must not revive an exited agent.
	} {
		s.setAgentStatus(step.input)
		if s.agentStatus != step.want {
			t.Fatalf("after %s: got %s, want %s", step.input, s.agentStatus, step.want)
		}
	}
}

func TestAgentLaunchIntegration(t *testing.T) {
	for _, kind := range []string{"codex", "claude", "pi", "opencode"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("OPENCODE_CONFIG_CONTENT", `{"theme":"existing","plugin":["existing-plugin"]}`)
			command, activity, err := prepareAgentActivity(kind + " --help")
			if err != nil {
				t.Fatal(err)
			}
			defer activity.cleanup()
			if !strings.Contains(command, " --help") || activity.kind != kind {
				t.Fatalf("launch lost arguments or identity: %s", command)
			}
			if kind != "opencode" && !strings.Contains(command, map[string]string{"codex": "mcp_servers.libro_browser", "claude": "--mcp-config", "pi": "--append-system-prompt"}[kind]) {
				t.Fatal("browser discovery missing")
			}
			if kind == "codex" && !strings.Contains(command, `tui.terminal_title=["run-state","session-id","thread-name"]`) {
				t.Fatal("Codex titles must include the session ID for resume")
			}
			if kind == "opencode" {
				var config map[string]any
				if err := json.Unmarshal([]byte(strings.TrimPrefix(activity.env[0], "OPENCODE_CONFIG_CONTENT=")), &config); err != nil {
					t.Fatal(err)
				}
				if config["mcp"].(map[string]any)["libro_browser"] == nil {
					t.Fatal("browser MCP missing")
				}
				if config["theme"] != "existing" || len(config["plugin"].([]any)) != 2 {
					t.Fatal("existing config lost")
				}
			}
		})
	}
	command := "bash -l"
	if got, activity, err := prepareAgentActivity(command); got != command || activity != nil || err != nil {
		t.Fatal("ordinary terminal changed")
	}
}

func TestClaudeActivityHooks(t *testing.T) {
	_, activity, err := prepareAgentActivity("claude")
	if err != nil {
		t.Fatal(err)
	}
	defer activity.cleanup()
	data, err := os.ReadFile(filepath.Join(activity.dir, "claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Hooks       map[string][]struct{ Hooks []struct{ Command string } }
		Permissions struct{ Allow []string }
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Permissions.Allow, []string{"mcp__libro_browser__browser", "mcp__libro_browser__application"}) {
		t.Fatalf("browser permission = %v", config.Permissions.Allow)
	}
	for _, step := range []struct{ event, want string }{{"UserPromptSubmit", "working"}, {"Stop", "done"}, {"StopFailure", "error"}, {"SessionEnd", "idle"}, {"SessionStart", "idle"}} {
		if output, err := exec.Command("sh", "-c", config.Hooks[step.event][0].Hooks[0].Command).CombinedOutput(); err != nil {
			t.Fatalf("hook failed: %v: %s", err, output)
		}
		got, _ := os.ReadFile(activity.path)
		if string(got) != step.want {
			t.Fatalf("%s: %s, want %s", step.event, got, step.want)
		}
	}
}

func TestOllamaClaudeActivity(t *testing.T) {
	for _, test := range []struct{ command, prefix, suffix string }{
		{"ollama launch claude", "ollama launch claude --", ""},
		{"ollama launch claude --model kimi-k3:cloud", "ollama launch claude --model kimi-k3:cloud --", ""},
		{"/usr/bin/ollama launch claude --model 'some model' --yes -- --resume", "/usr/bin/ollama launch claude --model 'some model' --yes --", " --resume"},
		{"ollama launch claude -y --model=kimi-k3:cloud --", "ollama launch claude -y --model=kimi-k3:cloud --", ""},
	} {
		t.Run(test.command, func(t *testing.T) {
			command, activity, err := prepareAgentActivity(test.command)
			if err != nil || activity == nil {
				t.Fatalf("missing integration: %v", err)
			}
			defer activity.cleanup()
			executable, _ := os.Executable()
			mcp, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"libro_browser": map[string]any{"command": executable, "args": []string{"browser-mcp"}}}})
			want := agentExitCommand(test.prefix + " --settings " + shellQuote(filepath.Join(activity.dir, "claude.json")) + " --mcp-config " + shellQuote(string(mcp)) + test.suffix)
			if command != want || activity.kind != "claude" {
				t.Fatalf("launch = %q, want %q", command, want)
			}
			if _, err := os.Stat(filepath.Join(activity.dir, "claude.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, command := range []string{"ollama launch", "ollama launch claude --config", "ollama launch claude --restore", "ollama launch opencode", "ollama run claude"} {
		got, activity, err := prepareAgentActivity(command)
		if got != command || activity != nil || err != nil {
			activity.cleanup()
			t.Fatalf("unrelated launch changed: %s", command)
		}
	}
}

func TestPiAndOpenCodeLifecycle(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	for _, kind := range []string{"pi", "opencode"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("OPENCODE_CONFIG_CONTENT", "")
			_, activity, err := prepareAgentActivity(kind)
			if err != nil {
				t.Fatal(err)
			}
			defer activity.cleanup()
			script := `import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
const plugin = (await import(pathToFileURL(process.argv[1]))).default;
const check = expected => assert.equal(readFileSync(process.argv[2], 'utf8'), expected);
`
			if kind == "pi" {
				script += `const hooks = {};
let name;
plugin({on:(event, handler) => hooks[event] = handler, getSessionName:() => name, setSessionName:value => name = value});
const ctx = {sessionManager:{getSessionId:() => 'pi-session', getBranch:() => []}};
hooks.input({text:'Fix\n login', source:'interactive'});
assert.equal(name, 'Fix login');
hooks.input({text:'Second prompt', source:'interactive'});
assert.equal(name, 'Fix login');
name = undefined;
hooks.input({text:'Extension instructions', source:'extension'});
assert.equal(name, undefined);
hooks.session_start({reason:'resume'}, {sessionManager:{getBranch:() => [{type:'message', message:{role:'user', content:[{type:'image'}, {type:'text', text:'Restore title'}]}}]}});
assert.equal(name, 'Restore title');
hooks.agent_start({}); check('working');
hooks.agent_end({willRetry:true}); check('working');
hooks.agent_end({willRetry:false}); check('done');
hooks.session_start({reason:'new'}, ctx); check('idle');
assert.equal(JSON.parse(readFileSync(process.argv[2].replace(/status$/, 'session'), 'utf8')).session_id, 'pi-session');
hooks.agent_start({}); check('working');
hooks.session_shutdown({}); check('idle');`
			} else {
				script += `const hooks = await plugin();
await hooks.event({event:{type:'session.created',properties:{info:{id:'parent-session'}}}});
await hooks.event({event:{type:'session.created',properties:{info:{id:'child-session',parentID:'parent-session'}}}});
assert.equal(JSON.parse(readFileSync(process.argv[2].replace(/status$/, 'session'), 'utf8')).session_id, 'parent-session');
const send = (sessionID, type) => hooks.event({event:{type:'session.status',properties:{sessionID,status:{type}}}});
await send('a','busy'); check('working');
await send('b','busy'); check('working');
await send('a','idle'); check('working');
await send('b','retry'); check('working');
await send('b','idle'); check('done');
await send('a','busy'); check('working');
await hooks.event({event:{type:'session.error',properties:{sessionID:'a'}}}); check('error');
await send('a','idle'); check('error');`
			}
			cmd := exec.Command("node", "--input-type=module", "-e", script, filepath.Join(activity.dir, kind+".mjs"), activity.path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("lifecycle failed: %v: %s", err, output)
			}
		})
	}
}

func TestCodexActivityNewSession(t *testing.T) {
	for _, reset := range []string{"Starting", "Ready"} {
		t.Run(reset, func(t *testing.T) {
			s := &TerminalSession{activity: &agentActivity{kind: "codex"}}
			for _, step := range []struct{ title, want string }{
				{"Ready | old-thread", "idle"},
				{"Working | old-thread", "working"},
				{"Ready | old-thread", "done"},
				{reset, "idle"},
				{"Ready | new-thread", "idle"},
				{"Ready | new-thread", "idle"},
				{"Working | new-thread", "working"},
				{"Ready | new-thread", "done"},
			} {
				s.activity.output([]byte("\x1b]2;"+step.title+"\x07"), s.setAgentStatus)
				if s.agentStatus != step.want {
					t.Fatalf("after %s: got %s, want %s", step.title, s.agentStatus, step.want)
				}
			}
		})
	}
}

func TestResumeAgentCommand(t *testing.T) {
	for _, test := range []struct{ command, want string }{
		{"codex --model example", "codex resume 'session-123' --model example"},
		{"/usr/bin/claude --model example", "/usr/bin/claude --model example --resume 'session-123'"},
		{"pi --model example", "pi --model example --session 'session-123'"},
		{"opencode", "opencode --session 'session-123'"},
		{"ollama launch claude --model 'some model' --yes", "ollama launch claude --model 'some model' --yes -- --resume 'session-123'"},
		{"ollama launch claude -- --verbose", "ollama launch claude -- --verbose --resume 'session-123'"},
	} {
		if got := ResumeAgentCommand(test.command, "session-123"); got != test.want {
			t.Errorf("resume %q = %q, want %q", test.command, got, test.want)
		}
		if got := ResumeAgentCommand(test.command, ""); got != test.command {
			t.Errorf("new launch changed: %q", got)
		}
	}
	if got := ResumeAgentCommand("pi", "a'; echo injected"); got != `pi --session 'a'"'"'; echo injected'` {
		t.Fatalf("session argument not quoted: %s", got)
	}
}

func TestAgentSessionCapture(t *testing.T) {
	const id = "01a0c304-7225-78c3-b807-123456789abc"
	var got []string
	s := &TerminalSession{activity: &agentActivity{kind: "codex"}, reportSession: func(id string) { got = append(got, id) }}
	for _, chunk := range []string{"\x1b]0;Ready | 01a0c304-", "7225-78c3-b807-123456789abc | Fix login\x07", "\x1b]0;Working | " + id + " ⠋ | Fix login\x07"} {
		s.activity.output([]byte(chunk), s.setAgentStatus)
	}
	if !reflect.DeepEqual(got, []string{id}) {
		t.Fatalf("session reports = %v", got)
	}
	_, activity, err := prepareAgentActivity("claude")
	if err != nil {
		t.Fatal(err)
	}
	defer activity.cleanup()
	data, err := os.ReadFile(filepath.Join(activity.dir, "claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Hooks map[string][]struct{ Hooks []struct{ Command string } }
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", config.Hooks["SessionStart"][0].Hooks[0].Command)
	cmd.Stdin = strings.NewReader(`{"session_id":"claude-session"}`)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook: %v: %s", err, output)
	}
	s.activity = activity
	s.readAgentSession()
	if !reflect.DeepEqual(got, []string{id, "claude-session"}) {
		t.Fatalf("hook session reports = %v", got)
	}
}
