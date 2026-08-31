package main

import (
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

func canonicalDir(root, primary, branch string) string {
	return filepath.Join(root, strings.TrimPrefix(primary, "/"), branchHash(branch)[:8])
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
	want := canonicalDir(root, repo, "topic")
	if got != want {
		t.Errorf("worktree path = %q, want canonical %q", got, want)
	}
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
	want := canonicalDir(root, repo, "remote-only")
	if got != want {
		t.Errorf("worktree path = %q, want canonical %q", got, want)
	}
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
	if want := canonicalDir(root, repo, "feature-b"); wtB != want {
		t.Errorf("worktree path = %q, want canonical %q", wtB, want)
	}
}

func TestResolveWorktreeInvalidName(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	if _, err := resolveWorktree(ctx, repo, "origin/foo"); err == nil {
		t.Error("invalid branch name must be rejected")
	}
}
