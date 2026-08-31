package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

// branchHash returns the full lowercase sha256 hex of the branch short name.
// The directory name for a worktree is a truncated prefix of this hash; the
// mapping back to the branch always goes through git (porcelain), never
// through the directory name.
func branchHash(branch string) string {
	sum := sha256.Sum256([]byte(branch))
	return hex.EncodeToString(sum[:])
}

// worktreeDirCandidates returns the canonical directory candidates for a
// branch, from the shortest hash prefix to the full hash. Longer prefixes are
// used only when a shorter one is already taken by another branch.
func worktreeDirCandidates(root, primaryPath, branch string) []string {
	h := branchHash(branch)
	base := filepath.Join(root, strings.TrimPrefix(primaryPath, string(filepath.Separator)))
	var candidates []string
	for _, n := range []int{8, 16, 32, 64} {
		candidates = append(candidates, filepath.Join(base, h[:n]))
	}
	return candidates
}

// chooseWorktreeDir picks the first candidate that is either unused or
// already the worktree of the requested branch. Candidates registered to a
// different branch (a truncated-hash collision) are skipped.
func chooseWorktreeDir(candidates []string, branch string, entries []worktreeEntry) (string, error) {
	byPath := make(map[string]worktreeEntry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}
	for _, c := range candidates {
		e, taken := byPath[c]
		if !taken || e.Branch == branch {
			return c, nil
		}
	}
	return "", fmt.Errorf("no available worktree directory for branch %q: all hash candidates collide", branch)
}
