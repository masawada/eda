# eda

eda (枝, a branch of a tree) is a small CLI that makes git worktrees as light to work with as branches. One independent task maps to one branch and one worktree, placed under a canonical location outside the repository. It also ships Claude Code worktree hooks so humans and coding agents share the same placement policy.

eda is not a git wrapper: it only manages worktree lifecycle and navigation. `git worktree list --porcelain` is the single source of truth; there is no registry or database. eda never touches the network — syncing with remotes is your job, and every decision is made from local repository state.

## Requirements

git 2.36 or later.

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
$ eda switch <branch>     # resolve or create the worktree and move there
$ eda list                # list worktrees of the current repository
$ eda tree                # show where the worktrees diverged from each other
$ eda remove <branch>...  # remove worktrees and their branches as pairs
$ eda root                # move back to the primary checkout
$ eda status              # print current repository, worktree, and branch
```

`eda switch` resolves in order: an existing worktree, an existing local branch, a remote branch on `origin` (a tracking branch is created), and finally a new branch based on the HEAD of the directory you run it in — so switching from inside a worktree stacks the new branch on top of it, like `git switch -c`.

`eda remove` deletes the worktree and its branch together. Without `--force`, the worktree must be clean and the branch tip must be reachable from its upstream. When no upstream ref can be resolved, the HEAD of the directory you run it in is used instead; removing the worktree you are in uses the HEAD of the primary checkout. Ignored files do not count as changes. With several branches each one is removed independently: a refused branch does not stop the rest, and the command fails if any branch failed.

## Divergence tree

`eda tree` shows where the worktrees of the repository diverged from each other, which is what you want to know when branches are stacked:

```console
$ eda tree
4ace591 add config loader
├── main[primary] +3
└── s1 +2
    └── 56cd457 s2: add repository +1
        ├── s2 +1
        └── s3* +1
```

Rows are the worktree branches. When a branch has moved past the commit another one was cut from, that commit appears as a row of its own — above, `s3` was cut from `s2` before `s2` gained one more commit, and `main` has moved on since `s1` was cut. `+n` is the number of commits since the parent row, `*` marks the worktree you are in, and the notes are the same as in `eda list`. Branches sitting on the same commit share a row.

Nothing is recorded when a branch is created: the tree is derived from commit ancestry alone, so it stays correct after rebases and merges, and a stack that has not been restacked shows its old fork point.

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

To carry gitignored files such as `.env` into every new worktree, list them in a `.worktreeinclude` file at the repository root (same name and gitignore syntax as Claude Code's native feature). Only files that match a pattern and are also gitignored are copied. Symbolic links are skipped, never followed.

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

The create hook has exactly the same semantics as `eda switch`, with the session's current directory as the base: a subagent spawned inside a worktree stacks on that worktree. The remove hook receives the path of that worktree and removes the worktree and its branch together under the same conditions as `eda remove`, with the HEAD of the primary checkout as the fallback; when they are not met it leaves both in place. Claude Code ignores the hook's result, so a kept worktree shows up only in Claude Code's debug log and in `eda list`.

## Development

```console
$ make test
$ make build
```

## License

[MIT](LICENSE)
