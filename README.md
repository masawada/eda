# eda

eda (枝, a branch of a tree) is a small CLI that makes git worktrees as light to work with as branches. Each independent task gets one branch and one worktree, placed under a canonical location outside the repository. eda also ships Claude Code worktree hooks, so humans and coding agents share the same placement policy.

eda is not a git wrapper. It manages worktree lifecycle and navigation only. `git worktree list --porcelain` is the single source of truth, and eda keeps no registry or database of its own. eda never fetches or pushes. Syncing with remotes is your job, and eda decides everything from local repository state. It creates worktrees with a plain `git worktree add`, so git's checkout hooks and filters run as configured.

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
$ eda version             # print the version of eda
```

`eda switch` resolves in order: an existing worktree, an existing local branch, a remote branch on `origin` (eda creates a tracking branch), and finally a new branch based on the HEAD of the directory you run it in. Switching from inside a worktree therefore stacks the new branch on top of it, like `git switch -c`.

`eda remove` deletes the worktree and its branch together. Without `--force`, the worktree must be clean and the branch tip must be reachable from its upstream. When no upstream ref can be resolved, eda uses the HEAD of the directory you run it in instead. Removing the worktree you are in uses the HEAD of the primary checkout. Ignored files do not count as changes. With several branches, eda removes each one independently. A refused branch does not stop the rest, and the command fails if any branch failed.

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

Rows are the worktree branches. When a branch has moved past the commit another one was cut from, that commit appears as a row of its own. Above, `s3` was cut from `s2` before `s2` gained one more commit, and `main` has moved on since `s1` was cut. `+n` is the number of commits since the parent row, `*` marks the worktree you are in, and the notes are the same as in `eda list`. Branches sitting on the same commit share a row.

eda records nothing when a branch is created. The tree comes from commit ancestry alone, so it stays correct after rebases and merges, and a stack that has not been restacked shows its old fork point.

## Worktree placement

Worktrees live outside the repository, keyed by the primary checkout path:

```
<worktreeRoot>/<absolute path of the primary checkout>/<random name>/
```

The directory name is a random string and carries no meaning. The mapping between branches and paths lives in git alone, so renaming a branch with `git branch -m` needs no follow-up.

The root defaults to `~/.local/share/worktrees` and is configured through git config:

```console
$ git config --global eda.worktreeRoot ~/worktrees
```

A per-repository value in `.git/config` overrides the global one.

The root is reserved for eda. Any worktree under it with a branch checked out is managed by eda, whichever tool created it, and `eda remove` is the way to delete it.

## Copying ignored files

To carry gitignored files such as `.env` into every new worktree, list them in a `.worktreeinclude` file at the repository root. The file has the same name and gitignore syntax as Claude Code's native feature. eda copies only files that match a pattern and are also gitignored. It does not copy a path that is tracked, or not ignored, in the new worktree. It skips symbolic links instead of following them. The copies are ignored files like the originals, so `eda remove` does not count them as changes.

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

The create hook has exactly the same semantics as `eda switch`, with the session's current directory as the base, so a subagent spawned inside a worktree stacks on that worktree. The remove hook receives the path of that worktree and removes the worktree and its branch together under the same conditions as `eda remove`, with the HEAD of the primary checkout as the fallback. When those conditions are not met, it leaves both in place. Claude Code ignores the hook's result, so a kept worktree shows up only in Claude Code's debug log and in `eda list`.

## Development

```console
$ make test
$ make build
```

## License

[MIT](LICENSE)
