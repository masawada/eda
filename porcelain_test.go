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
			in: "worktree /home/user/repo\n" +
				"HEAD 8cf50be1111111111111111111111111111111aa\n" +
				"branch refs/heads/main\n" +
				"\n",
			want: []worktreeEntry{
				{Path: "/home/user/repo", Head: "8cf50be1111111111111111111111111111111aa", Branch: "main"},
			},
		},
		{
			name: "linked worktree with slash branch",
			in: "worktree /home/user/repo\n" +
				"HEAD 8cf50be1111111111111111111111111111111aa\n" +
				"branch refs/heads/main\n" +
				"\n" +
				"worktree /wt/abc123\n" +
				"HEAD 9df50be2222222222222222222222222222222bb\n" +
				"branch refs/heads/feature/foo\n" +
				"\n",
			want: []worktreeEntry{
				{Path: "/home/user/repo", Head: "8cf50be1111111111111111111111111111111aa", Branch: "main"},
				{Path: "/wt/abc123", Head: "9df50be2222222222222222222222222222222bb", Branch: "feature/foo"},
			},
		},
		{
			name: "detached, locked and prunable attributes",
			in: "worktree /home/user/repo\n" +
				"HEAD 8cf50be1111111111111111111111111111111aa\n" +
				"branch refs/heads/main\n" +
				"\n" +
				"worktree /wt/detached\n" +
				"HEAD 9df50be2222222222222222222222222222222bb\n" +
				"detached\n" +
				"\n" +
				"worktree /wt/locked\n" +
				"HEAD 9df50be3333333333333333333333333333333cc\n" +
				"branch refs/heads/wip\n" +
				"locked reason with spaces\n" +
				"\n" +
				"worktree /wt/prunable\n" +
				"HEAD 9df50be4444444444444444444444444444444dd\n" +
				"branch refs/heads/gone-dir\n" +
				"prunable gitdir file points to non-existent location\n" +
				"\n",
			want: []worktreeEntry{
				{Path: "/home/user/repo", Head: "8cf50be1111111111111111111111111111111aa", Branch: "main"},
				{Path: "/wt/detached", Head: "9df50be2222222222222222222222222222222bb", Detached: true},
				{Path: "/wt/locked", Head: "9df50be3333333333333333333333333333333cc", Branch: "wip", Locked: true},
				{Path: "/wt/prunable", Head: "9df50be4444444444444444444444444444444dd", Branch: "gone-dir", Prunable: true},
			},
		},
		{
			name: "bare repository entry",
			in: "worktree /home/user/repo.git\n" +
				"bare\n" +
				"\n",
			want: []worktreeEntry{
				{Path: "/home/user/repo.git", Bare: true},
			},
		},
		{
			name: "missing trailing blank line",
			in: "worktree /home/user/repo\n" +
				"HEAD 8cf50be1111111111111111111111111111111aa\n" +
				"branch refs/heads/main\n",
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
