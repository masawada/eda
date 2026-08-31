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

func TestResolveWorktreeCopiesIncludes(t *testing.T) {
	repo := setupIncludeRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)

	wt := mustResolve(t, ctx, repo, "feature-x")
	if _, err := os.Stat(filepath.Join(wt, ".env")); err != nil {
		t.Errorf(".env must be copied into the new worktree: %v", err)
	}
}
