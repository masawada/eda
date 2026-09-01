# eda

`eda` is a small CLI that makes git worktrees as light to work with as branches. One independent task maps to one branch and one worktree, placed under a canonical location outside the repository. It also ships Claude Code worktree hooks so humans and coding agents share the same placement policy.

`eda` is not a git wrapper: it only manages worktree lifecycle and navigation. `git worktree list --porcelain` is the single source of truth; there is no registry or database. `eda` never touches the network — syncing with remotes is your job, and every decision is made from local repository state.

## Install

```console
$ go install github.com/masawada/eda@latest
```

Then add the shell integration to your rc file (zsh and bash are supported):

```sh
eval "$(eda init - zsh)"
```

The integration makes `eda switch` / `eda root` change the current directory and provides completion.

## Usage

```console
$ eda switch <branch>   # resolve or create the worktree and move there
$ eda list              # list worktrees of the current repository
$ eda remove <branch>...  # remove worktrees and their branches as pairs
$ eda root              # move back to the primary checkout
$ eda path <branch>     # print the worktree path (never creates)
$ eda status            # print current repository, worktree, and branch
```

`eda switch` resolves in order: an existing worktree, an existing local branch, a remote branch on `origin` (a tracking branch is created), and finally a new branch based on the HEAD of the directory you run it in — so switching from inside a worktree stacks the new branch on top of it, like `git switch -c`.

`eda remove` deletes the worktree and its branch together, and only when nothing would be lost: the worktree must be clean, and the branch must either have no commits unreachable from other refs, or have a gone upstream (the state `git fetch --prune` leaves after the remote branch was deleted, e.g. by a squash merge). A pushed branch that is still under review is protected. Use `--force` to override. With several branches each one is removed independently: a refused branch does not stop the rest, and the command fails if any branch failed.

## Worktree placement

Worktrees live outside the repository, keyed by the primary checkout path:

```
<worktreeRoot>/<absolute path of the primary checkout>/<random name>/
```

The directory name is a random string and carries no meaning: the mapping between branches and paths always lives in git, so renaming a branch with `git branch -m` needs no follow-up.

The root defaults to `~/.local/share/worktrees` and is configured through git config:

```console
$ git config --global eda.worktreeRoot ~/worktrees
```

A per-repository value in `.git/config` overrides the global one.

## Copying ignored files

To carry gitignored files such as `.env` into every new worktree, list them in a `.worktreeinclude` file at the repository root (same name and gitignore syntax as Claude Code's native feature). Only files that match a pattern and are also gitignored are copied.

## Claude Code integration

Configure the worktree hooks in `~/.claude/settings.json` so that worktrees Claude creates (`--worktree`, subagent `isolation: "worktree"`) follow the same placement policy:

```json
{
  "hooks": {
    "WorktreeCreate": [
      { "hooks": [{ "type": "command", "command": "eda hook worktree-create" }] }
    ],
    "WorktreeRemove": [
      { "hooks": [{ "type": "command", "command": "eda hook worktree-remove" }] }
    ]
  }
}
```

The create hook has exactly the same semantics as `eda switch`, with the session's current directory as the base: a subagent spawned inside a worktree stacks on that worktree. The remove hook deletes the worktree only when nothing would be lost, and keeps it (reporting on stderr) otherwise.

## Development

```console
$ make test
$ make build
```

## License

`eda` is licensed under the [MIT License](LICENSE).
