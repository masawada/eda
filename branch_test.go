package main

import (
	"path/filepath"
	"testing"
)

func TestValidateBranchName(t *testing.T) {
	repo := newTestRepo(t)
	valid := []string{"main", "feature/foo", "fix-123", "a.b"}
	for _, name := range valid {
		if err := validateBranchName(repo, name); err != nil {
			t.Errorf("validateBranchName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"", "-flag", "refs/heads/foo", "origin/foo", "has space", "a..b", "trailing/"}
	for _, name := range invalid {
		if err := validateBranchName(repo, name); err == nil {
			t.Errorf("validateBranchName(%q) = nil, want error", name)
		}
	}
}

func TestLocalBranchExists(t *testing.T) {
	repo := newTestRepo(t)
	gitT(t, repo, "branch", "topic")
	tests := []struct {
		branch string
		want   bool
	}{
		{"main", true},
		{"topic", true},
		{"nope", false},
	}
	for _, tt := range tests {
		got, err := localBranchExists(repo, tt.branch)
		if err != nil {
			t.Fatalf("localBranchExists(%q): %v", tt.branch, err)
		}
		if got != tt.want {
			t.Errorf("localBranchExists(%q) = %v, want %v", tt.branch, got, tt.want)
		}
	}
}

func TestRemoteBranchExists(t *testing.T) {
	origin := newTestRepo(t)
	gitT(t, origin, "branch", "remote-only")
	repo := filepath.Join(t.TempDir(), "clone")
	gitT(t, origin, "clone", "-q", origin, repo)

	got, err := remoteBranchExists(repo, "remote-only")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("remoteBranchExists(remote-only) = false, want true")
	}
	got, err = remoteBranchExists(repo, "nope")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("remoteBranchExists(nope) = true, want false")
	}
}

func TestHeadCommit(t *testing.T) {
	repo := newTestRepo(t)
	sha, err := headCommit(repo)
	if err != nil {
		t.Fatalf("headCommit: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("headCommit = %q, want 40-char sha", sha)
	}
}

func TestHeadCommitUnbornHead(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "main")
	if _, err := headCommit(dir); err == nil {
		t.Error("headCommit on unborn HEAD must fail")
	}
}
