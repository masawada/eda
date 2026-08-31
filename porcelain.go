package main

import "strings"

// worktreeEntry is one worktree from `git worktree list --porcelain`.
// Branch holds the short branch name; it is empty for detached or bare
// entries.
type worktreeEntry struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Bare     bool
	Locked   bool
	Prunable bool
}

// parseWorktrees parses `git worktree list --porcelain` output. Git is the
// source of truth for the branch-to-worktree mapping, so this parser is the
// only discovery mechanism eda has; it must tolerate attributes it does not
// know about (future git versions may add some) by ignoring them.
func parseWorktrees(out string) []worktreeEntry {
	var entries []worktreeEntry
	var cur *worktreeEntry
	flush := func() {
		if cur != nil {
			entries = append(entries, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			flush()
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur = &worktreeEntry{Path: value}
		case "HEAD":
			if cur != nil {
				cur.Head = value
			}
		case "branch":
			if cur != nil {
				cur.Branch = strings.TrimPrefix(value, "refs/heads/")
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "bare":
			if cur != nil {
				cur.Bare = true
			}
		case "locked":
			if cur != nil {
				cur.Locked = true
			}
		case "prunable":
			if cur != nil {
				cur.Prunable = true
			}
		}
	}
	flush()
	return entries
}
