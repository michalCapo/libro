# Sidebar Redesign: Active Threads Only

Status: design, not implemented.

Mockup: https://claude.ai/artifact/HirB3ocg1EmFNNfzPZk5CN

## Sidebar

- By default show only open threads as cards. Hide the project list.
- Each card: project name, title, branch, status, number.
- Threads stay in the list until the user closes them (also when Done).
- Threads of the same project sit next to each other. A new thread goes right under the other threads of its project.
- Threads without a project are normal cards in the same list, labelled "No project". No separate list at the bottom.
- Open idea (not decided): treat "No project" as one more group and put it last. Newest thread at the bottom of each group.
- Small "All projects (count)" row at the bottom opens the full project list.

## New thread

- Large "New thread" button at the top of the sidebar.
- Opens a project picker: search box, "No project" first, then Recent, then All projects.
- "Own worktree" checkbox, ticked by default: creates a new worktree with a branch from main.
- Enter creates the thread. The current project is first in Recent, so Ctrl+T then Enter makes a thread in the current project.
- The old behaviour (new thread in the current project with no question) is removed.

## Shortcuts (new defaults)

- Ctrl+T: new thread (opens the project picker)
- Ctrl+Enter: new terminal panel
- Ctrl+Shift+Enter: Terminal
- Ctrl+Shift+T and Ctrl+Shift+N: free
- Users who set their own shortcuts keep them.
- Shortcut settings must allow the Enter key (today the key pattern only allows letters, numbers and a few symbols).
- Trade-off: Ctrl+Enter in a browser panel opens a terminal instead of sending a form.
