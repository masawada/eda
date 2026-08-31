package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// repoContext is everything eda needs to know about the repository the
// command was invoked in, resolved purely from local git state.
type repoContext struct {
	// PrimaryPath is the realpath of the main checkout (the first entry of
	// `git worktree list --porcelain`).
	PrimaryPath string
	// WorktreeRoot is the absolute directory under which canonical worktrees
	// are placed (eda.worktreeRoot, or ~/.local/share/worktrees).
	WorktreeRoot string
	// Entries are all worktrees of this repository, primary included.
	Entries []worktreeEntry
}

// runGit runs git with the given working directory and returns its stdout.
// On failure the error carries git's stderr so callers can surface it.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return string(out), nil
}

// loadRepo resolves the repository context for the given directory.
func loadRepo(dir string) (*repoContext, error) {
	out, err := runGit(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	entries := parseWorktrees(out)
	if len(entries) == 0 {
		return nil, fmt.Errorf("no worktrees found in %s", dir)
	}
	primary, err := filepath.EvalSymlinks(entries[0].Path)
	if err != nil {
		return nil, fmt.Errorf("resolve primary checkout path: %w", err)
	}
	root, err := worktreeRoot(dir)
	if err != nil {
		return nil, err
	}
	return &repoContext{PrimaryPath: primary, WorktreeRoot: root, Entries: entries}, nil
}

// worktreeRoot reads eda.worktreeRoot from git config (--type=path expands
// "~"), falling back to ~/.local/share/worktrees. The value must be an
// absolute path: a relative root would resolve differently depending on the
// invocation directory, breaking the canonical placement policy. The root is
// created and resolved to a realpath so the paths eda computes compare
// equal to the realpaths git records in its worktree list.
func worktreeRoot(dir string) (string, error) {
	var root string
	out, err := runGit(dir, "config", "--type=path", "--get", "eda.worktreeroot")
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		root = filepath.Join(home, ".local", "share", "worktrees")
	} else {
		root = strings.TrimSpace(out)
		if !filepath.IsAbs(root) {
			return "", fmt.Errorf("eda.worktreeRoot must be an absolute path, got %q", root)
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create worktree root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve worktree root: %w", err)
	}
	return resolved, nil
}
