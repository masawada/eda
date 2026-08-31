package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveWorktree implements the switch semantics: reuse an existing
// worktree, materialize a worktree for an existing local or remote branch,
// or create a new branch based on the HEAD of the invoking directory. It
// returns the absolute path of the resolved worktree. Newly created
// worktrees receive the .worktreeinclude files of the invoking worktree; a
// copy failure rolls the creation back so a retry starts from a clean slate
// instead of finding a half-initialized worktree.
func resolveWorktree(ctx *repoContext, cwd, branch string) (string, error) {
	srcTop, err := runGit(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	dir, created, branchCreated, err := resolveWorktreeDir(ctx, cwd, branch)
	if err != nil {
		return "", err
	}
	if created {
		if err := copyWorktreeInclude(strings.TrimSpace(srcTop), dir); err != nil {
			rollback := ""
			if _, rbErr := runGit(ctx.PrimaryPath, "worktree", "remove", "--force", dir); rbErr != nil {
				rollback = fmt.Sprintf("; rollback failed, remove it manually: %v", rbErr)
			} else if branchCreated {
				if _, rbErr := runGit(ctx.PrimaryPath, "branch", "-q", "-D", branch); rbErr != nil {
					rollback = fmt.Sprintf("; branch rollback failed, delete it manually: %v", rbErr)
				}
			}
			return "", fmt.Errorf("include copy into %s failed (worktree rolled back%s): %w", dir, rollback, err)
		}
	}
	return dir, nil
}

// resolveWorktreeDir returns the worktree path for the branch, creating it
// when needed. created reports whether a worktree was created by this call;
// branchCreated reports whether the branch itself was created too.
func resolveWorktreeDir(ctx *repoContext, cwd, branch string) (dir string, created, branchCreated bool, err error) {
	if err := validateBranchName(ctx.PrimaryPath, branch); err != nil {
		return "", false, false, err
	}

	// Stage 1: a worktree for this branch already exists.
	for _, e := range ctx.Entries {
		if !e.Bare && !e.Detached && e.Branch == branch {
			if e.Prunable {
				return "", false, false, fmt.Errorf("worktree registration for %q at %s is stale; run `git worktree prune` and retry", branch, e.Path)
			}
			return e.Path, false, false, nil
		}
	}

	root, err := ensureWorktreeRoot(ctx)
	if err != nil {
		return "", false, false, err
	}
	candidates := worktreeDirCandidates(root, ctx.PrimaryPath, branch)
	dir, err = chooseWorktreeDir(candidates, branch, ctx.Entries)
	if err != nil {
		return "", false, false, err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", false, false, fmt.Errorf("create worktree parent directory: %w", err)
	}

	// Stage 2: the local branch exists; give it a worktree.
	if ok, err := localBranchExists(ctx.PrimaryPath, branch); err != nil {
		return "", false, false, err
	} else if ok {
		if _, err := runGit(ctx.PrimaryPath, "worktree", "add", dir, branch); err != nil {
			return "", false, false, err
		}
		return dir, true, false, nil
	}

	// Stage 3: only the remote-tracking ref exists; create a tracking
	// branch. This looks at local refs only, so freshness depends on the
	// user's last fetch.
	if ok, err := remoteBranchExists(ctx.PrimaryPath, branch); err != nil {
		return "", false, false, err
	} else if ok {
		if _, err := runGit(ctx.PrimaryPath, "worktree", "add", "--track", "-b", branch, dir, "origin/"+branch); err != nil {
			return "", false, false, err
		}
		return dir, true, true, nil
	}

	// Stage 4: the branch does not exist anywhere; create it from the HEAD
	// of the invoking directory, so switching inside a worktree stacks on it.
	base, err := headCommit(cwd)
	if err != nil {
		return "", false, false, err
	}
	if _, err := runGit(ctx.PrimaryPath, "worktree", "add", "-b", branch, dir, base); err != nil {
		return "", false, false, err
	}
	return dir, true, true, nil
}
