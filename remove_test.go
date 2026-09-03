package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustResolve(t *testing.T, ctx *repoContext, cwd, branch string) string {
	t.Helper()
	wt, err := resolveWorktree(ctx, cwd, branch)
	if err != nil {
		t.Fatal(err)
	}
	return wt
}

func reload(t *testing.T, repo string) *repoContext {
	t.Helper()
	ctx, err := loadRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

// baseFrom resolves the fallback base of a remove command run in cwd.
func baseFrom(t *testing.T, ctx *repoContext, cwd string) removeBase {
	t.Helper()
	base, err := invocationBase(ctx, cwd)
	if err != nil {
		t.Fatal(err)
	}
	return base
}

// removeFrom runs removeWorktree as a remove command started in cwd would:
// a forced removal takes no base, like cmdRemove.
func removeFrom(t *testing.T, repo, cwd, branch string, force bool) error {
	t.Helper()
	ctx := reload(t, repo)
	var base removeBase
	if !force {
		base = baseFrom(t, ctx, cwd)
	}
	return removeWorktree(ctx, base, branch, force)
}

// newTestClone clones a fresh test repository so branches can have a
// remote-tracking upstream on origin.
func newTestClone(t *testing.T) (repo string) {
	t.Helper()
	origin := newTestRepo(t)
	repo = filepath.Join(t.TempDir(), "clone")
	gitT(t, origin, "clone", "-q", origin, repo)
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "config", "user.email", "test@example.com")
	gitT(t, repo, "config", "user.name", "test")
	return repo
}

func assertRemoved(t *testing.T, repo, wt, branch string) {
	t.Helper()
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree directory %q must be removed", wt)
	}
	if ok, _ := localBranchExists(repo, branch); ok {
		t.Errorf("branch %q must be removed", branch)
	}
}

func assertKept(t *testing.T, repo, wt, branch string) {
	t.Helper()
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("worktree directory %q must be kept: %v", wt, err)
	}
	if ok, _ := localBranchExists(repo, branch); !ok {
		t.Errorf("branch %q must be kept", branch)
	}
}

