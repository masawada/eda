package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runEda invokes run() as the CLI entrypoint would, capturing output.
func runEda(t *testing.T, cwd, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf strings.Builder
	code = run(strings.NewReader(stdin), &out, &errBuf, args, cwd)
	return code, out.String(), errBuf.String()
}

func TestRunSwitchPrintsPathOnly(t *testing.T) {
	repo := newTestRepo(t)
	ctx, root := loadRepoWithRoot(t, repo)
	_ = ctx

	code, stdout, _ := runEda(t, repo, "", "switch", "feature-a")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	want := canonicalDir(root, repo, "feature-a") + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q (absolute path, one line, nothing else)", stdout, want)
	}
	if _, err := os.Stat(strings.TrimSpace(stdout)); err != nil {
		t.Errorf("printed worktree must exist: %v", err)
	}
}

func TestRunSwitchInvalidUsage(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	code, stdout, stderr := runEda(t, repo, "", "switch")
	if code == 0 {
		t.Error("switch without branch must fail")
	}
	if stdout != "" {
		t.Errorf("stdout must stay empty on error, got %q", stdout)
	}
	if stderr == "" {
		t.Error("error must be reported on stderr")
	}
}

func TestRunRoot(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	code, stdout, _ := runEda(t, repo, "", "root")
	if code != 0 || stdout != repo+"\n" {
		t.Errorf("root: code=%d stdout=%q, want 0 and %q", code, stdout, repo+"\n")
	}
}

func TestRunPath(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")

	code, stdout, _ := runEda(t, repo, "", "path", "topic")
	if code != 0 || stdout != wt+"\n" {
		t.Errorf("path: code=%d stdout=%q, want 0 and %q", code, stdout, wt+"\n")
	}
	// path never creates: an unknown branch is an error.
	code, stdout, _ = runEda(t, repo, "", "path", "nope")
	if code == 0 {
		t.Error("path for a branch without worktree must fail")
	}
	if stdout != "" {
		t.Errorf("stdout must stay empty on error, got %q", stdout)
	}
}

func TestRunList(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	mustResolve(t, ctx, repo, "topic")

	code, stdout, _ := runEda(t, repo, "", "list")
	if code != 0 {
		t.Fatalf("list: exit = %d", code)
	}
	if !strings.Contains(stdout, "main") || !strings.Contains(stdout, "topic") {
		t.Errorf("list output must mention both branches, got %q", stdout)
	}
	if !strings.Contains(stdout, "primary") {
		t.Errorf("list output must mark the primary checkout, got %q", stdout)
	}
}

func TestRunRemove(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")

	code, _, stderr := runEda(t, repo, "", "remove", "topic")
	if code != 0 {
		t.Fatalf("remove: exit=%d stderr=%q", code, stderr)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRunRemoveForce(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "work")

	if code, _, _ := runEda(t, repo, "", "remove", "topic"); code == 0 {
		t.Fatal("unsafe removal must fail without --force")
	}
	if code, _, stderr := runEda(t, repo, "", "remove", "--force", "topic"); code != 0 {
		t.Fatalf("remove --force: exit=%d stderr=%q", code, stderr)
	}
	assertRemoved(t, repo, wt, "topic")
}

func TestRunStatus(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")

	code, stdout, _ := runEda(t, wt, "", "status")
	if code != 0 {
		t.Fatalf("status: exit = %d", code)
	}
	for _, want := range []string{"branch topic\n", "worktree " + wt + "\n", "primary " + repo + "\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status output missing %q, got %q", want, stdout)
		}
	}
}

func TestRunHookWorktreeCreate(t *testing.T) {
	repo := newTestRepo(t)
	_, root := loadRepoWithRoot(t, repo)

	stdin := `{"session_id":"s","cwd":` + jsonString(repo) + `,"hook_event_name":"WorktreeCreate","name":"agent-abc"}`
	code, stdout, stderr := runEda(t, repo, stdin, "hook", "worktree-create")
	if code != 0 {
		t.Fatalf("hook worktree-create: exit=%d stderr=%q", code, stderr)
	}
	want := canonicalDir(root, repo, "agent-abc") + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunHookWorktreeRemove(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")

	stdin := `{"cwd":` + jsonString(repo) + `,"name":"agent-abc"}`
	code, _, stderr := runEda(t, repo, stdin, "hook", "worktree-remove")
	if code != 0 {
		t.Fatalf("hook worktree-remove: exit=%d stderr=%q", code, stderr)
	}
	assertRemoved(t, repo, wt, "agent-abc")
}

func TestRunHookWorktreeRemoveKeepsUnsafe(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "agent work")

	stdin := `{"cwd":` + jsonString(repo) + `,"name":"agent-abc"}`
	code, _, stderr := runEda(t, repo, stdin, "hook", "worktree-remove")
	if code != 0 {
		t.Fatalf("keeping a worktree is a success for the hook: exit=%d", code)
	}
	if stderr == "" {
		t.Error("the kept worktree must be reported on stderr")
	}
	assertKept(t, repo, wt, "agent-abc")
}

func TestRunUnknownCommand(t *testing.T) {
	repo := newTestRepo(t)
	code, _, stderr := runEda(t, repo, "", "bogus")
	if code == 0 {
		t.Error("unknown command must fail")
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr must include usage, got %q", stderr)
	}
}

func jsonString(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func TestMainBinaryDir(t *testing.T) {
	// run() receives the process working directory from main(); make sure
	// helpers that rely on it resolve relative to that directory.
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runEda(t, sub, "", "root")
	if code != 0 || stdout != repo+"\n" {
		t.Errorf("root from subdirectory: code=%d stdout=%q, want %q", code, stdout, repo+"\n")
	}
}
