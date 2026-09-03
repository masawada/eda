package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadRepoWithRoot loads the repo context with eda.worktreeRoot pointing at a
// fresh temp directory, and returns both.
func loadRepoWithRoot(t *testing.T, repo string) (*repoContext, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "config", "eda.worktreeRoot", root)
	ctx, err := loadRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, root
}

// assertWorktreeDir checks that dir sits directly under the repo's worktree
// base with a random-style leaf name. The exact name is unpredictable by
// design, so tests only pin down where the worktree lives and what the name
// looks like.
func assertWorktreeDir(t *testing.T, root, primary, dir string) {
	t.Helper()
	if base := worktreeBase(root, primary); filepath.Dir(dir) != base {
		t.Errorf("worktree %q must live directly under %q", dir, base)
	}
	leaf := filepath.Base(dir)
	if len(leaf) != 8 {
		t.Errorf("worktree directory name %q must be 8 characters", leaf)
	}
	for _, c := range leaf {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("worktree directory name %q must be lowercase hex", leaf)
		}
	}
}

func TestResolveWorktreeExisting(t *testing.T) {
	repo := newTestRepo(t)
	wt := filepath.Join(t.TempDir(), "manual-wt")
	gitT(t, repo, "worktree", "add", "-q", "-b", "topic", wt)
	wt, err := filepath.EvalSymlinks(wt)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := loadRepoWithRoot(t, repo)

	got, err := resolveWorktree(ctx, repo, "topic")
	if err != nil {
		t.Fatal(err)
	}
	if got != wt {
		t.Errorf("existing worktree must be reused: got %q, want %q", got, wt)
	}
}

func TestResolveWorktreeFromLocalBranch(t *testing.T) {
	repo := newTestRepo(t)
	gitT(t, repo, "branch", "topic")
	ctx, root := loadRepoWithRoot(t, repo)

	got, err := resolveWorktree(ctx, repo, "topic")
	if err != nil {
		t.Fatal(err)
	}
	assertWorktreeDir(t, root, repo, got)
	if out := gitT(t, got, "rev-parse", "--abbrev-ref", "HEAD"); strings.TrimSpace(out) != "topic" {
		t.Errorf("worktree HEAD = %q, want topic", strings.TrimSpace(out))
	}
}

func TestResolveWorktreeFromRemoteBranch(t *testing.T) {
	origin := newTestRepo(t)
	gitT(t, origin, "branch", "remote-only")
	repo := filepath.Join(t.TempDir(), "clone")
	gitT(t, origin, "clone", "-q", origin, repo)
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx, root := loadRepoWithRoot(t, repo)

	got, err := resolveWorktree(ctx, repo, "remote-only")
	if err != nil {
		t.Fatal(err)
	}
	assertWorktreeDir(t, root, repo, got)
	up := strings.TrimSpace(gitT(t, got, "rev-parse", "--abbrev-ref", "remote-only@{upstream}"))
	if up != "origin/remote-only" {
		t.Errorf("upstream = %q, want origin/remote-only", up)
	}
}

func TestResolveWorktreeCreatesFromCwdHead(t *testing.T) {
	repo := newTestRepo(t)
	ctx, root := loadRepoWithRoot(t, repo)

	// From the primary checkout: base is the primary HEAD.
	mainSha, err := headCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	wtA, err := resolveWorktree(ctx, repo, "feature-a")
	if err != nil {
		t.Fatal(err)
	}
	if sha, _ := headCommit(wtA); sha != mainSha {
		t.Errorf("feature-a base = %s, want primary HEAD %s", sha, mainSha)
	}

	// Advance feature-a, then create feature-b from inside its worktree:
	// the new branch must be stacked on feature-a, not on main.
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "work on a")
	aSha, err := headCommit(wtA)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = loadRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	wtB, err := resolveWorktree(ctx, wtA, "feature-b")
	if err != nil {
		t.Fatal(err)
	}
	if sha, _ := headCommit(wtB); sha != aSha {
		t.Errorf("feature-b base = %s, want feature-a HEAD %s (stacked)", sha, aSha)
	}
	assertWorktreeDir(t, root, repo, wtB)
}

func TestResolveWorktreeRejectsPrunableEntry(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	ctx.Entries = append(ctx.Entries, worktreeEntry{
		Path: "/nonexistent/stale", Branch: "topic", Prunable: true,
	})
	if _, err := resolveWorktree(ctx, repo, "topic"); err == nil {
		t.Error("a prunable registration must not be returned as a usable worktree")
	}
}

