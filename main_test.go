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
	dir, ok := strings.CutSuffix(stdout, "\n")
	if !ok || strings.Contains(dir, "\n") {
		t.Fatalf("stdout = %q, want an absolute path, one line, nothing else", stdout)
	}
	assertWorktreeDir(t, root, repo, dir)
	if got := strings.TrimSpace(gitT(t, dir, "rev-parse", "--abbrev-ref", "HEAD")); got != "feature-a" {
		t.Errorf("printed worktree HEAD = %q, want feature-a", got)
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

func TestRunListAlignsColumns(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "a-much-longer-branch-name")
	// EvalSymlinks matches what git prints (/tmp vs /private/tmp on macOS).
	extBase, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ext := filepath.Join(extBase, "manual-worktree-with-a-longer-name")
	gitT(t, repo, "worktree", "add", "-q", "-b", "manual", ext)
	gitT(t, repo, "worktree", "lock", ext)

	code, stdout, _ := runEda(t, repo, "", "list")
	if code != 0 {
		t.Fatalf("list: exit = %d", code)
	}
	if strings.Contains(stdout, "\t") {
		t.Errorf("list output must be space-aligned, not tab-separated, got %q", stdout)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("list must print one line per worktree, got %q", stdout)
	}
	// Notes sit right after the branch name, not in a trailing column where
	// long paths would push them out of sight.
	if !strings.HasPrefix(lines[0], "main[primary] ") {
		t.Errorf("line must start with the annotated name, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[2], "manual[external,locked] ") {
		t.Errorf("line must start with the annotated name, got %q", lines[2])
	}
	column := func(line, cell string) int {
		t.Helper()
		i := strings.Index(line, cell)
		if i < 0 {
			t.Fatalf("line %q must contain %q", line, cell)
		}
		return i
	}
	if column(lines[0], repo) != column(lines[1], wt) || column(lines[1], wt) != column(lines[2], ext) {
		t.Errorf("paths must start at the same column:\n%s", stdout)
	}
	for _, line := range lines {
		if strings.TrimRight(line, " ") != line {
			t.Errorf("line must not have trailing spaces, got %q", line)
		}
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

func TestRunRemoveMultiple(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "topic-a")
	wtB := mustResolve(t, ctx, repo, "topic-b")

	code, _, stderr := runEda(t, repo, "", "remove", "topic-a", "topic-b")
	if code != 0 {
		t.Fatalf("remove: exit=%d stderr=%q", code, stderr)
	}
	if stderr != "" {
		t.Errorf("stderr must stay empty on success, got %q", stderr)
	}
	assertRemoved(t, repo, wtA, "topic-a")
	assertRemoved(t, repo, wtB, "topic-b")
}

func TestRunRemoveMultipleBestEffort(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "topic-a")
	wtB := mustResolve(t, ctx, repo, "topic-b")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "unique work")

	// topic-a is refused but topic-b must still be removed.
	code, _, stderr := runEda(t, repo, "", "remove", "topic-a", "topic-b")
	if code == 0 {
		t.Fatal("a refused branch must fail the command")
	}
	assertKept(t, repo, wtA, "topic-a")
	assertRemoved(t, repo, wtB, "topic-b")
	if !strings.Contains(stderr, "remove topic-a:") {
		t.Errorf("stderr must name the refused branch, got %q", stderr)
	}
	if !strings.Contains(stderr, "failed to remove 1 of 2") {
		t.Errorf("stderr must summarize the failures, got %q", stderr)
	}
}

func TestRunRemoveMultipleFromInvokingWorktree(t *testing.T) {
	repo := newTestClone(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "work on a")
	gitT(t, wtA, "push", "-q", "-u", "origin", "a")
	wtB := mustResolve(t, reload(t, repo), wtA, "b")

	// Run from worktree a: a passes through its upstream, and b (no
	// upstream, tip reachable only from the HEAD of a) must still be judged
	// against the HEAD the command started with, even though removing a
	// deleted that directory and branch.
	code, _, stderr := runEda(t, wtA, "", "remove", "a", "b")
	if code != 0 {
		t.Fatalf("remove: exit=%d stderr=%q", code, stderr)
	}
	assertRemoved(t, repo, wtA, "a")
	assertRemoved(t, repo, wtB, "b")
}

func TestRunRemoveFromInsideViaSymlink(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "unique work")

	// Entered through a symlink, the invoking directory must still be
	// recognized as the worktree being removed, so the primary HEAD is the
	// base and not the worktree's own HEAD.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(wt, link); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runEda(t, link, "", "remove", "topic")
	if code == 0 {
		t.Fatal("unmerged branch must be refused from inside its own worktree")
	}
	if !strings.Contains(stderr, "the HEAD of the primary checkout") {
		t.Errorf("stderr must name the primary HEAD as the base, got %q", stderr)
	}
	assertKept(t, repo, wt, "topic")
}

