package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	dir, created, baseOID, err := resolveWorktreeDir(ctx, cwd, branch)
	if err != nil {
		return "", err
	}
	if created {
		if err := copyWorktreeInclude(strings.TrimSuffix(srcTop, "\n"), dir); err != nil {
			rollback := ""
			if left := rollbackWorktreeAdd(ctx, dir, branch, baseOID); left != "" {
				rollback = "; " + left
			}
			return "", fmt.Errorf("include copy into %s failed (worktree rolled back%s): %w", dir, rollback, err)
		}
	}
	return dir, nil
}

// resolveWorktreeDir returns the worktree path for the branch, creating it
// when needed. created reports whether a worktree was created by this call;
// baseOID is the commit the branch was created at when this call created
// the branch too, and empty when the branch already existed.
//
// `git worktree add` can fail after the worktree and branch exist (the
// post-checkout hook runs last), so a failed add is rolled back before the
// error is returned; otherwise the next switch would find the
// half-initialized worktree and reuse it as is.
func resolveWorktreeDir(ctx *repoContext, cwd, branch string) (dir string, created bool, baseOID string, err error) {
	if err := validateBranchName(ctx.PrimaryPath, branch); err != nil {
		return "", false, "", err
	}

	// Stage 1: a worktree for this branch already exists.
	for _, e := range ctx.Entries {
		if !e.Bare && !e.Detached && e.Branch == branch {
			if e.Prunable {
				return "", false, "", fmt.Errorf("worktree registration for %q at %s is stale; run `git worktree prune` and retry", branch, e.Path)
			}
			return e.Path, false, "", nil
		}
	}

	root, err := ensureWorktreeRoot(ctx)
	if err != nil {
		return "", false, "", err
	}
	dir, err = chooseWorktreeDir(worktreeBase(root, ctx.PrimaryPath), ctx.Entries)
	if err != nil {
		return "", false, "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", false, "", fmt.Errorf("create worktree parent directory: %w", err)
	}

	// Stage 2: the local branch exists; give it a worktree.
	if ok, err := localBranchExists(ctx.PrimaryPath, branch); err != nil {
		return "", false, "", err
	} else if ok {
		if _, err := runGit(ctx.PrimaryPath, "worktree", "add", dir, branch); err != nil {
			return "", false, "", failedWorktreeAdd(ctx, dir, branch, "", err)
		}
		return dir, true, "", nil
	}

	// Stage 3: only the remote-tracking ref exists; create a tracking
	// branch. This looks at local refs only, so freshness depends on the
	// user's last fetch.
	if ok, err := remoteBranchExists(ctx.PrimaryPath, branch); err != nil {
		return "", false, "", err
	} else if ok {
		base, err := runGit(ctx.PrimaryPath, "rev-parse", "--verify", "refs/remotes/origin/"+branch+"^{commit}")
		if err != nil {
			return "", false, "", err
		}
		base = strings.TrimSpace(base)
		if _, err := runGit(ctx.PrimaryPath, "worktree", "add", "--track", "-b", branch, dir, "origin/"+branch); err != nil {
			return "", false, "", failedWorktreeAdd(ctx, dir, branch, base, err)
		}
		return dir, true, base, nil
	}

	// Stage 4: the branch does not exist anywhere; create it from the HEAD
	// of the invoking directory, so switching inside a worktree stacks on it.
	base, err := headCommit(cwd)
	if err != nil {
		return "", false, "", err
	}
	if _, err := runGit(ctx.PrimaryPath, "worktree", "add", "-b", branch, dir, base); err != nil {
		return "", false, "", failedWorktreeAdd(ctx, dir, branch, base, err)
	}
	return dir, true, base, nil
}

// failedWorktreeAdd rolls back whatever a failed `git worktree add` left
// behind and returns the add error, extended with what could not be undone.
func failedWorktreeAdd(ctx *repoContext, dir, branch, baseOID string, addErr error) error {
	if left := rollbackWorktreeAdd(ctx, dir, branch, baseOID); left != "" {
		return fmt.Errorf("%w (rollback incomplete: %s)", addErr, left)
	}
	return addErr
}