func TestResolveWorktreeRollsBackOnCopyFailure(t *testing.T) {
	repo := setupIncludeRepo(t)
	ctx, root := loadRepoWithRoot(t, repo)
	if err := os.Chmod(filepath.Join(repo, ".worktreeinclude"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(repo, ".worktreeinclude"), 0o644) })

	_, err := resolveWorktree(ctx, repo, "feature-x")
	if err == nil {
		t.Fatal("copy failure must surface as an error")
	}
	// The directory name is random, so check that nothing is left under the
	// repo's worktree base instead of probing a known path.
	left, err := os.ReadDir(worktreeBase(root, repo))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Errorf("worktree base must be empty after rollback, found %v", left)
	}
	if ok, _ := localBranchExists(repo, "feature-x"); ok {
		t.Error("created branch must be rolled back")
	}
}

// installPostCheckoutHook points core.hooksPath of the repo at a directory
// holding a post-checkout hook with the given shell body. `git worktree add`
// runs the hook after the worktree and branch exist, so a failing hook is
// how git leaves a half-initialized worktree behind.
func installPostCheckoutHook(t *testing.T, repo, body string) {
	t.Helper()
	hooks := t.TempDir()
	hook := filepath.Join(hooks, "post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "config", "core.hooksPath", hooks)
}

// assertNothingCreated checks that neither a worktree for the branch nor
// any directory under the repo's worktree base survived a rollback.
func assertNothingCreated(t *testing.T, root, repo, branch string) {
	t.Helper()
	for _, e := range reload(t, repo).Entries {
		if e.Branch == branch {
			t.Errorf("worktree registration for %q must be rolled back, found %s", branch, e.Path)
		}
	}
	left, err := os.ReadDir(worktreeBase(root, repo))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Errorf("worktree base must be empty after rollback, found %v", left)
	}
}

func TestResolveWorktreeRollsBackFailedAddKeepingExistingBranch(t *testing.T) {
	repo := newTestRepo(t)
	gitT(t, repo, "branch", "topic")
	installPostCheckoutHook(t, repo, "exit 1\n")
	ctx, root := loadRepoWithRoot(t, repo)

	if _, err := resolveWorktree(ctx, repo, "topic"); err == nil {
		t.Fatal("a failing post-checkout hook must surface as an error")
	}
	assertNothingCreated(t, root, repo, "topic")
	if ok, _ := localBranchExists(repo, "topic"); !ok {
		t.Error("a pre-existing branch must never be deleted by the rollback")
	}
}

func TestResolveWorktreeRollsBackFailedAddOfTrackingBranch(t *testing.T) {
	origin := newTestRepo(t)
	// Dots and slashes in the name exercise the config section removal.
	const branch = "release/v1.2"
	gitT(t, origin, "branch", branch)
	repo := filepath.Join(t.TempDir(), "clone")
	gitT(t, origin, "clone", "-q", origin, repo)
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	installPostCheckoutHook(t, repo, "exit 1\n")
	ctx, root := loadRepoWithRoot(t, repo)

	if _, err := resolveWorktree(ctx, repo, branch); err == nil {
		t.Fatal("a failing post-checkout hook must surface as an error")
	}
	assertNothingCreated(t, root, repo, branch)
	if ok, _ := localBranchExists(repo, branch); ok {
		t.Error("the branch created by this call must be rolled back")
	}
	for _, key := range []string{"remote", "merge"} {
		_, code, _, err := runGitExit(repo, "config", "--get", "branch."+branch+"."+key)
		if err != nil {
			t.Fatal(err)
		}
		if code != 1 {
			t.Errorf("branch.%s.%s must be removed with the branch, git config exit = %d", branch, key, code)
		}
	}
}

func TestResolveWorktreeRollsBackFailedAddOfNewBranch(t *testing.T) {
	repo := newTestRepo(t)
	installPostCheckoutHook(t, repo, "exit 1\n")
	ctx, root := loadRepoWithRoot(t, repo)

	if _, err := resolveWorktree(ctx, repo, "feature-x"); err == nil {
		t.Fatal("a failing post-checkout hook must surface as an error")
	}
	assertNothingCreated(t, root, repo, "feature-x")
	if ok, _ := localBranchExists(repo, "feature-x"); ok {
		t.Error("the branch created by this call must be rolled back")
	}
}

