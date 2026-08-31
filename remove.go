package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// refusalError marks a removal that the safety policy declined: nothing was
// deleted and keeping the worktree is a legitimate outcome. Operational
// failures (git errors, partial deletion) use ordinary errors instead, so
// the WorktreeRemove hook can report success for the former and fail for
// the latter.
type refusalError struct{ msg string }

func (e refusalError) Error() string { return e.msg }

func refusalf(format string, a ...any) error {
	return refusalError{msg: fmt.Sprintf(format, a...)}
}

// managedWorktree reports whether the path lives under eda's worktree
// root. eda only deletes worktrees it placed; anything else belongs to
// `git worktree remove`.
func managedWorktree(ctx *repoContext, path string) bool {
	return strings.HasPrefix(path, ctx.WorktreeRoot+string(filepath.Separator))
}

// removeWorktree deletes a worktree and its branch as a pair. Unless force
// is set, it refuses to touch anything when the worktree is dirty or the
// branch still has commits that no other ref can reach: "if nothing would be
// lost it all goes away, if work remains nothing is deleted".
//
// All conditions are checked before any deletion. A failure between the
// worktree removal and the branch deletion can still leave a partial state;
// re-running converges it.
func removeWorktree(ctx *repoContext, branch string, force bool) error {
	if err := validateBranchName(ctx.PrimaryPath, branch); err != nil {
		return err
	}
	if len(ctx.Entries) > 0 && ctx.Entries[0].Branch == branch {
		return refusalf("branch %q is checked out in the primary checkout; eda does not remove it", branch)
	}
	var entry *worktreeEntry
	for i, e := range ctx.Entries[1:] {
		if !e.Bare && !e.Detached && e.Branch == branch {
			entry = &ctx.Entries[i+1]
			break
		}
	}
	if entry == nil {
		return refusalf("no worktree found for branch %q", branch)
	}
	if entry.Locked {
		return refusalf("worktree %s is locked; unlock it with `git worktree unlock` first", entry.Path)
	}
	if entry.Prunable {
		return refusalf("worktree registration for %q at %s is stale; run `git worktree prune`", branch, entry.Path)
	}
	if !managedWorktree(ctx, entry.Path) {
		return refusalf("worktree %s is outside the eda worktree root; remove it with `git worktree remove`", entry.Path)
	}

	if !force {
		clean, err := worktreeClean(entry.Path)
		if err != nil {
			return err
		}
		if !clean {
			return refusalf("worktree %s has uncommitted changes; commit them or use --force", entry.Path)
		}
		safe, reason, err := branchSafeToDelete(ctx.PrimaryPath, branch)
		if err != nil {
			return err
		}
		if !safe {
			return refusalf("branch %q %s; push and merge it, or use --force", branch, reason)
		}
	}

	removeArgs := []string{"worktree", "remove"}
	if force {
		removeArgs = append(removeArgs, "--force")
	}
	removeArgs = append(removeArgs, entry.Path)
	if _, err := runGit(ctx.PrimaryPath, removeArgs...); err != nil {
		return err
	}
	// The safety conditions above are stricter than `git branch -d` (which
	// cannot see squash merges), so deletion itself uses -D.
	if _, err := runGit(ctx.PrimaryPath, "branch", "-q", "-D", branch); err != nil {
		return fmt.Errorf("worktree removed but branch deletion failed: %w", err)
	}
	return nil
}

// worktreeClean reports whether the worktree has no changed or untracked
// files. Ignored files are not considered: files carried over by
// .worktreeinclude are disposable by design. The explicit options override
// repository configuration (status.showUntrackedFiles, submodule ignore
// rules) that could otherwise hide dirt from this safety check.
func worktreeClean(dir string) (bool, error) {
	out, err := runGit(dir, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return false, err
	}
	return strings.Trim(out, "\x00") == "", nil
}

// branchSafeToDelete reports whether deleting the branch loses no commits,
// judged purely from local state:
//
//   - the tip is reachable from another ref (any local branch or
//     remote-tracking ref, excluding the branch itself and its own upstream,
//     so a pushed branch that is merely under review stays protected), or
//   - the configured upstream is gone (its remote-tracking ref no longer
//     exists locally, the state `git fetch --prune` leaves after the remote
//     branch was deleted, e.g. by a squash merge).
func branchSafeToDelete(dir, branch string) (bool, string, error) {
	upstream, err := branchUpstream(dir, branch)
	if err != nil {
		return false, "", err
	}
	if upstream != "" {
		exists, err := refExists(dir, upstream)
		if err != nil {
			return false, "", err
		}
		if !exists {
			return true, "", nil
		}
	}

	refs, err := listRefs(dir)
	if err != nil {
		return false, "", err
	}
	self := "refs/heads/" + branch
	others := make([]string, 0, len(refs))
	for _, r := range refs {
		if r != self && r != upstream {
			others = append(others, r)
		}
	}
	args := append([]string{"rev-list", "--count", self, "--not"}, others...)
	out, err := runGit(dir, args...)
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(out) == "0" {
		return true, "", nil
	}
	return false, fmt.Sprintf("has %s commit(s) not reachable from any other ref", strings.TrimSpace(out)), nil
}

// listRefs returns the full names of all local branches and remote-tracking
// refs.
func listRefs(dir string) ([]string, error) {
	out, err := runGit(dir, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// branchUpstream returns the full ref of the branch's configured upstream,
// or "" when no upstream is configured.
func branchUpstream(dir, branch string) (string, error) {
	out, err := runGit(dir, "for-each-ref", "--format=%(upstream)", "refs/heads/"+branch)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
