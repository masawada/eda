package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubDirNames replaces randomDirName with a deterministic sequence for the
// duration of the test.
func stubDirNames(t *testing.T, names ...string) {
	t.Helper()
	orig := randomDirName
	i := 0
	randomDirName = func() (string, error) {
		if i >= len(names) {
			t.Fatalf("randomDirName called %d times, only %d names stubbed", i+1, len(names))
		}
		n := names[i]
		i++
		return n, nil
	}
	t.Cleanup(func() { randomDirName = orig })
}

func TestRandomDirName(t *testing.T) {
	name, err := randomDirName()
	if err != nil {
		t.Fatal(err)
	}
	if len(name) != 8 {
		t.Errorf("randomDirName must return 8 characters, got %d (%q)", len(name), name)
	}
	for _, c := range name {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("randomDirName must be lowercase hex, got %q", name)
		}
	}
}

func TestWorktreeBase(t *testing.T) {
	got := worktreeBase("/root", "/home/user/repo")
	if want := "/root/home/user/repo"; got != want {
		t.Errorf("worktreeBase() = %q, want %q", got, want)
	}
}

func TestChooseWorktreeDir(t *testing.T) {
	base := t.TempDir()

	t.Run("free name is taken on the first draw", func(t *testing.T) {
		stubDirNames(t, "aaaa1111")
		got, err := chooseWorktreeDir(base, nil)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(base, "aaaa1111"); got != want {
			t.Errorf("chooseWorktreeDir() = %q, want %q", got, want)
		}
	})

	t.Run("porcelain-registered name is redrawn", func(t *testing.T) {
		stubDirNames(t, "aaaa1111", "bbbb2222")
		entries := []worktreeEntry{{Path: filepath.Join(base, "aaaa1111"), Branch: "other"}}
		got, err := chooseWorktreeDir(base, entries)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(base, "bbbb2222"); got != want {
			t.Errorf("chooseWorktreeDir() = %q, want %q", got, want)
		}
	})

	t.Run("name existing on disk is redrawn", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(base, "cccc3333"), 0o755); err != nil {
			t.Fatal(err)
		}
		stubDirNames(t, "cccc3333", "dddd4444")
		got, err := chooseWorktreeDir(base, nil)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(base, "dddd4444"); got != want {
			t.Errorf("chooseWorktreeDir() = %q, want %q", got, want)
		}
	})

	t.Run("exhausting the attempts is an error", func(t *testing.T) {
		stubDirNames(t, "eeee5555", "eeee5555", "eeee5555", "eeee5555")
		entries := []worktreeEntry{{Path: filepath.Join(base, "eeee5555"), Branch: "other"}}
		if got, err := chooseWorktreeDir(base, entries); err == nil {
			t.Fatalf("want error after exhausting attempts, got %q", got)
		}
	})
}