func TestResolveWorktreeKeepsBranchMovedByHook(t *testing.T) {
	repo := newTestRepo(t)
	base, err := headCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	// The hook runs inside the new worktree: commit on the branch, then fail.
	installPostCheckoutHook(t, repo, "git commit -q --allow-empty -m hook || exit 2\nexit 1\n")
	ctx, root := loadRepoWithRoot(t, repo)

	_, err = resolveWorktree(ctx, repo, "feature-x")
	if err == nil {
		t.Fatal("a failing post-checkout hook must surface as an error")
	}
	assertNothingCreated(t, root, repo, "feature-x")
	tip := strings.TrimSpace(gitT(t, repo, "rev-parse", "--verify", "refs/heads/feature-x"))
	if tip == base {
		t.Fatal("test setup: the hook must have moved the branch")
	}
	if !strings.Contains(err.Error(), `"feature-x"`) {
		t.Errorf("error must name the branch that was kept: %v", err)
	}
}

// A hook can turn the created ref into a symbolic ref pointing at another
// branch at the same commit; a dereferencing delete would then remove that
// other branch instead. git reports the worktree as checking out the
// target, so the worktree no longer matches the branch either and is
// reported instead of removed.
func TestResolveWorktreeKeepsRefTurnedSymbolicByHook(t *testing.T) {
	repo := newTestRepo(t)
	installPostCheckoutHook(t, repo, "git symbolic-ref refs/heads/feature-x refs/heads/main || exit 2\nexit 1\n")
	ctx, _ := loadRepoWithRoot(t, repo)

	_, err := resolveWorktree(ctx, repo, "feature-x")
	if err == nil {
		t.Fatal("a failing post-checkout hook must surface as an error")
	}
	if !strings.Contains(err.Error(), "left in place") {
		t.Errorf("a worktree that no longer checks out the branch must be reported: %v", err)
	}
	if ok, _ := localBranchExists(repo, "main"); !ok {
		t.Error("the branch the symbolic ref points at must not be deleted")
	}
	if _, code, _, _ := runGitExit(repo, "symbolic-ref", "-q", "refs/heads/feature-x"); code != 0 {
		t.Error("a ref that no longer is the created branch must be kept")
	}
}

// When `worktree add -b` fails before registering the worktree, the branch
// cannot be attributed to this call: another switch may have created it
// at the same base in the meantime, so it must be left alone.
func TestRollbackWorktreeAddKeepsBranchWithoutWorktree(t *testing.T) {
	repo := newTestRepo(t)
	gitT(t, repo, "branch", "topic")
	base, err := headCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx, root := loadRepoWithRoot(t, repo)

	dir := filepath.Join(worktreeBase(root, repo), "deadbeef")
	if left := rollbackWorktreeAdd(ctx, dir, "topic", base); left != "" {
		t.Errorf("nothing to roll back must not be reported as incomplete: %s", left)
	}
	if ok, _ := localBranchExists(repo, "topic"); !ok {
		t.Error("a branch without a worktree at the candidate path must be kept")
	}
}

func TestResolveWorktreeReportsConfigCleanupFailure(t *testing.T) {
	origin := newTestRepo(t)
	gitT(t, origin, "branch", "remote-only")
	repo := filepath.Join(t.TempDir(), "clone")
	gitT(t, origin, "clone", "-q", origin, repo)
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	// A stale config lock makes every later config write fail, while the
	// ref and worktree rollback are unaffected. The hook runs after the
	// tracking configuration was written, so `worktree add` itself gets
	// through.
	lock := filepath.Join(repo, ".git", "config.lock")
	installPostCheckoutHook(t, repo, "touch '"+lock+"'\nexit 1\n")
	ctx, root := loadRepoWithRoot(t, repo)

	_, err = resolveWorktree(ctx, repo, "remote-only")
	if err == nil {
		t.Fatal("a failing post-checkout hook must surface as an error")
	}
	assertNothingCreated(t, root, repo, "remote-only")
	if ok, _ := localBranchExists(repo, "remote-only"); ok {
		t.Error("the branch created by this call must be rolled back")
	}
	msg := err.Error()
	if !strings.Contains(msg, "git worktree add") {
		t.Errorf("error must carry the original worktree add failure: %v", err)
	}
	if !strings.Contains(msg, "branch config cleanup failed") {
		t.Errorf("error must report the config cleanup failure separately: %v", err)
	}
}

func TestResolveWorktreeInvalidName(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	if _, err := resolveWorktree(ctx, repo, "origin/foo"); err == nil {
		t.Error("invalid branch name must be rejected")
	}
}
