package main

import (
	"strings"
	"testing"
)

func TestBranchHash(t *testing.T) {
	h1 := branchHash("feature/foo")
	h2 := branchHash("feature/foo")
	h3 := branchHash("feature/bar")
	if h1 != h2 {
		t.Errorf("branchHash is not deterministic: %q != %q", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("different branches must hash differently: %q == %q", h1, h3)
	}
	if len(h1) != 64 {
		t.Errorf("branchHash must return full sha256 hex (64 chars), got %d", len(h1))
	}
	for _, c := range h1 {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("branchHash must be lowercase hex, got %q", h1)
		}
	}
}

func TestWorktreeDirCandidates(t *testing.T) {
	got := worktreeDirCandidates("/root", "/home/user/repo", "feature/foo")
	if len(got) != 4 {
		t.Fatalf("want 4 candidates (hash lengths 8/16/32/64), got %d: %v", len(got), got)
	}
	h := branchHash("feature/foo")
	want := []string{
		"/root/home/user/repo/" + h[:8],
		"/root/home/user/repo/" + h[:16],
		"/root/home/user/repo/" + h[:32],
		"/root/home/user/repo/" + h,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestChooseWorktreeDir(t *testing.T) {
	candidates := []string{"/root/repo/aaaa", "/root/repo/bbbb", "/root/repo/cccc"}
	tests := []struct {
		name    string
		entries []worktreeEntry
		want    string
		wantErr bool
	}{
		{
			name:    "no existing entries picks first candidate",
			entries: nil,
			want:    "/root/repo/aaaa",
		},
		{
			name: "existing entry for the same branch is reused",
			entries: []worktreeEntry{
				{Path: "/root/repo/aaaa", Branch: "feature/foo"},
			},
			want: "/root/repo/aaaa",
		},
		{
			name: "collision with another branch extends to next candidate",
			entries: []worktreeEntry{
				{Path: "/root/repo/aaaa", Branch: "other"},
			},
			want: "/root/repo/bbbb",
		},
		{
			name: "all candidates taken by other branches",
			entries: []worktreeEntry{
				{Path: "/root/repo/aaaa", Branch: "a"},
				{Path: "/root/repo/bbbb", Branch: "b"},
				{Path: "/root/repo/cccc", Branch: "c"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chooseWorktreeDir(candidates, "feature/foo", tt.entries)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("chooseWorktreeDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