// rollbackWorktreeAdd undoes a worktree creation for branch at dir: the
// worktree is removed when git still lists it there for that branch, and
// when baseOID is non-empty (the add was asked to create the branch) the
// branch is deleted together with its branch.<name> config section, the
// same cleanup `git branch -D` does. It returns a description of what is
// left for the user to clean up, or "" when the rollback is complete.
//
// The branch is only deleted when the worktree at dir was registered for
// it: `worktree add -b` refuses an existing branch before it creates the
// worktree, so that registration proves this call created the branch. A
// branch that exists without the worktree may be the work of a concurrent
// switch and is left alone. A hook ran between creation and rollback, so
// the branch is also kept when its tip no longer equals baseOID or it has
// become a symbolic ref; `update-ref -d` with the expected value refuses to
// discard commits the hook made on it in between.
func rollbackWorktreeAdd(ctx *repoContext, dir, branch, baseOID string) string {
	out, err := runGit(ctx.PrimaryPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return fmt.Sprintf("rollback failed, clean up manually: %v", err)
	}
	var registered *worktreeEntry
	for _, e := range parseWorktrees(out) {
		if e.Path == dir {
			registered = &e
			break
		}
	}
	if registered == nil {
		return ""
	}
	if registered.Branch != branch {
		return fmt.Sprintf("worktree %s no longer checks out %q and was left in place", dir, branch)
	}
	if _, err := runGit(ctx.PrimaryPath, "worktree", "remove", "--force", dir); err != nil {
		return fmt.Sprintf("rollback failed, remove it manually: %v", err)
	}
	if baseOID == "" {
		return ""
	}

	ref := "refs/heads/" + branch
	_, code, stderrMsg, err := runGitExit(ctx.PrimaryPath, "symbolic-ref", "-q", ref)
	switch {
	case err != nil:
		return fmt.Sprintf("branch rollback failed, delete it manually: %v", err)
	case code == 0:
		return fmt.Sprintf("branch %q was modified after creation and was kept", branch)
	case code != 1:
		return fmt.Sprintf("branch rollback failed, delete it manually: git symbolic-ref %s: exit status %d: %s", ref, code, stderrMsg)
	}
	out, code, stderrMsg, err = runGitExit(ctx.PrimaryPath, "rev-parse", "--verify", "--quiet", ref)
	switch {
	case err != nil:
		return fmt.Sprintf("branch rollback failed, delete it manually: %v", err)
	case code == 0:
		if strings.TrimSpace(out) != baseOID {
			return fmt.Sprintf("branch %q was modified after creation and was kept", branch)
		}
		if _, err := runGit(ctx.PrimaryPath, "update-ref", "--no-deref", "-d", ref, baseOID); err != nil {
			return fmt.Sprintf("branch rollback failed, delete it manually: %v", err)
		}
	case code != 1:
		return fmt.Sprintf("branch rollback failed, delete it manually: git rev-parse %s: exit status %d: %s", ref, code, stderrMsg)
	}
	// The config section is cleaned up even when the ref is already gone:
	// it was written for the branch this call created.
	if err := removeBranchConfig(ctx.PrimaryPath, branch); err != nil {
		return fmt.Sprintf("branch config cleanup failed, remove the config section of branch %q manually: %v", branch, err)
	}
	return ""
}

// removeBranchConfig deletes the branch.<name> section from the repository
// config, as `git branch -D` does after deleting the ref. The section is
// looked up first because `git config --remove-section` fails on a missing
// section, which would be indistinguishable from a real failure.
func removeBranchConfig(dir, branch string) error {
	// A key is branch.<name>.<key> with a dot-free key, so anchoring both
	// ends keeps branch "a" from matching branch "a.b".
	pattern := `^branch\.` + regexp.QuoteMeta(branch) + `\.[^.]*$`
	_, code, stderrMsg, err := runGitExit(dir, "config", "--local", "--get-regexp", "--name-only", pattern)
	switch {
	case err != nil:
		return err
	case code == 1:
		// No branch.<name> keys exist.
		return nil
	case code != 0:
		return fmt.Errorf("read branch config: exit status %d: %s", code, stderrMsg)
	}
	_, err = runGit(dir, "config", "--local", "--remove-section", "branch."+branch)
	return err
}
