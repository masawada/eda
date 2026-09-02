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

// removeBase is the HEAD a branch without a resolvable upstream is judged
// against, fixed once when the command starts so that every branch of one
// invocation sees the same commits, whatever earlier removals deleted.
type removeBase struct {
	// Top is the realpath of the worktree the command was run in, empty
	// when it was not run in one (the hook), and Head is the OID its HEAD
	// pointed to.
	Top  string
	Head string
	// PrimaryHead is the OID of the primary checkout's HEAD. It replaces
	// Head for the worktree the command was run in, and is the base for
	// everything when there is no such worktree.
	PrimaryHead string
}

// invocationBase resolves the removeBase of a command run in cwd.
func invocationBase(ctx *repoContext, cwd string) (removeBase, error) {
	out, err := runGit(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return removeBase{}, err
	}
	// Entries hold the realpaths git recorded; resolve the same way so the
	// invoking worktree is recognized through a symlinked path.
	top, err := filepath.EvalSymlinks(strings.TrimSuffix(out, "\n"))
	if err != nil {
		return removeBase{}, fmt.Errorf("resolve invoking worktree path: %w", err)
	}
	head, err := headCommit(cwd)
	if err != nil {
		return removeBase{}, err
	}
	primaryHead, err := headCommit(ctx.PrimaryPath)
	if err != nil {
		return removeBase{}, err
	}
	return removeBase{Top: top, Head: head, PrimaryHead: primaryHead}, nil
}

// headFor returns the base OID for the worktree at path and a description
// of it for diagnostics.
func (b removeBase) headFor(path string) (oid, desc string) {
	if b.Top == "" || path == b.Top {
		return b.PrimaryHead, "the HEAD of the primary checkout"
	}
	return b.Head, "the HEAD of " + b.Top
}

// removeWorktree deletes a worktree and its branch as a pair. Unless force
// is set, it refuses to touch anything when the worktree is dirty or the
// branch tip is not reachable from its base, the rule of `git branch -d`:
// the base is the branch's upstream when that ref resolves, otherwise the
// HEAD that base names.
//
// All conditions are checked before any deletion. A failure between the
// worktree removal and the branch deletion leaves the branch behind without
// a worktree; eda does not remove such a branch, so it must be deleted by
// hand.
func removeWorktree(ctx *repoContext, base removeBase, branch string, force bool) error {
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
		rev, desc, err := upstreamBase(ctx.PrimaryPath, branch)
		if err != nil {
			return err
		}
		if rev == "" {
			rev, desc = base.headFor(entry.Path)
		}
		reachable, err := isAncestor(ctx.PrimaryPath, "refs/heads/"+branch, rev)
		if err != nil {
			return err
		}
		if !reachable {
			return refusalf("branch %q is not fully merged into %s; merge it or use --force", branch, desc)
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
	// `git branch -d` would judge the tip again, against the HEAD of the
	// directory it runs in rather than the base checked above, so deletion
	// uses -D.
	if _, err := runGit(ctx.PrimaryPath, "branch", "-q", "-D", branch); err != nil {
		// The branch name is data here, not a command to paste: it may
		// contain shell metacharacters.
		return fmt.Errorf("worktree removed but branch %q was not deleted (%v); delete it manually", branch, err)
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

// upstreamBase returns the configured upstream of the branch as the ref the
// tip must be reachable from, with a description for diagnostics, or ""
// when no upstream is configured or its ref does not resolve (the state
// `git fetch --prune` leaves after the remote branch was deleted). A local
// branch set as upstream (branch.<name>.remote = .) counts as well.
func upstreamBase(dir, branch string) (rev, desc string, err error) {
	out, err := runGit(dir, "for-each-ref", "--format=%(upstream)", "refs/heads/"+branch)
	if err != nil {
		return "", "", err
	}
	upstream := strings.TrimSpace(out)
	if upstream == "" {
		return "", "", nil
	}
	exists, err := refExists(dir, upstream)
	if err != nil {
		return "", "", err
	}
	if !exists {
		return "", "", nil
	}
	return upstream, "its upstream " + upstream, nil
}

// isAncestor reports whether rev is reachable from base (or equal to it).
func isAncestor(dir, rev, base string) (bool, error) {
	_, code, stderrMsg, err := runGitExit(dir, "merge-base", "--is-ancestor", rev, base)
	if err != nil {
		return false, err
	}
	switch code {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("git merge-base --is-ancestor %s %s: exit status %d: %s", rev, base, code, stderrMsg)
	}
}