func TestRemoveWorktreeNoUniqueCommits(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")

	if err := removeFrom(t, repo, repo, "topic", false); err != nil {
		t.Fatalf("removeWorktree: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeRefusesDirty(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	if err := os.WriteFile(filepath.Join(wt, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeFrom(t, repo, repo, "topic", false); err == nil {
		t.Fatal("dirty worktree must be refused")
	}
	assertKept(t, repo, wt, "topic")
}

func TestRemoveWorktreeIgnoredFilesAreNotChanges(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	// info/exclude lives in the common git dir, so it applies to the
	// linked worktree without adding a tracked .gitignore.
	if err := os.WriteFile(filepath.Join(repo, ".git", "info", "exclude"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "debug.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeFrom(t, repo, repo, "topic", false); err != nil {
		t.Fatalf("ignored files must not block removal: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeRefusesUniqueCommits(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "unique work")

	// No upstream: the base is the HEAD of the primary checkout the command
	// runs in, which does not contain the commit.
	if err := removeFrom(t, repo, repo, "topic", false); err == nil {
		t.Fatal("branch with unique commits must be refused")
	}
	assertKept(t, repo, wt, "topic")
}

func TestRemoveWorktreeForce(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "unique work")
	if err := os.WriteFile(filepath.Join(wt, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeFrom(t, repo, repo, "topic", true); err != nil {
		t.Fatalf("forced removal failed: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeMergedIntoAnotherBranch(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "work")
	// Merge topic into main: the tip becomes reachable from main.
	gitT(t, repo, "merge", "-q", "--ff-only", "topic")

	if err := removeFrom(t, repo, repo, "topic", false); err != nil {
		t.Fatalf("merged branch must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeHeadOfInvokingWorktree(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "work on a")
	wtB := mustResolve(t, reload(t, repo), wtA, "b")

	// a has no upstream and main does not contain its commit, but run from
	// the stacked worktree b, whose HEAD reaches the tip of a.
	if err := removeFrom(t, repo, wtB, "a", false); err != nil {
		t.Fatalf("parent branch must be removable from the child worktree: %v", err)
	}
	assertRemoved(t, repo, wtA, "a")
}

func TestRemoveWorktreeFromInsidePrimaryHead(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "unique work")

	// Run from the worktree being removed, its own HEAD would trivially
	// contain the tip; the primary checkout's HEAD is used instead.
	if err := removeFrom(t, repo, wt, "topic", false); err == nil {
		t.Fatal("unmerged branch must be refused from inside its own worktree")
	}
	assertKept(t, repo, wt, "topic")

	gitT(t, repo, "merge", "-q", "--ff-only", "topic")
	if err := removeFrom(t, repo, wt, "topic", false); err != nil {
		t.Fatalf("branch merged into the primary checkout must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeReachableFromUpstream(t *testing.T) {
	repo := newTestClone(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "reviewed work")
	gitT(t, wt, "push", "-q", "-u", "origin", "topic")

	// main does not contain the commit, but the upstream origin/topic does:
	// a pushed branch is removable like with `git branch -d`.
	if err := removeFrom(t, repo, repo, "topic", false); err != nil {
		t.Fatalf("pushed branch must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeUpstreamWinsOverHead(t *testing.T) {
	repo := newTestClone(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "reviewed work")
	gitT(t, wt, "push", "-q", "-u", "origin", "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "local-only work")
	gitT(t, repo, "merge", "-q", "--ff-only", "topic")

	// main contains everything, but the upstream is the base once it
	// resolves, and origin/topic lacks the local-only commit.
	if err := removeFrom(t, repo, repo, "topic", false); err == nil {
		t.Fatal("branch ahead of its upstream must be refused")
	}
	assertKept(t, repo, wt, "topic")
}

func TestRemoveWorktreeLocalUpstream(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtParent := mustResolve(t, ctx, repo, "parent")
	gitT(t, wtParent, "commit", "-q", "--allow-empty", "-m", "work on parent")
	wt := mustResolve(t, reload(t, repo), wtParent, "topic")

	if err := removeFrom(t, repo, repo, "topic", false); err == nil {
		t.Fatal("branch not reachable from the primary HEAD must be refused without an upstream")
	}
	// branch.topic.remote = . and branch.topic.merge = refs/heads/parent:
	// a local branch is an upstream too.
	gitT(t, repo, "branch", "-q", "--set-upstream-to=parent", "topic")
	if err := removeFrom(t, repo, repo, "topic", false); err != nil {
		t.Fatalf("branch reachable from its local upstream must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeUnbornPrimaryHead(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	// `git switch --orphan` leaves the primary checkout on an unborn HEAD
	// that resolves to no commit.
	gitT(t, repo, "switch", "-q", "--orphan", "orphan")

	// Without an upstream the unborn HEAD is the base, and resolving it
	// fails only now, for this branch.
	err := removeFrom(t, repo, repo, "topic", false)
	if err == nil || !strings.Contains(err.Error(), "cannot resolve HEAD") {
		t.Fatalf("branch judged by the unborn HEAD must fail on it, got %v", err)
	}
	assertKept(t, repo, wt, "topic")

	// Judged by its upstream, the branch never needs that HEAD.
	gitT(t, repo, "branch", "-q", "--set-upstream-to=main", "topic")
	if err := removeFrom(t, repo, repo, "topic", false); err != nil {
		t.Fatalf("branch reachable from its upstream must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeUnbornInvokingHead(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	wtOther := mustResolve(t, reload(t, repo), repo, "other")
	// The primary HEAD stays resolvable; only the worktree the command runs
	// in is left on an unborn HEAD.
	gitT(t, wtOther, "switch", "-q", "--orphan", "orphan")

	err := removeFrom(t, repo, wtOther, "topic", false)
	if err == nil || !strings.Contains(err.Error(), "cannot resolve HEAD") {
		t.Fatalf("branch judged by the unborn invoking HEAD must fail on it, got %v", err)
	}
	if !strings.Contains(err.Error(), "the HEAD of "+wtOther) {
		t.Errorf("error must name the invoking HEAD, got %q", err)
	}
	assertKept(t, repo, wt, "topic")

	gitT(t, repo, "branch", "-q", "--set-upstream-to=main", "topic")
	if err := removeFrom(t, repo, wtOther, "topic", false); err != nil {
		t.Fatalf("branch reachable from its upstream must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeGoneUpstream(t *testing.T) {
	repo := newTestClone(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "squash-merged work")
	gitT(t, wt, "push", "-q", "-u", "origin", "topic")
	// Simulate a squash-merge cleanup: the remote branch is deleted and the
	// local remote-tracking ref is pruned.
	gitT(t, repo, "update-ref", "-d", "refs/remotes/origin/topic")

	// The upstream ref no longer resolves, so the base falls back to the
	// primary HEAD, which does not contain the commit.
	err := removeFrom(t, repo, repo, "topic", false)
	var refusal refusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("gone-upstream branch with unmerged commits must be refused, got %v", err)
	}
	if !strings.Contains(err.Error(), "the HEAD of "+repo) {
		t.Errorf("refusal must name the fallback HEAD, got %q", err)
	}
	assertKept(t, repo, wt, "topic")

	gitT(t, repo, "merge", "-q", "--ff-only", "topic")
	if err := removeFrom(t, repo, repo, "topic", false); err != nil {
		t.Fatalf("gone-upstream branch merged into the primary HEAD must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeBaseFixedAcrossRemovals(t *testing.T) {
	repo := newTestClone(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "work on a")
	gitT(t, wtA, "push", "-q", "-u", "origin", "a")
	wtB := mustResolve(t, reload(t, repo), wtA, "b")

	// Started in worktree a: a passes through its upstream; b has no
	// upstream and its tip is reachable only from the HEAD of a, which the
	// removal of a deleted along with the directory the command started in.
	base := baseFrom(t, ctx, wtA)
	if err := removeWorktree(reload(t, repo), base, "a", false); err != nil {
		t.Fatalf("remove a: %v", err)
	}
	if err := removeWorktree(reload(t, repo), base, "b", false); err != nil {
		t.Fatalf("remove b after the invoking worktree is gone: %v", err)
	}
	assertRemoved(t, repo, wtA, "a")
	assertRemoved(t, repo, wtB, "b")
}

func TestRemoveWorktreeRefusesPrimary(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)

	if err := removeFrom(t, repo, repo, "main", false); err == nil {
		t.Fatal("primary checkout branch must be refused")
	}
	if err := removeFrom(t, repo, repo, "main", true); err == nil {
		t.Fatal("primary checkout branch must be refused even with force")
	}
}

func TestRemoveWorktreeRefusesUnmanagedWorktree(t *testing.T) {
	repo := newTestRepo(t)
	wt := filepath.Join(t.TempDir(), "manual-wt")
	gitT(t, repo, "worktree", "add", "-q", "-b", "topic", wt)
	loadRepoWithRoot(t, repo)

	// A manual worktree outside the worktree root is not eda's to delete,
	// with or without --force.
	if err := removeFrom(t, repo, repo, "topic", false); err == nil {
		t.Fatal("unmanaged worktree must be refused")
	}
	if err := removeFrom(t, repo, repo, "topic", true); err == nil {
		t.Fatal("unmanaged worktree must be refused even with force")
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("unmanaged worktree must be kept: %v", err)
	}
}

func TestRemoveWorktreeRefusesPrunable(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	// Deleting the directory behind git's back leaves a prunable
	// registration; eda must point at `git worktree prune` instead of
	// deleting the branch.
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	if err := removeFrom(t, repo, repo, "topic", false); err == nil {
		t.Fatal("prunable registration must be refused")
	}
	if ok, _ := localBranchExists(repo, "topic"); !ok {
		t.Error("branch must be kept for a prunable registration")
	}
}

func TestRemoveWorktreeRefusesDuplicateBranch(t *testing.T) {
	repo := newTestRepo(t)
	ctx, root := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	// `git worktree add --force` attaches a branch that is already checked
	// out elsewhere; the branch then names two worktrees and eda cannot
	// tell which one to remove.
	dup := filepath.Join(root, "dup")
	gitT(t, repo, "worktree", "add", "-q", "--force", dup, "topic")

	err := removeFrom(t, repo, repo, "topic", true)
	var refusal refusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("branch attached to two worktrees must be refused, got %v", err)
	}
	for _, path := range []string{wt, dup} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("refusal must list the worktree %q, got %q", path, err)
		}
	}
	list := gitT(t, repo, "worktree", "list", "--porcelain")
	for _, path := range []string{wt, dup} {
		if !strings.Contains(list, "worktree "+path+"\n") {
			t.Errorf("worktree %q must stay registered, got %q", path, list)
		}
	}
	assertKept(t, repo, wt, "topic")
	assertKept(t, repo, dup, "topic")
}

func TestRemoveWorktreeRefusesDuplicateBranchOutsideRoot(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	// The duplicate outside the root is not eda's, but neither is the
	// decision which attach is the real one: nothing is touched.
	dup := filepath.Join(t.TempDir(), "manual-wt")
	gitT(t, repo, "worktree", "add", "-q", "--force", dup, "topic")

	err := removeFrom(t, repo, repo, "topic", true)
	var refusal refusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("branch attached to two worktrees must be refused, got %v", err)
	}
	// The duplicate refusal names both worktrees; the unmanaged refusal
	// names only one and must not take precedence.
	for _, path := range []string{wt, dup} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("refusal must list the worktree %q, got %q", path, err)
		}
	}
	assertKept(t, repo, wt, "topic")
	assertKept(t, repo, dup, "topic")
}

func TestRemoveWorktreeCleanCheckIgnoresStatusConfig(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	if err := os.WriteFile(filepath.Join(wt, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// status.showUntrackedFiles=no would hide the file from a bare
	// `git status --porcelain`; the safety check must not be fooled.
	gitT(t, repo, "config", "status.showUntrackedFiles", "no")

	if err := removeFrom(t, repo, repo, "topic", false); err == nil {
		t.Fatal("dirty worktree must be refused regardless of status config")
	}
	assertKept(t, repo, wt, "topic")
}

func TestRemoveWorktreeUnknownBranch(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	if err := removeFrom(t, repo, repo, "nope", false); err == nil {
		t.Fatal("branch without a worktree must be an error")
	}
}
