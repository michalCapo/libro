package libro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	r "github.com/michalCapo/g-sui/ui"
)

const threadHelp = `Finish your own Libro thread: merge or squash it into its destination branch, close it, or discard it.
Use this tool whenever the user asks to merge, squash, finish, close or discard this thread. Do not run git merge,
git worktree remove or git branch -D yourself for that; Libro also closes the thread's panels and updates the sidebar.
CLI: libro thread '<JSON command>'; MCP: thread with the same fields.
Actions: preview (base), close, merge (base, sourceHead, targetHead, keepOpen), squash (base, sourceHead, targetHead,
message, keepOpen), cleanup (sourceHead), discard (sourceHead).
Close only closes the thread's panels, including this agent. Files, branch and worktree stay; the user can reopen it.
To merge or squash:
1. Commit your work first. Merge and squash need a clean thread.
2. Call preview. It returns the branch, destination base, commits and diff summary, sourceHead, targetHead and a blocker.
   Base defaults to the branch the thread was created from. Pass base only when the user names another branch.
3. If blocker is set, fix it or report it to the user. Then call merge or squash with the sourceHead and
   targetHead from the latest preview. Merge is the default when the user does not say squash.
To keep working after the merge, for example to push, install or release from the destination, pass keepOpen: true.
The thread and this agent stay open, and the result names the destination checkout (targetPath). Work there, then
call preview and cleanup with its sourceHead to remove the thread. Cleanup refuses a thread with commits made after
that merge; merge again first.
To discard, do not commit. Call preview, then discard with its sourceHead. The blocker does not apply to discard.
Discard deletes the worktree, its uncommitted changes and its unmerged commits. Use it only when the user asks to
discard, delete or throw away the thread. On the project's own checkout, discard only drops uncommitted changes;
commits stay.
A successful merge or squash without keepOpen, cleanup or discard removes the thread and stops its panels, including
this agent.
Report what you are about to do before the final call. If a merge fails with conflicts, the thread stays open:
abort the unfinished merge in the destination checkout (git merge --abort, or git reset --merge after a squash),
merge the destination branch into this thread, resolve the conflicts, commit, then preview and merge again.
`

type threadCommand struct {
	Action     string `json:"action"`
	Base       string `json:"base,omitempty"`
	SourceHead string `json:"sourceHead,omitempty"`
	TargetHead string `json:"targetHead,omitempty"`
	Message    string `json:"message,omitempty"`
	KeepOpen   bool   `json:"keepOpen,omitempty"`
}

func threadTool() map[string]any {
	properties := map[string]any{}
	for _, key := range []string{"base", "sourceHead", "targetHead", "message"} {
		properties[key] = map[string]any{"type": "string"}
	}
	properties["keepOpen"] = map[string]any{"type": "boolean"}
	properties["action"] = map[string]any{"type": "string", "enum": []string{"preview", "close", "merge", "squash", "cleanup", "discard"}}
	return map[string]any{"name": "thread", "description": threadHelp, "inputSchema": map[string]any{"type": "object", "properties": properties, "required": []string{"action"}, "additionalProperties": false}}
}

// ThreadCommand always uses the caller's Libro binding, never a target path.
func ThreadCommand(command json.RawMessage) (json.RawMessage, error) {
	scope := os.Getenv("LIBRO_APPLICATION_PATH")
	if scope == "" {
		var err error
		scope, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	return scopedThreadCommand(command, scope)
}
func scopedThreadCommand(command json.RawMessage, scope string) (json.RawMessage, error) {
	var args threadCommand
	decoder := json.NewDecoder(bytes.NewReader(command))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return nil, err
	}
	if scope == "" {
		return nil, errors.New("thread workspace binding unavailable")
	}
	payload, err := json.Marshal(map[string]any{"action": "thread", "project": scope, "thread": args})
	if err != nil {
		return nil, err
	}
	return agentControlCommand(payload)
}
func RunThreadCLI(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" {
		_, err := io.WriteString(out, threadHelp)
		return err
	}
	if len(args) != 1 {
		return errors.New("expected one JSON command; see libro thread --help")
	}
	result, err := ThreadCommand(json.RawMessage(args[0]))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}

// controlThread finishes only the thread whose checkout is the caller's workspace.
func controlThread(sid, scope string, args threadCommand) (r.Result, any, error) {
	if !slices.Contains([]string{"preview", "close", "merge", "squash", "cleanup", "discard"}, args.Action) {
		return r.Result{}, nil, errors.New("unknown thread action")
	}
	if args.KeepOpen && args.Action != "merge" && args.Action != "squash" {
		return r.Result{}, nil, errors.New("keepOpen works only with merge and squash")
	}
	worktreeFinishMu.Lock()
	defer worktreeFinishMu.Unlock()
	state := sm.Get(sid)
	workspace, path, err := applicationProject(state, scope)
	if err != nil {
		return r.Result{}, nil, err
	}
	info, err := inspectFinishThread(state, state.projectScope(workspace), args.Base)
	if err != nil {
		return r.Result{}, nil, err
	}
	// QA children and plain folders resolve to a parent project; never finish that instead.
	if applicationPath(info.Path) != applicationPath(path) {
		return r.Result{}, nil, errors.New("this workspace is not a Libro thread that can be finished")
	}
	switch args.Action {
	case "preview":
		return r.Result{}, info, nil
	case "close":
		js, err := closeWorkspace(sid, info.Name)
		if err != nil {
			return js, nil, err
		}
		return js, map[string]string{"result": "Thread closed"}, nil
	}
	active := state.ActiveProject
	js, warning, _, _, err := finishThread(sid, info, args.Action, args.SourceHead, args.TargetHead, args.Message, "", args.KeepOpen)
	if args.KeepOpen {
		if err != nil {
			return js, nil, err
		}
		done := "Thread " + map[string]string{"merge": "merged", "squash": "squashed"}[args.Action] + " into " + info.Base + "; it stays open"
		return clientScript("window.__libroShowToast?.(props[0],'','success');", done), map[string]string{"result": done, "base": info.Base, "targetPath": info.TargetPath}, nil
	}
	done := map[string]string{"merge": "Thread merged", "squash": "Thread squashed", "cleanup": "Thread removed", "discard": "Thread discarded"}[args.Action]
	if info.Checkout {
		done = "Changes discarded and thread closed"
	}
	kind := "success"
	if err != nil {
		// A failed cleanup may already have stopped this agent, so tell the user too.
		done, kind = err.Error(), "error"
	} else if warning != "" {
		done, kind = warning, "error"
	}
	if err == nil && active == info.Name {
		js = r.Merge(js, clientScript("window.libroWorkspace?.threadRemoved();"))
	} else if sm.Get(sid).ActiveProject != active {
		// Keep the user on the thread they were viewing.
		js = r.Merge(js, switchToProjectName(sid, active))
	}
	js = r.Merge(js, clientScript("window.__libroShowToast?.(props[0],'',props[1]);", done, kind))
	if err != nil {
		return js, nil, err
	}
	return js, map[string]string{"result": done, "warning": warning}, nil
}
