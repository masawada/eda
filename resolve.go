package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveWorktree implements the switch semantics: reuse an existing
// worktree, materialize a worktree for an existing local or remote branch,
// or create a new branch based on the HEAD of the invoking directory. It
// returns the absolute path of the resolved worktree.
func resolveWorktree(ctx *repoContext, cwd, branch string) (string, error) {
	if err := validateBranchName(ctx.PrimaryPath, branch); err != nil {
		return "", err
	}

	// Stage 1: a worktree for this branch already exists.
	for _, e := range ctx.Entries {
		if !e.Bare && !e.Detached && e.Branch == branch {
			return e.Path, nil
		}
	}

	candidates := worktreeDirCandidates(ctx.WorktreeRoot, ctx.PrimaryPath, branch)
	dir, err := chooseWorktreeDir(candidates, branch, ctx.Entries)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("create worktree parent directory: %w", err)
	}

	// Stage 2: the local branch exists; give it a worktree.
	if ok, err := localBranchExists(ctx.PrimaryPath, branch); err != nil {
		return "", err
	} else if ok {
		if _, err := runGit(ctx.PrimaryPath, "worktree", "add", dir, branch); err != nil {
			return "", err
		}
		return dir, nil
	}

	// Stage 3: only the remote-tracking ref exists; create a tracking
	// branch. This looks at local refs only, so freshness depends on the
	// user's last fetch.
	if ok, err := remoteBranchExists(ctx.PrimaryPath, branch); err != nil {
		return "", err
	} else if ok {
		if _, err := runGit(ctx.PrimaryPath, "worktree", "add", "--track", "-b", branch, dir, "origin/"+branch); err != nil {
			return "", err
		}
		return dir, nil
	}

	// Stage 4: the branch does not exist anywhere; create it from the HEAD
	// of the invoking directory, so switching inside a worktree stacks on it.
	base, err := headCommit(cwd)
	if err != nil {
		return "", err
	}
	if _, err := runGit(ctx.PrimaryPath, "worktree", "add", "-b", branch, dir, base); err != nil {
		return "", err
	}
	return dir, nil
}
