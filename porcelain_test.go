package main

import (
	"reflect"
	"testing"
)

func TestParseWorktrees(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []worktreeEntry
	}{
		{
			name: "empty input",
			in:   "",
			want: nil,
		},
		{
			name: "primary checkout only",
			in: "worktree /home/user/repo\x00" +
				"HEAD 8cf50be1111111111111111111111111111111aa\x00" +
				"branch refs/heads/main\x00" +
				"\x00",
			want: []worktreeEntry{
				{Path: "/home/user/repo", Head: "8cf50be1111111111111111111111111111111aa", Branch: "main"},
			},
		},
		{
			name: "linked worktree with slash branch",
			in: "worktree /home/user/repo\x00" +
				"HEAD 8cf50be1111111111111111111111111111111aa\x00" +
				"branch refs/heads/main\x00" +
				"\x00" +
				"worktree /wt/abc123\x00" +
				"HEAD 9df50be2222222222222222222222222222222bb\x00" +
				"branch refs/heads/feature/foo\x00" +
				"\x00",
			want: []worktreeEntry{
				{Path: "/home/user/repo", Head: "8cf50be1111111111111111111111111111111aa", Branch: "main"},
				{Path: "/wt/abc123", Head: "9df50be2222222222222222222222222222222bb", Branch: "feature/foo"},
			},
		},
		{
			name: "detached, locked and prunable attributes",
			in: "worktree /home/user/repo\x00" +
				"HEAD 8cf50be1111111111111111111111111111111aa\x00" +
				"branch refs/heads/main\x00" +
				"\x00" +
				"worktree /wt/detached\x00" +
				"HEAD 9df50be2222222222222222222222222222222bb\x00" +
				"detached\x00" +
				"\x00" +
				"worktree /wt/locked\x00" +
				"HEAD 9df50be3333333333333333333333333333333cc\x00" +
				"branch refs/heads/wip\x00" +
				"locked reason with spaces\x00" +
				"\x00" +
				"worktree /wt/prunable\x00" +
				"HEAD 9df50be4444444444444444444444444444444dd\x00" +
				"branch refs/heads/gone-dir\x00" +
				"prunable gitdir file points to non-existent location\x00" +
				"\x00",
			want: []worktreeEntry{
				{Path: "/home/user/repo", Head: "8cf50be1111111111111111111111111111111aa", Branch: "main"},
				{Path: "/wt/detached", Head: "9df50be2222222222222222222222222222222bb", Detached: true},
				{Path: "/wt/locked", Head: "9df50be3333333333333333333333333333333cc", Branch: "wip", Locked: true},
				{Path: "/wt/prunable", Head: "9df50be4444444444444444444444444444444dd", Branch: "gone-dir", Prunable: true},
			},
		},
		{
			name: "bare repository entry",
			in: "worktree /home/user/repo.git\x00" +
				"bare\x00" +
				"\x00",
			want: []worktreeEntry{
				{Path: "/home/user/repo.git", Bare: true},
			},
		},
		{
			name: "path containing a newline",
			in: "worktree /wt/evil\npath\x00" +
				"HEAD 8cf50be1111111111111111111111111111111aa\x00" +
				"branch refs/heads/main\x00" +
				"\x00",
			want: []worktreeEntry{
				{Path: "/wt/evil\npath", Head: "8cf50be1111111111111111111111111111111aa", Branch: "main"},
			},
		},
		{
			name: "missing trailing blank line",
			in: "worktree /home/user/repo\x00" +
				"HEAD 8cf50be1111111111111111111111111111111aa\x00" +
				"branch refs/heads/main\x00",
			want: []worktreeEntry{
				{Path: "/home/user/repo", Head: "8cf50be1111111111111111111111111111111aa", Branch: "main"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWorktrees(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseWorktrees() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
