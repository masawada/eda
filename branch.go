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
	// "@" alone is git's alias for HEAD, not a usable branch name. "HEAD"
	// itself passes check-ref-format but git's branch commands refuse it,
	// so reject it here instead of leaking a raw git error later.
	if name == "@" || name == "HEAD" {
		return fmt.Errorf("invalid branch name %q", name)
	}
	// A branch literally named "origin/x" is technically legal in git, but
	// accepting it would make "eda switch origin/foo" ambiguous with the
	// remote-tracking spelling, so refuse it outright.
	if strings.HasPrefix(name, "origin/") {
		return fmt.Errorf("branch must be a short name without a remote prefix: %q", name)
	}
	// check-ref-format --branch expands the previous-checkout shorthand
	// (e.g. "@{-1}") to a different name, which eda would then use verbatim
	// as its ref and hash input. Validate against the literal ref form so
	// only genuine short names pass.
	if _, err := runGit(dir, "check-ref-format", "refs/heads/"+name); err != nil {
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
	_, code, stderrMsg, err := runGitExit(dir, "show-ref", "--verify", "--quiet", ref)
	if err != nil {
		return false, err
	}
	switch code {
	case 0:
		return true, nil
	case 1:
		// show-ref --verify --quiet exits 1 for a missing ref.
		return false, nil
	default:
		// Anything else (corrupt refs, permissions, ...) must not be read
		// as absence: the gone-upstream deletion path would fail open.
		return false, fmt.Errorf("git show-ref %s: exit status %d: %s", ref, code, stderrMsg)
	}
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
