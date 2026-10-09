package libro

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
)

const childrenHelp = `Manage isolated QA agents in visible Libro child threads.
CLI: libro children '<JSON command>'; MCP: children with the same fields.
Actions: create (name, base, prompt), launch (id, command array, provider, model, thinking),
list, status (id), followup (id, prompt), interrupt (id), restart (id), cleanup (id).
Command defaults to ["pi","-p"]. Provider/model/thinking are optional flags.
Example launch: {"action":"launch","id":"<child ID>","provider":"openrouter","model":"deepseek/deepseek-v4.1-flash","thinking":"high"}.
Children require per-thread application mode. Libro creates and binds each worktree.
Bash startup credentials are loaded privately. Never put credentials in commands or prompts.
Status includes application assignment, recent redacted output, exit status, last activity and attempt number.
A running agent with no activity for five minutes is stalled; interrupt or restart to recover.
Followup restarts the agent with its saved task and added instructions, including in print mode.
Restart creates a fresh attempt log. Cleanup stops only owned resources and preserves a
worktree archive, Git bundle, attempt logs and screenshots in the returned results directory.
Cleanup is idempotent; retry after any interruption. Cleaned children remain listed.
`

type childCommand struct {
	Action   string   `json:"action"`
	ID       string   `json:"id,omitempty"`
	Name     string   `json:"name,omitempty"`
	Base     string   `json:"base,omitempty"`
	Prompt   string   `json:"prompt,omitempty"`
	Command  []string `json:"command,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Model    string   `json:"model,omitempty"`
	Thinking string   `json:"thinking,omitempty"`
}

type childRecord struct {
	ID      string                   `json:"id"`
	Name    string                   `json:"name"`
	Owner   string                   `json:"owner"`
	Project string                   `json:"project"`
	Root    string                   `json:"root"`
	Path    string                   `json:"path"`
	Branch  string                   `json:"branch"`
	Base    string                   `json:"base"`
	Prompt  string                   `json:"prompt"`
	Command []string                 `json:"command"`
	Browser string                   `json:"browser"`
	Results string                   `json:"results"`
	Agent   string                   `json:"agent"`
	Attempt int                      `json:"attempt"`
	Phase   string                   `json:"phase"`
	State   components.ManagedStatus `json:"state"`
}

var childControlMu sync.Mutex
var childExecutablePattern = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

func childrenTool() map[string]any {
	properties := map[string]any{}
	for _, key := range []string{"action", "id", "name", "base", "prompt", "provider", "model", "thinking"} {
		properties[key] = map[string]any{"type": "string"}
	}
	properties["action"] = map[string]any{"type": "string", "enum": []string{"create", "launch", "list", "status", "followup", "interrupt", "restart", "cleanup"}}
	properties["command"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return map[string]any{"name": "children", "description": childrenHelp, "inputSchema": map[string]any{"type": "object", "properties": properties, "required": []string{"action"}, "additionalProperties": false}}
}

// ChildrenCommand always uses the caller's Libro binding, never a target path.
func ChildrenCommand(command json.RawMessage) (json.RawMessage, error) {
	scope := os.Getenv("LIBRO_APPLICATION_PATH")
	if scope == "" {
		var err error
		scope, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	return scopedChildrenCommand(command, scope)
}
func scopedChildrenCommand(command json.RawMessage, scope string) (json.RawMessage, error) {
	var args childCommand
	decoder := json.NewDecoder(bytes.NewReader(command))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return nil, err
	}
	if scope == "" {
		return nil, errors.New("child workspace binding unavailable")
	}
	payload, err := json.Marshal(map[string]any{"action": "children", "project": scope, "command": args})
	if err != nil {
		return nil, err
	}
	return agentControlCommand(payload)
}
func RunChildrenCLI(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" {
		_, err := io.WriteString(out, childrenHelp)
		return err
	}
	if len(args) != 1 {
		return errors.New("expected one JSON command; see libro children --help")
	}
	result, err := ChildrenCommand(json.RawMessage(args[0]))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}

func saveChild(child *childRecord) error {
	data, err := json.Marshal(child)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "child:"+child.ID, string(data))
	return err
}
func loadChildren(owner string) ([]childRecord, error) {
	rows, err := db.Query(`SELECT value FROM settings WHERE key LIKE 'child:%' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	children := []childRecord{}
	for rows.Next() {
		var data string
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		var child childRecord
		if err = json.Unmarshal([]byte(data), &child); err != nil {
			return nil, err
		}
		if owner == "" || child.Owner == owner {
			children = append(children, child)
		}
	}
	return children, rows.Err()
}
func attachChild(sid string, child *childRecord) {
	if child.Phase == "cleaned" {
		return
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state := sm.states[sid]
	if state.thread(child.ID) == nil {
		state.Threads = append(state.Threads, Thread{Managed: true, ID: child.ID, Name: child.Name, Project: child.Project, Path: child.Path})
	}
}
func childView(sid string, child *childRecord) map[string]any {
	logs, truncated := tm.Logs(child.Agent)
	if logs == "" && child.Attempt > 0 {
		file, err := os.Open(filepath.Join(child.Results, fmt.Sprintf("attempt-%d.log", child.Attempt)))
		if err == nil {
			defer func() { _ = file.Close() }()
			if info, e := file.Stat(); e == nil && info.Size() > 64<<10 {
				_, _ = file.Seek(-(64 << 10), io.SeekEnd)
				truncated = true
			}
			data, _ := io.ReadAll(io.LimitReader(file, 64<<10))
			logs = string(data)
		}
	}
	_, application, _ := controlApplication(sid, child.Path, "status")
	return map[string]any{"application": application, "id": child.ID, "name": child.Name, "base": child.Base, "path": child.Path, "browserSession": child.Browser, "results": child.Results, "phase": child.Phase, "status": child.State.Status, "lastActivity": child.State.LastActivity, "exitCode": child.State.ExitCode, "attempt": child.Attempt, "output": logs, "truncated": truncated}
}
func refreshChild(child *childRecord) {
	if state, ok := tm.ManagedStatus(child.Agent, 5*time.Minute); ok {
		child.State = state
	} else {
		data, err := os.ReadFile(filepath.Join(child.Results, fmt.Sprintf("attempt-%d.log.json", child.Attempt)))
		if err == nil {
			_ = json.Unmarshal(data, &child.State)
		}
		if child.State.Status == "running" || child.State.Status == "waiting" || child.State.Status == "stalled" {
			// Never trust a saved PID after Libro restarts.
			child.State.Status = "failed"
		}
	}
}

func controlChildren(sid, scope string, args childCommand) (r.Result, any, error) {
	childControlMu.Lock()
	defer childControlMu.Unlock()
	owner := applicationPath(scope)
	workspace, _, err := applicationProject(sm.Get(sid), scope)
	if err != nil {
		return r.Result{}, nil, err
	}
	if args.Action == "create" {
		return createChild(sid, owner, workspace, args)
	}
	children, err := loadChildren(owner)
	if err != nil {
		return r.Result{}, nil, err
	}
	result := []map[string]any{}
	for i := range children {
		child := &children[i]
		refreshChild(child)
		attachChild(sid, child)
		if err = saveChild(child); err != nil {
			return r.Result{}, nil, err
		}
		if args.Action == "list" {
			result = append(result, childView(sid, child))
			continue
		}
		if child.ID != args.ID {
			continue
		}
		var js r.Result
		switch args.Action {
		case "status":
		case "launch":
			if tm.IsRunning(child.Agent) {
				return r.Result{}, nil, errors.New("agent is already running; use followup, interrupt or restart")
			}
			if len(args.Command) > 0 {
				child.Command = slices.Clone(args.Command)
			} else if len(child.Command) == 0 {
				child.Command = []string{"pi", "-p"}
			}
			for _, flag := range []struct{ name, value string }{{"provider", args.Provider}, {"model", args.Model}, {"thinking", args.Thinking}} {
				if flag.value != "" {
					child.Command = append(child.Command, "--"+flag.name, flag.value)
				}
			}
			js, err = launchChild(sid, child)
		case "restart":
			tm.InterruptManaged(child.Agent)
			refreshChild(child)
			js, err = launchChild(sid, child)
		case "followup":
			if strings.TrimSpace(args.Prompt) == "" {
				return r.Result{}, nil, errors.New("followup prompt is required")
			}
			tm.InterruptManaged(child.Agent)
			refreshChild(child)
			child.Prompt += "\n\nFollow-up instructions:\n" + args.Prompt
			js, err = launchChild(sid, child)
		case "interrupt":
			tm.InterruptManaged(child.Agent)
			refreshChild(child)
		case "cleanup":
			js, err = cleanupChild(sid, child)
		default:
			return r.Result{}, nil, errors.New("unknown child action")
		}
		if saveErr := saveChild(child); err == nil {
			err = saveErr
		}
		return r.Merge(js, projectsJS(sm.Get(sid))), childView(sid, child), err
	}
	if args.Action == "list" {
		return projectsJS(sm.Get(sid)), result, nil
	}
	return r.Result{}, nil, errors.New("child not found in this orchestrator's workspace")
}

func createChild(sid, owner, workspace string, args childCommand) (r.Result, any, error) {
	if strings.TrimSpace(args.Name) == "" || len(args.Name) > 200 || strings.TrimSpace(args.Prompt) == "" || args.Base == "" {
		return r.Result{}, nil, errors.New("name (up to 200 bytes), base branch and prompt are required")
	}
	root := applicationRoot(sm.Get(sid), workspace)
	if loadApplicationSettings(root).Mode != "thread" {
		return r.Result{}, nil, errors.New("choose per-thread application mode in project settings before creating isolated children")
	}
	branches, err := GitListBranches(root)
	if err != nil {
		return r.Result{}, nil, err
	}
	if !slices.Contains(branches, args.Base) {
		return r.Result{}, nil, errors.New("choose an existing local base branch")
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return r.Result{}, nil, err
	}
	token := hex.EncodeToString(random[:])
	dir, err := libroDataDir()
	if err != nil {
		return r.Result{}, nil, err
	}
	project := sm.Get(sid).projectScope(workspace)
	for _, p := range sm.Get(sid).Projects {
		if p.Path == root {
			project = p.Name
			break
		}
	}
	child := &childRecord{ID: "thread:" + token, Name: args.Name, Owner: owner, Project: project, Root: root, Path: filepath.Join(filepath.Dir(root), filepath.Base(root)+"-qa-"+token), Branch: "libro-qa-" + token, Base: args.Base, Prompt: args.Prompt, Browser: "libro-qa-" + token, Results: filepath.Join(dir, "children", token), Phase: "creating", State: components.ManagedStatus{Status: "waiting", LastActivity: time.Now().UTC()}}
	if err = os.MkdirAll(child.Results, 0o700); err != nil {
		return r.Result{}, nil, err
	}
	if err = saveChild(child); err != nil {
		return r.Result{}, nil, err
	}
	if _, err = worktreeGit(root, "worktree", "add", "-b", child.Branch, child.Path, "refs/heads/"+args.Base); err != nil {
		return r.Result{}, nil, err
	}
	child.Phase = "ready"
	if err = saveChild(child); err != nil {
		return r.Result{}, nil, err
	}
	attachChild(sid, child)
	return projectsJS(sm.Get(sid)), childView(sid, child), nil
}

func launchChild(sid string, child *childRecord) (r.Result, error) {
	generation := tm.Generation()
	if child.Phase != "ready" {
		return r.Result{}, errors.New("child is not ready; finish cleanup or create another child")
	}
	if tm.IsRunning(child.Agent) {
		return r.Result{}, errors.New("agent is already running; use followup, interrupt or restart")
	}
	if len(child.Command) == 0 {
		return r.Result{}, errors.New("launch the child with an agent command first")
	}
	// Executable and arguments are separate data, never arbitrary shell scripts.
	if !childExecutablePattern.MatchString(child.Command[0]) {
		return r.Result{}, errors.New("command must be an executable followed by separate arguments")
	}
	environment, err := components.BashEnvironment()
	if err != nil {
		return r.Result{}, err
	}
	var sensitive []string
	for name, value := range agentEnvironment() {
		environment = append(environment, name+"="+value)
		sensitive = append(sensitive, value)
	}
	environment = append(environment, "LIBRO_APPLICATION_PATH="+child.Path, "LIBRO_INSTANCE="+os.Getenv("LIBRO_INSTANCE"), "LIBRO_PORT="+os.Getenv("LIBRO_PORT"), "AGENT_BROWSER_SESSION="+child.Browser)
	prompt := child.Prompt + "\n\n" + components.AgentInstructions + "\nUse browser session " + child.Browser + " on every agent-browser command. Save results and screenshots inside this worktree before cleanup."
	command := child.Command[0]
	for _, arg := range append(slices.Clone(child.Command[1:]), prompt) {
		command = components.CommandWithFile(command, arg)
	}
	old := child.Agent
	child.Attempt++
	child.Agent = fmt.Sprintf("qa-%s-%d", strings.TrimPrefix(child.ID, "thread:"), child.Attempt)
	child.State = components.ManagedStatus{Status: "running", LastActivity: time.Now().UTC()}
	if err = saveChild(child); err != nil {
		return r.Result{}, err
	}
	logPath := filepath.Join(child.Results, fmt.Sprintf("attempt-%d.log", child.Attempt))
	if _, err = tm.StartManaged(generation, child.Agent, command, child.Path, logPath, environment, sensitive...); err != nil {
		child.State.Status = "failed"
		return r.Result{}, err
	}
	tm.Stop(old)
	sm.RemoveAppByID(sid, old)
	panel := Application{ID: child.Agent, TerminalID: child.Agent, Type: AppTypeTerminal, PluginID: filepath.Base(child.Command[0]), Dock: "center", Name: child.Name, Width: WidthFull, Writable: true, TerminalReady: true}
	sm.mu.Lock()
	state := sm.states[sid]
	index := 0
	if state.ActiveProject == child.ID {
		index = len(state.Apps)
		state.Apps = append(state.Apps, panel)
	} else {
		if state.snapshots == nil {
			state.snapshots = map[string]*projectSnapshot{}
		}
		if state.snapshots[child.ID] == nil {
			state.snapshots[child.ID] = &projectSnapshot{}
		}
		index = len(state.snapshots[child.ID].Apps)
		state.snapshots[child.ID].Apps = append(state.snapshots[child.ID].Apps, panel)
	}
	sm.mu.Unlock()
	// A ready panel attaches to its owned PTY when the child is selected.
	return r.Merge(removeAppJS(old), insertAppJS(renderAppFrame(panel, index, false, sid), false, child.ID)), nil
}

func registerChildControl(app *r.App) {
	registerWorkspaceAction(app, "children.control", func(_ *r.Context, in actionChildrenControlInput) (r.Result, error) {
		request := in.Request
		scope := in.Project
		js, result, err := controlChildren(inputSID(in.SID), scope, in.Command)
		reply := map[string]any{"result": result}
		if err != nil {
			reply["error"] = err.Error()
		}
		return r.Merge(js, clientScript("window.libroWorkspace.applicationResult(props[0],props[1]);", request, reply)), nil
	})
}

// Archive atomically before removing any Git resource. Retry never overwrites a
// completed archive with an empty worktree after partial cleanup.
func archiveChild(child *childRecord) error {
	target := filepath.Join(child.Results, "worktree.tar.gz")
	if fileExists(target) {
		return nil
	}
	file, err := os.OpenFile(target+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(file)
	archive := tar.NewWriter(gz)
	err = filepath.WalkDir(child.Path, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, e := filepath.Rel(child.Path, path)
		if e != nil {
			return e
		}
		if relative == "." {
			return nil
		}
		if relative == ".git" {
			return nil
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, e = os.Readlink(path)
			if e != nil {
				return e
			}
		}
		header, e := tar.FileInfoHeader(info, link)
		if e != nil {
			return e
		}
		header.Name = filepath.ToSlash(relative)
		if e = archive.WriteHeader(header); e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		source, e := os.Open(path)
		if e != nil {
			return e
		}
		_, e = io.Copy(archive, source)
		closeErr := source.Close()
		if e != nil {
			return e
		}
		return closeErr
	})
	err = errors.Join(err, archive.Close(), gz.Close(), file.Sync(), file.Close())
	if err != nil {
		return err
	}
	return os.Rename(target+".tmp", target)
}
func cleanupChild(sid string, child *childRecord) (r.Result, error) {
	if child.Phase == "cleaned" {
		return r.Result{}, nil
	}
	child.Phase = "stopping"
	if err := saveChild(child); err != nil {
		return r.Result{}, err
	}
	tm.InterruptManaged(child.Agent)
	refreshChild(child)
	var js r.Result

	// Never stop a shared application, even if settings changed after creation.
	if loadApplicationSettings(child.Root).Mode == "thread" {
		effects, _, err := controlApplication(sid, child.Path, "stop")
		if err != nil {
			return r.Result{}, err
		}
		js = js.Add(effects)
	}
	for _, workspace := range sm.GetAllRunningApps(sid) {
		if workspace.Name == child.ID {
			for _, panel := range workspace.Apps {
				if isSharedProjectApp(panel) {
					continue
				}
				tm.Stop(panel.ID)
				sm.RemoveAppByID(sid, panel.ID)
				js = js.Add(removeAppJS(panel.ID))
			}
		}
	}
	for attempt := 1; attempt <= child.Attempt; attempt++ {
		tm.Stop(fmt.Sprintf("qa-%s-%d", strings.TrimPrefix(child.ID, "thread:"), attempt))
	}
	// agent-browser owns its session; never use a PID or another session's port.
	environment, err := components.BashEnvironment()
	if err != nil {
		return js, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", "-c", `if command -v agent-browser >/dev/null 2>&1; then exec agent-browser --session "$1" close; fi`, "libro-child", child.Browser)
	cmd.Env = append(append(environment, agentEnvironmentList()...), "BASH_ENV=", "ENV=")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		return js, errors.New("could not close child browser; retry cleanup")
	}

	child.Phase = "preserving"
	if err := saveChild(child); err != nil {
		return js, err
	}
	trees, err := GitListWorktrees(child.Root)
	if err != nil {
		return js, err
	}
	exists := false
	for _, tree := range trees {
		if applicationPath(tree.Path) == applicationPath(child.Path) {
			if tree.Branch != child.Branch {
				return js, errors.New("child worktree branch changed; refusing cleanup")
			}
			exists = true
		}
	}
	if exists {
		if err = archiveChild(child); err != nil {
			return js, err
		}
		bundle := filepath.Join(child.Results, "commits.bundle")
		if !fileExists(bundle) {
			if _, err = worktreeGit(child.Root, "bundle", "create", bundle+".tmp", "refs/heads/"+child.Branch); err != nil {
				return js, err
			}
			if err = os.Rename(bundle+".tmp", bundle); err != nil {
				return js, err
			}
		}
	}
	child.Phase = "removing"
	if err = saveChild(child); err != nil {
		return js, err
	}
	if exists {
		if _, err = worktreeGit(child.Root, "worktree", "remove", "--force", child.Path); err != nil {
			return js, err
		}
	}
	branches, err := GitListBranches(child.Root)
	if err != nil {
		return js, err
	}
	if slices.Contains(branches, child.Branch) {
		if _, err = worktreeGit(child.Root, "branch", "-D", child.Branch); err != nil {
			return js, err
		}
	}
	if sm.Get(sid).ActiveProject == child.ID {
		js = js.Add(switchToProjectName(sid, child.Project))
	}
	sm.mu.Lock()
	state := sm.states[sid]
	// Keep any shared panels parked in this workspace reachable from Base.
	var shared []Application
	if snapshot := state.snapshots[child.ID]; snapshot != nil {
		shared = snapshot.Apps
	}
	if len(shared) > 0 {
		if state.ActiveProject == child.Project {
			state.Apps = append(state.Apps, shared...)
		} else {
			if state.snapshots[child.Project] == nil {
				state.snapshots[child.Project] = &projectSnapshot{}
			}
			state.snapshots[child.Project].Apps = append(state.snapshots[child.Project].Apps, shared...)
		}
	}
	state.Threads = slices.DeleteFunc(state.Threads, func(thread Thread) bool { return thread.ID == child.ID })
	delete(state.snapshots, child.ID)
	sm.mu.Unlock()
	js = js.Add(reparentProjectAppsJS(shared, child.Project))
	js = js.Add(r.Result{}.Run(r.Remove(projectMainID(child.ID))))
	child.Phase = "cleaned"
	return js, saveChild(child)
}

// Restore visible child threads after desktop restart without reviving stale PIDs.
func loadChildThreads(projects []Project) []Thread {
	if db == nil {
		return nil
	}
	children, err := loadChildren("")
	if err != nil {
		return nil
	}
	var threads []Thread
	for _, child := range children {
		if child.Phase == "cleaned" {
			continue
		}
		if slices.ContainsFunc(projects, func(project Project) bool {
			return project.Name == child.Project && applicationPath(project.Path) == applicationPath(child.Root)
		}) {
			threads = append(threads, Thread{Managed: true, ID: child.ID, Name: child.Name, Project: child.Project, Path: child.Path})
		}
	}
	return threads
}