func TestRunRemoveDuplicateBranch(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")

	// The second occurrence must see the state after the first removal and
	// be refused as unknown, not operate on the already-deleted path.
	code, _, stderr := runEda(t, repo, "", "remove", "topic", "topic")
	if code == 0 {
		t.Fatal("duplicate branch must fail the command")
	}
	assertRemoved(t, repo, wt, "topic")
	if !strings.Contains(stderr, `no worktree found for branch "topic"`) {
		t.Errorf("stderr must report the branch as unknown, got %q", stderr)
	}
	if !strings.Contains(stderr, "failed to remove 1 of 2") {
		t.Errorf("stderr must summarize the failures, got %q", stderr)
	}
}

func TestRunRemoveSingleKeepsErrorFormat(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "topic")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "unique work")

	// A single branch keeps the one-line error without the per-branch
	// prefix or the summary line.
	code, _, stderr := runEda(t, repo, "", "remove", "topic")
	if code == 0 {
		t.Fatal("unsafe removal must fail")
	}
	if strings.Contains(stderr, "remove topic:") || strings.Contains(stderr, "failed to remove") {
		t.Errorf("single-branch error must keep the plain format, got %q", stderr)
	}
	assertKept(t, repo, wt, "topic")
}

func TestRunRemoveNoBranch(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	code, _, stderr := runEda(t, repo, "", "remove")
	if code == 0 {
		t.Error("remove without branches must fail")
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr must include usage, got %q", stderr)
	}
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
	// Labels are padded to a common width so the values line up.
	for _, want := range []string{"branch   topic\n", "worktree " + wt + "\n", "primary  " + repo + "\n"} {
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
	dir, ok := strings.CutSuffix(stdout, "\n")
	if !ok || strings.Contains(dir, "\n") {
		t.Fatalf("stdout = %q, want an absolute path, one line, nothing else", stdout)
	}
	assertWorktreeDir(t, root, repo, dir)
	if got := strings.TrimSpace(gitT(t, dir, "rev-parse", "--abbrev-ref", "HEAD")); got != "agent-abc" {
		t.Errorf("printed worktree HEAD = %q, want agent-abc", got)
	}
}

// removeHookInput builds the WorktreeRemove payload: the common fields plus
// worktree_path, the path the create hook returned. There is no name field.
func removeHookInput(cwd, worktreePath string) string {
	return `{"session_id":"s","cwd":` + jsonString(cwd) + `,"hook_event_name":"WorktreeRemove","worktree_path":` + jsonString(worktreePath) + `}`
}

func TestRunHookWorktreeRemove(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")

	code, _, stderr := runEda(t, repo, removeHookInput(repo, wt), "hook", "worktree-remove")
	if code != 0 {
		t.Fatalf("hook worktree-remove: exit=%d stderr=%q", code, stderr)
	}
	assertRemoved(t, repo, wt, "agent-abc")
}

func TestRunHookWorktreeRemoveUnknownPath(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")

	// An existing directory that is not a worktree, one that does not exist
	// at all, and a directory inside the worktree (which locates the
	// repository but is not the worktree itself) must all fail without
	// touching anything.
	sub := filepath.Join(wt, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, unknown := range []string{t.TempDir(), filepath.Join(t.TempDir(), "missing"), sub} {
		code, _, stderr := runEda(t, repo, removeHookInput(repo, unknown), "hook", "worktree-remove")
		if code != 1 {
			t.Fatalf("unknown worktree_path %q: exit=%d, want 1 (stderr=%q)", unknown, code, stderr)
		}
		if !strings.Contains(stderr, unknown) {
			t.Errorf("stderr must name the path %q, got %q", unknown, stderr)
		}
	}
	assertKept(t, repo, wt, "agent-abc")
}

func TestRunHookWorktreeRemoveEmptyPath(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")

	stdin := `{"session_id":"s","cwd":` + jsonString(repo) + `,"hook_event_name":"WorktreeRemove"}`
	code, _, stderr := runEda(t, repo, stdin, "hook", "worktree-remove")
	if code != 1 {
		t.Fatalf("missing worktree_path: exit=%d, want 1 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stderr, "worktree_path") {
		t.Errorf("stderr must name the missing field, got %q", stderr)
	}
	assertKept(t, repo, wt, "agent-abc")
}

func TestRunHookWorktreeRemoveDetached(t *testing.T) {
	repo := newTestRepo(t)
	_, root := loadRepoWithRoot(t, repo)
	wt := filepath.Join(root, "detached")
	gitT(t, repo, "worktree", "add", "-q", "--detach", wt)

	code, _, stderr := runEda(t, repo, removeHookInput(repo, wt), "hook", "worktree-remove")
	if code != 1 {
		t.Fatalf("detached worktree has no branch to remove: exit=%d, want 1 (stderr=%q)", code, stderr)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("detached worktree %q must be kept: %v", wt, err)
	}
}

func TestRunHookWorktreeRemoveSymlinkedPath(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")

	// git records realpaths; a path reaching the worktree through a symlink
	// must still identify it.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Dir(wt), link); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runEda(t, repo, removeHookInput(repo, filepath.Join(link, filepath.Base(wt))), "hook", "worktree-remove")
	if code != 0 {
		t.Fatalf("hook worktree-remove via symlink: exit=%d stderr=%q", code, stderr)
	}
	assertRemoved(t, repo, wt, "agent-abc")
}

func TestRunHookWorktreeRemoveKeepsUnsafe(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "agent work")

	code, _, stderr := runEda(t, repo, removeHookInput(repo, wt), "hook", "worktree-remove")
	if code != 0 {
		t.Fatalf("keeping a worktree is a success for the hook: exit=%d", code)
	}
	if stderr == "" {
		t.Error("the kept worktree must be reported on stderr")
	}
	assertKept(t, repo, wt, "agent-abc")
}

func TestRunHookWorktreeRemoveUsesPrimaryHead(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "agent-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "work on a")
	wtB := mustResolve(t, reload(t, repo), wtA, "agent-b")

	// The session cwd is worktree a, whose HEAD reaches the tip of b; the
	// hook must judge against the primary HEAD instead and keep b.
	code, _, stderr := runEda(t, repo, removeHookInput(wtA, wtB), "hook", "worktree-remove")
	if code != 0 {
		t.Fatalf("keeping a worktree is a success for the hook: exit=%d stderr=%q", code, stderr)
	}
	if stderr == "" {
		t.Error("the kept worktree must be reported on stderr")
	}
	assertKept(t, repo, wtB, "agent-b")
}

func TestRunHookWorktreeRemoveFromOtherDirectory(t *testing.T) {
	// The session cwd follows `cd`, so by the time the session ends it may
	// be in another repository or outside any; worktree_path alone must
	// identify the worktree. The hook process cwd is outside as well.
	for name, cwd := range map[string]string{"other repository": newTestRepo(t), "no repository": t.TempDir()} {
		t.Run(name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx, _ := loadRepoWithRoot(t, repo)
			wt := mustResolve(t, ctx, repo, "agent-abc")

			code, _, stderr := runEda(t, cwd, removeHookInput(cwd, wt), "hook", "worktree-remove")
			if code != 0 {
				t.Fatalf("hook worktree-remove: exit=%d stderr=%q", code, stderr)
			}
			assertRemoved(t, repo, wt, "agent-abc")
		})
	}
}

func TestRunHookWorktreeRemoveWithoutCwd(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")

	stdin := `{"session_id":"s","hook_event_name":"WorktreeRemove","worktree_path":` + jsonString(wt) + `}`
	code, _, stderr := runEda(t, t.TempDir(), stdin, "hook", "worktree-remove")
	if code != 0 {
		t.Fatalf("hook worktree-remove without cwd: exit=%d stderr=%q", code, stderr)
	}
	assertRemoved(t, repo, wt, "agent-abc")
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

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, os.ErrClosed
}

func TestRunRootFailsOnBrokenStdout(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	var stderr strings.Builder
	if code := run(strings.NewReader(""), failWriter{}, &stderr, []string{"root"}, repo); code == 0 {
		t.Error("a failed path write must fail the command, not report success")
	}
}

func TestRunListFailsOnBrokenStdout(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	var stderr strings.Builder
	if code := run(strings.NewReader(""), failWriter{}, &stderr, []string{"list"}, repo); code == 0 {
		t.Error("a failed list write must fail the command, not report success")
	}
}

func TestRunStatusFailsOnBrokenStdout(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	var stderr strings.Builder
	if code := run(strings.NewReader(""), failWriter{}, &stderr, []string{"status"}, repo); code == 0 {
		t.Error("a failed status write must fail the command, not report success")
	}
}

func TestRunInitFailsOnBrokenStdout(t *testing.T) {
	repo := newTestRepo(t)
	var stderr strings.Builder
	if code := run(strings.NewReader(""), failWriter{}, &stderr, []string{"init", "-", "zsh"}, repo); code == 0 {
		t.Error("a failed script write must fail the command, not report success")
	}
}

func TestRunHookWorktreeRemoveKeptReportFails(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wt := mustResolve(t, ctx, repo, "agent-abc")
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "agent work")

	var stdout strings.Builder
	if code := run(strings.NewReader(removeHookInput(repo, wt)), &stdout, failWriter{}, []string{"hook", "worktree-remove"}, repo); code == 0 {
		t.Error("an undeliverable kept-report must fail the hook, not report success")
	}
	assertKept(t, repo, wt, "agent-abc")
}

func TestRunListMarksExternalWorktree(t *testing.T) {
	repo := newTestRepo(t)
	wt := filepath.Join(t.TempDir(), "manual-wt")
	gitT(t, repo, "worktree", "add", "-q", "-b", "topic", wt)
	loadRepoWithRoot(t, repo)

	_, stdout, _ := runEda(t, repo, "", "list")
	if !strings.Contains(stdout, "external") {
		t.Errorf("a worktree outside the root must be marked external, got %q", stdout)
	}
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
