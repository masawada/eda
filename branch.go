package main

import (
	"fmt"
	"strings"
)

// validateBranchName accepts only branch short names. Full refs, remote
// prefixed names, and anything git itself rejects are refused so the rest of
// eda can treat the name as both a ref and a hash input without ambiguity.
func validateBranchName(dir, name string) error {
	if name == "" {
		return fmt.Errorf("branch name is empty")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid branch name %q", name)
	}
	if strings.HasPrefix(name, "refs/") {
		return fmt.Errorf("branch must be a short name, not a full ref: %q", name)
	}
	// A branch literally named "origin/x" is technically legal in git, but
	// accepting it would make "eda switch origin/foo" ambiguous with the
	// remote-tracking spelling, so refuse it outright.
	if strings.HasPrefix(name, "origin/") {
		return fmt.Errorf("branch must be a short name without a remote prefix: %q", name)
	}
	if _, err := runGit(dir, "check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("invalid branch name %q", name)
	}
	return nil
}

// localBranchExists reports whether refs/heads/<branch> exists.
func localBranchExists(dir, branch string) (bool, error) {
	return refExists(dir, "refs/heads/"+branch)
}

// remoteBranchExists reports whether refs/remotes/origin/<branch> exists.
// This looks only at the local remote-tracking ref: eda never touches the
// network, so freshness is the user's responsibility.
func remoteBranchExists(dir, branch string) (bool, error) {
	return refExists(dir, "refs/remotes/origin/"+branch)
}

func refExists(dir, ref string) (bool, error) {
	_, err := runGit(dir, "show-ref", "--verify", "--quiet", ref)
	if err == nil {
		return true, nil
	}
	// show-ref --quiet exits 1 when the ref is missing; treat every failure
	// as absence since the repository itself was already validated.
	return false, nil
}

// headCommit resolves HEAD to a commit sha. It fails on an unborn HEAD
// (a repository without commits has no base to branch from).
func headCommit(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("cannot resolve HEAD (repository without commits?): %w", err)
	}
	return strings.TrimSpace(out), nil
}
