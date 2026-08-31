package main

import (
	"errors"
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

// runGitExit runs git with the given working directory and returns its
// stdout and exit code. err is non-nil only when git could not run at all;
// a nonzero exit is reported through code so callers can distinguish
// expected failures (e.g. "ref missing", "config key unset") from
// operational errors instead of failing open.
func runGitExit(dir string, args ...string) (out string, code int, stderrMsg string, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	outBytes, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return string(outBytes), exitErr.ExitCode(), strings.TrimSpace(stderr.String()), nil
		}
		return "", -1, "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(outBytes), 0, "", nil
}

// runGit runs git and returns its stdout, turning any failure into an error
// that carries git's stderr.
func runGit(dir string, args ...string) (string, error) {
	out, code, stderrMsg, err := runGitExit(dir, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		if stderrMsg == "" {
			stderrMsg = fmt.Sprintf("exit status %d", code)
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), stderrMsg)
	}
	return out, nil
}

// loadRepo resolves the repository context for the given directory.
func loadRepo(dir string) (*repoContext, error) {
	out, err := runGit(dir, "worktree", "list", "--porcelain", "-z")
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
// "~"), falling back to ~/.local/share/worktrees only when the key is unset
// (exit 1); any other config failure is an error, not a silent fallback.
// The value must be an absolute path: a relative root would resolve
// differently depending on the invocation directory, breaking the canonical
// placement policy. An existing root is resolved to a realpath so the paths
// eda computes compare equal to the realpaths git records; a missing root is
// returned as-is and created lazily by worktree creation, so read-only
// commands never provision storage.
func worktreeRoot(dir string) (string, error) {
	var root string
	out, code, stderrMsg, err := runGitExit(dir, "config", "--type=path", "--get", "eda.worktreeroot")
	switch {
	case err != nil:
		return "", err
	case code == 0:
		root = strings.TrimSuffix(out, "\n")
		if !filepath.IsAbs(root) {
			return "", fmt.Errorf("eda.worktreeRoot must be an absolute path, got %q", root)
		}
	case code == 1:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		root = filepath.Join(home, ".local", "share", "worktrees")
	default:
		return "", fmt.Errorf("read eda.worktreeRoot: exit status %d: %s", code, stderrMsg)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		if os.IsNotExist(err) {
			return root, nil
		}
		return "", fmt.Errorf("resolve worktree root: %w", err)
	}
	return resolved, nil
}

// ensureWorktreeRoot creates the worktree root if needed and returns its
// realpath. Only worktree creation calls this.
func ensureWorktreeRoot(ctx *repoContext) (string, error) {
	if err := os.MkdirAll(ctx.WorktreeRoot, 0o755); err != nil {
		return "", fmt.Errorf("create worktree root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(ctx.WorktreeRoot)
	if err != nil {
		return "", fmt.Errorf("resolve worktree root: %w", err)
	}
	return resolved, nil
}
