package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGit(t *testing.T) {
	repo := newTestRepo(t)
	out, err := runGit(repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("runGit: %v", err)
	}
	if strings.TrimSpace(out) != "main" {
		t.Errorf("current branch = %q, want main", strings.TrimSpace(out))
	}
	if _, err := runGit(repo, "no-such-subcommand"); err == nil {
		t.Error("runGit must fail for an invalid subcommand")
	}
}

func TestLoadRepoPrimaryPath(t *testing.T) {
	repo := newTestRepo(t)
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{repo, sub} {
		ctx, err := loadRepo(dir)
		if err != nil {
			t.Fatalf("loadRepo(%q): %v", dir, err)
		}
		if ctx.PrimaryPath != repo {
			t.Errorf("PrimaryPath = %q, want %q", ctx.PrimaryPath, repo)
		}
		if len(ctx.Entries) != 1 || ctx.Entries[0].Branch != "main" {
			t.Errorf("Entries = %#v, want single main entry", ctx.Entries)
		}
	}
}

func TestLoadRepoFromLinkedWorktree(t *testing.T) {
	repo := newTestRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	gitT(t, repo, "worktree", "add", "-q", "-b", "topic", wt)

	ctx, err := loadRepo(wt)
	if err != nil {
		t.Fatalf("loadRepo from worktree: %v", err)
	}
	if ctx.PrimaryPath != repo {
		t.Errorf("PrimaryPath = %q, want %q (primary checkout, not the worktree)", ctx.PrimaryPath, repo)
	}
	if len(ctx.Entries) != 2 {
		t.Errorf("Entries = %#v, want two entries", ctx.Entries)
	}
}

func TestLoadRepoOutsideRepository(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	if _, err := loadRepo(t.TempDir()); err == nil {
		t.Error("loadRepo outside a repository must fail")
	}
}

func TestWorktreeRootDefault(t *testing.T) {
	repo := newTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, err := loadRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "worktrees")
	if ctx.WorktreeRoot != want {
		t.Errorf("WorktreeRoot = %q, want default %q", ctx.WorktreeRoot, want)
	}
}

func TestWorktreeRootFromConfig(t *testing.T) {
	repo := newTestRepo(t)
	gitT(t, repo, "config", "eda.worktreeRoot", "/custom/worktrees")
	ctx, err := loadRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.WorktreeRoot != "/custom/worktrees" {
		t.Errorf("WorktreeRoot = %q, want /custom/worktrees", ctx.WorktreeRoot)
	}
}

func TestWorktreeRootTildeExpansion(t *testing.T) {
	repo := newTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	gitT(t, repo, "config", "eda.worktreeRoot", "~/wt")
	ctx, err := loadRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "wt")
	if ctx.WorktreeRoot != want {
		t.Errorf("WorktreeRoot = %q, want %q", ctx.WorktreeRoot, want)
	}
}

func TestWorktreeRootRejectsRelativePath(t *testing.T) {
	repo := newTestRepo(t)
	gitT(t, repo, "config", "eda.worktreeRoot", "relative/path")
	if _, err := loadRepo(repo); err == nil {
		t.Error("relative worktreeRoot must be rejected")
	}
}
