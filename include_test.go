package main

import (
	"os"
	"path/filepath"
	"testing"
)

// setupIncludeRepo builds a repo with a committed .gitignore, ignored files,
// and a .worktreeinclude covering some of them.
func setupIncludeRepo(t *testing.T) string {
	t.Helper()
	repo := newTestRepo(t)
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(".gitignore", ".env\nsecrets/\n")
	gitT(t, repo, "add", ".gitignore")
	gitT(t, repo, "commit", "-q", "-m", "add gitignore")
	writeFile(".env", "SECRET=1\n")
	writeFile("secrets/key.txt", "key\n")
	writeFile("untracked.txt", "not ignored\n")
	writeFile(".worktreeinclude", ".env\nsecrets/\nuntracked.txt\n")
	return repo
}

func TestCopyWorktreeInclude(t *testing.T) {
	repo := setupIncludeRepo(t)
	dst := filepath.Join(t.TempDir(), "wt")
	gitT(t, repo, "worktree", "add", "-q", "-b", "topic", dst)

	if err := copyWorktreeInclude(repo, dst); err != nil {
		t.Fatalf("copyWorktreeInclude: %v", err)
	}
	for _, rel := range []string{".env", "secrets/key.txt"} {
		body, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Errorf("%s must be copied: %v", rel, err)
			continue
		}
		orig, _ := os.ReadFile(filepath.Join(repo, rel))
		if string(body) != string(orig) {
			t.Errorf("%s content = %q, want %q", rel, body, orig)
		}
	}
	// untracked.txt matches a pattern but is not gitignored: never copied.
	if _, err := os.Stat(filepath.Join(dst, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("untracked but not ignored file must not be copied")
	}
}

func TestCopyWorktreeIncludeWithoutFile(t *testing.T) {
	repo := newTestRepo(t)
	dst := filepath.Join(t.TempDir(), "wt")
	gitT(t, repo, "worktree", "add", "-q", "-b", "topic", dst)
	if err := copyWorktreeInclude(repo, dst); err != nil {
		t.Fatalf("missing .worktreeinclude must be a no-op: %v", err)
	}
}

// makeDivergentBranch creates a branch whose tree is edited by mutate,
// using a throwaway worktree that is removed afterwards.
func makeDivergentBranch(t *testing.T, repo, branch string, mutate func(dir string)) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "edit")
	gitT(t, repo, "worktree", "add", "-q", "-b", branch, tmp)
	mutate(tmp)
	gitT(t, tmp, "add", "-A", "-f")
	gitT(t, tmp, "commit", "-q", "-m", "diverge")
	gitT(t, repo, "worktree", "remove", "--force", tmp)
}

func TestCopyWorktreeIncludeNeverOverwritesTrackedFile(t *testing.T) {
	repo := setupIncludeRepo(t)
	// The destination branch tracks .env (and stops ignoring it): the
	// checkout puts the tracked content in place, and the include copy at
	// creation time must not clobber it.
	makeDivergentBranch(t, repo, "tracked-env", func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("secrets/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TRACKED=1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "tracked-env")

	body, err := os.ReadFile(filepath.Join(wt, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "TRACKED=1\n" {
		t.Errorf("tracked .env was overwritten: %q", body)
	}
	if clean, _ := worktreeClean(wt); !clean {
		t.Error("destination worktree must stay clean after the copy")
	}
}

func TestCopyWorktreeIncludeSkipsWhenDestinationDoesNotIgnore(t *testing.T) {
	repo := setupIncludeRepo(t)
	// The destination branch stops ignoring .env: copying it would create
	// an immediately dirty worktree.
	makeDivergentBranch(t, repo, "no-ignore", func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("secrets/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "no-ignore")

	if _, err := os.Stat(filepath.Join(wt, ".env")); !os.IsNotExist(err) {
		t.Error(".env must not be copied when the destination does not ignore it")
	}
	if _, err := os.Stat(filepath.Join(wt, "secrets", "key.txt")); err != nil {
		t.Errorf("still-ignored file must be copied: %v", err)
	}
	if clean, _ := worktreeClean(wt); !clean {
		t.Error("destination worktree must stay clean after the copy")
	}
}

func TestResolveWorktreeCopiesIncludes(t *testing.T) {
	repo := setupIncludeRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)

	wt := mustResolve(t, ctx, repo, "feature-x")
	if _, err := os.Stat(filepath.Join(wt, ".env")); err != nil {
		t.Errorf(".env must be copied into the new worktree: %v", err)
	}
}
