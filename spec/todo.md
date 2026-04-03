--tree
add support to list and switch between multiple git worktress withing the same "project/directroy" show worktrees to application (even when user is not at master branch). user can select another worktree and app will create new "virtual project" which will be bound to existing project when its related by worktree. add shortcut to get list of git worktreen as popup. user can search by typing name and hit enter to go to this worktree directory. user can add new worktree and also delete it.
Move application list (from both side) to the top header menu. Move project list from top menu header to the left side and make it vertical. List git worktree as tree view for given project.
use this feature only when 'git' command is available

--color
change main color to outlook blue color

--apps
when adding/removing apps or displaying apps page it will reload all already running app in current project (other pojrects are fine). fix this when applications are running keep them without reload current project
