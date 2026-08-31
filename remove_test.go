package main

import (
	"os"
	"path/filepath"
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

	if err := removeWorktree(reload(t, repo), "topic", false); err != nil {
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

	if err := removeWorktree(reload(t, repo), "topic", false); err == nil {
		t.Fatal("dirty worktree must be refused")
	}
	assertKept(t, repo, wt, "topic")
}

func TestRemoveWorktreeRefusesUniqueCommits(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "unique work")

	if err := removeWorktree(reload(t, repo), "topic", false); err == nil {
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

	if err := removeWorktree(reload(t, repo), "topic", true); err != nil {
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

	if err := removeWorktree(reload(t, repo), "topic", false); err != nil {
		t.Fatalf("merged branch must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeProtectsPushedBranch(t *testing.T) {
	origin := newTestRepo(t)
	repo := filepath.Join(t.TempDir(), "clone")
	gitT(t, origin, "clone", "-q", origin, repo)
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "config", "user.email", "test@example.com")
	gitT(t, repo, "config", "user.name", "test")
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "reviewed work")
	gitT(t, wt, "push", "-q", "-u", "origin", "topic")

	// The tip is reachable from origin/topic, but that is the branch's own
	// upstream: a pushed branch under review must not be removable.
	if err := removeWorktree(reload(t, repo), "topic", false); err == nil {
		t.Fatal("pushed branch with unique commits must be refused")
	}
	assertKept(t, repo, wt, "topic")
}

func TestRemoveWorktreeGoneUpstream(t *testing.T) {
	origin := newTestRepo(t)
	repo := filepath.Join(t.TempDir(), "clone")
	gitT(t, origin, "clone", "-q", origin, repo)
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "config", "user.email", "test@example.com")
	gitT(t, repo, "config", "user.name", "test")
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "squash-merged work")
	gitT(t, wt, "push", "-q", "-u", "origin", "topic")
	// Simulate a squash-merge cleanup: the remote branch is deleted and the
	// local remote-tracking ref is pruned.
	gitT(t, repo, "update-ref", "-d", "refs/remotes/origin/topic")

	if err := removeWorktree(reload(t, repo), "topic", false); err != nil {
		t.Fatalf("gone-upstream branch must be removable: %v", err)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRemoveWorktreeRefusesPrimary(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)

	if err := removeWorktree(reload(t, repo), "main", false); err == nil {
		t.Fatal("primary checkout branch must be refused")
	}
	if err := removeWorktree(reload(t, repo), "main", true); err == nil {
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
	if err := removeWorktree(reload(t, repo), "topic", false); err == nil {
		t.Fatal("unmanaged worktree must be refused")
	}
	if err := removeWorktree(reload(t, repo), "topic", true); err == nil {
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
	if err := removeWorktree(reload(t, repo), "topic", false); err == nil {
		t.Fatal("prunable registration must be refused")
	}
	if ok, _ := localBranchExists(repo, "topic"); !ok {
		t.Error("branch must be kept for a prunable registration")
	}
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

	if err := removeWorktree(reload(t, repo), "topic", false); err == nil {
		t.Fatal("dirty worktree must be refused regardless of status config")
	}
	assertKept(t, repo, wt, "topic")
}

func TestRemoveWorktreeUnknownBranch(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	if err := removeWorktree(reload(t, repo), "nope", false); err == nil {
		t.Fatal("branch without a worktree must be an error")
	}
}
