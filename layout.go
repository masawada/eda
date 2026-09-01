package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxNameAttempts bounds the redraws in chooseWorktreeDir. Collisions on a
// 32-bit name are practically nonexistent, so exhausting the attempts means
// the environment is broken and is reported as an error.
const maxNameAttempts = 4

// randomDirName returns a random 8-character lowercase hex string. It is a
// variable so tests can substitute a deterministic sequence.
var randomDirName = func() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate worktree directory name: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// worktreeBase returns the directory that holds all worktrees of the repo
// identified by its primary checkout path.
func worktreeBase(root, primaryPath string) string {
	return filepath.Join(root, strings.TrimPrefix(primaryPath, string(filepath.Separator)))
}

// chooseWorktreeDir draws a random directory name under base. The name is
// meaningless on purpose: the branch <-> path mapping always lives in git
// (worktree list --porcelain), so nothing may depend on the directory name.
// A candidate is redrawn when it is registered as a worktree (including
// prunable leftovers) or already exists on disk.
func chooseWorktreeDir(base string, entries []worktreeEntry) (string, error) {
	registered := make(map[string]bool, len(entries))
	for _, e := range entries {
		registered[e.Path] = true
	}
	for range maxNameAttempts {
		name, err := randomDirName()
		if err != nil {
			return "", err
		}
		dir := filepath.Join(base, name)
		if registered[dir] {
			continue
		}
		if _, err := os.Lstat(dir); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("check worktree directory %s: %w", dir, err)
		}
		return dir, nil
	}
	return "", fmt.Errorf("no available worktree directory under %s after %d attempts", base, maxNameAttempts)
}
