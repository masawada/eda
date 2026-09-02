package main

import (
	"os"
	"strings"
	"testing"
)

// oneline returns "<abbrev sha> <subject>" for a revision, the way tree
// labels fork commits.
func oneline(t *testing.T, dir, rev string) string {
	t.Helper()
	return strings.TrimSpace(gitT(t, dir, "log", "-1", "--format=%h %s", rev))
}

func runTree(t *testing.T, cwd string) string {
	t.Helper()
	code, stdout, stderr := runEda(t, cwd, "", "tree")
	if code != 0 {
		t.Fatalf("tree: exit=%d stderr=%q", code, stderr)
	}
	return stdout
}

func TestRunTreeCleanStack(t *testing.T) {
	// Each tip is the fork point of the next branch, so the branches
	// themselves become the inner nodes and no commit rows appear.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a2")
	wtB := mustResolve(t, ctx, wtA, "feat-b")
	gitT(t, wtB, "commit", "-q", "--allow-empty", "-m", "b1")

	want := "main[primary]*\n" +
		"└── feat-a +2\n" +
		"    └── feat-b +1\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeForkCommitWhenBaseMovesOn(t *testing.T) {
	// Once main moves past the fork, the fork commit itself is the shared
	// ancestor and both branches hang off it as siblings.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	fork := oneline(t, repo, "HEAD")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "m1")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "m2")

	want := fork + "\n" +
		"├── feat-a* +1\n" +
		"└── main[primary] +2\n"
	if got := runTree(t, wtA); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeSameTipBranchesShareRow(t *testing.T) {
	// A branch just cut from another sits on the same commit; both are
	// listed on one row instead of inventing a parent-child direction.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	wtB := mustResolve(t, ctx, wtA, "feat-b")

	want := "main[primary]\n" +
		"└── feat-a, feat-b* +1\n"
	if got := runTree(t, wtB); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeIntermediateBranchMovesOn(t *testing.T) {
	// feat-a gains a commit after feat-b was cut from it: the old feat-a tip
	// becomes a commit row showing that feat-b lags behind feat-a.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	fork := oneline(t, wtA, "HEAD")
	wtB := mustResolve(t, ctx, wtA, "feat-b")
	gitT(t, wtB, "commit", "-q", "--allow-empty", "-m", "b1")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a2")

	want := "main[primary]*\n" +
		"└── " + fork + " +1\n" +
		"    ├── feat-a +1\n" +
		"    └── feat-b +1\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreePartialRebaseLeavesOldFork(t *testing.T) {
	// s1 is rebased onto main but s2 is not restacked: s1 moves under main
	// while s2 stays on the old fork, carrying the pre-rebase s1 commit.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	fork := oneline(t, repo, "HEAD")
	wt1 := mustResolve(t, ctx, repo, "s1")
	gitT(t, wt1, "commit", "-q", "--allow-empty", "-m", "s1")
	wt2 := mustResolve(t, ctx, wt1, "s2")
	gitT(t, wt2, "commit", "-q", "--allow-empty", "-m", "s2")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "m1")
	gitT(t, wt1, "rebase", "-q", "main")

	want := fork + "\n" +
		"├── main[primary]* +1\n" +
		"│   └── s1 +1\n" +
		"└── s2 +2\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeMergeMovesForkToMergedTip(t *testing.T) {
	// Merging main into feat-a makes main's tip the shared ancestor, so
	// feat-a now hangs directly off main.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "m1")
	gitT(t, wtA, "merge", "-q", "--no-edit", "main")

	want := "main[primary]*\n" +
		"└── feat-a +2\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeMergeOfTwoTipsAttachesToNearest(t *testing.T) {
	// feat-c merges main into a branch cut from feat-a: both tips are
	// nearest ancestors, and the one fewer commits away wins.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	fork := oneline(t, repo, "HEAD")
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a2")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "m1")
	wtC := mustResolve(t, ctx, wtA, "feat-c")
	gitT(t, wtC, "merge", "-q", "--no-edit", "main")

	want := fork + "\n" +
		"├── feat-a +2\n" +
		"│   └── feat-c +2\n" +
		"└── main[primary]* +1\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeCrissCrossPicksSmallestMergeBase(t *testing.T) {
	// main and feat-a each merge the other's earlier tip, so they have two
	// equally good merge bases. The one with the smaller object id is the
	// fork point, whatever order git would report them in.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "m1")
	a1 := strings.TrimSpace(gitT(t, wtA, "rev-parse", "HEAD"))
	m1 := strings.TrimSpace(gitT(t, repo, "rev-parse", "HEAD"))
	gitT(t, repo, "merge", "-q", "--no-edit", a1)
	gitT(t, wtA, "merge", "-q", "--no-edit", m1)

	fork := oneline(t, repo, min(a1, m1))
	want := fork + "\n" +
		"├── feat-a +2\n" +
		"└── main[primary]* +2\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeForkCommitsOrderedByCommitTime(t *testing.T) {
	// Two stacks fork off main at different commits; the fork rows are
	// ordered by when the fork commit was made, not by object id or by
	// the order the worktrees were created.
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	stack := func(name, date string) string {
		t.Setenv("GIT_COMMITTER_DATE", date)
		wt1 := mustResolve(t, ctx, repo, name+"1")
		gitT(t, wt1, "commit", "-q", "--allow-empty", "-m", name+"1")
		fork := oneline(t, wt1, "HEAD")
		wt2 := mustResolve(t, ctx, wt1, name+"2")
		gitT(t, wt2, "commit", "-q", "--allow-empty", "-m", name+"2")
		gitT(t, wt1, "commit", "-q", "--allow-empty", "-m", name+"1 again")
		return fork
	}
	late := stack("y", "2026-01-01T00:00:02Z")
	early := stack("x", "2026-01-01T00:00:01Z")

	want := "main[primary]*\n" +
		"├── " + early + " +1\n" +
		"│   ├── x1 +1\n" +
		"│   └── x2 +1\n" +
		"└── " + late + " +1\n" +
		"    ├── y1 +1\n" +
		"    └── y2 +1\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreePrunableWorktreeListedAfterForest(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	wtB := mustResolve(t, ctx, repo, "feat-b")
	if err := os.RemoveAll(wtB); err != nil {
		t.Fatal(err)
	}

	want := "main[primary]*\n" +
		"└── feat-a +1\n" +
		"feat-b[prunable]\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeUnrelatedHistoryIsIndependentRoot(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	// Build a branch with no common history via plumbing: an empty tree
	// commit with no parents.
	tree := strings.TrimSpace(gitT(t, repo, "hash-object", "-t", "tree", "-w", "--stdin"))
	commit := strings.TrimSpace(gitT(t, repo, "commit-tree", tree, "-m", "orphan"))
	gitT(t, repo, "branch", "orphan", commit)
	mustResolve(t, ctx, repo, "orphan")

	want := "main[primary]*\n" +
		"orphan\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeDetachedWorktree(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	head := strings.TrimSpace(gitT(t, repo, "rev-parse", "HEAD"))
	wt := t.TempDir() + "/detached-wt"
	gitT(t, repo, "worktree", "add", "-q", "--detach", wt, head)
	gitT(t, wt, "commit", "-q", "--allow-empty", "-m", "d1")
	detachedHead := strings.TrimSpace(gitT(t, wt, "rev-parse", "HEAD"))

	want := "main[primary]*\n" +
		"└── (detached) " + detachedHead[:7] + "[external] +1\n"
	if got := runTree(t, repo); got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeUsageError(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	code, stdout, stderr := runEda(t, repo, "", "tree", "extra")
	if code == 0 {
		t.Error("tree with arguments must fail")
	}
	if stdout != "" {
		t.Errorf("stdout must stay empty on error, got %q", stdout)
	}
	if stderr == "" {
		t.Error("error must be reported on stderr")
	}
}

func TestBuildTreeSkipsUnbornHead(t *testing.T) {
	repo := newTestRepo(t)
	ctx, _ := loadRepoWithRoot(t, repo)
	wtA := mustResolve(t, ctx, repo, "feat-a")
	gitT(t, wtA, "commit", "-q", "--allow-empty", "-m", "a1")
	ctx = reload(t, repo)
	// An unborn branch (e.g. from `git worktree add --orphan`) reports an
	// all-zero object id as HEAD; it must stay out of the topology instead
	// of failing every merge-base call.
	ctx.Entries = append(ctx.Entries, worktreeEntry{
		Path:   "/nonexistent/unborn-wt",
		Head:   strings.Repeat("0", 40),
		Branch: "unborn",
	})

	tr, err := buildTree(ctx)
	if err != nil {
		t.Fatalf("buildTree with unborn entry: %v", err)
	}
	unborn := len(ctx.Entries) - 1
	if len(tr.standalone) != 1 || tr.standalone[0] != unborn {
		t.Errorf("standalone entries = %v, want [%d]", tr.standalone, unborn)
	}
	for _, n := range tr.nodes {
		for _, e := range n.entries {
			if e == unborn {
				t.Errorf("unborn entry must not appear in a node, found in %q", n.commit)
			}
		}
	}
}

func TestRunTreeBarePrimary(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	dir := t.TempDir()
	if _, err := runGit(dir, "init", "-q", "--bare", "-b", "main"); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}

	if got, want := runTree(t, dir), "(bare)[primary]\n"; got != want {
		t.Errorf("tree output = %q, want %q", got, want)
	}
}

func TestRunTreeFailsOnBrokenStdout(t *testing.T) {
	repo := newTestRepo(t)
	loadRepoWithRoot(t, repo)
	var stderr strings.Builder
	if code := run(strings.NewReader(""), failWriter{}, &stderr, []string{"tree"}, repo); code == 0 {
		t.Error("a failed tree write must fail the command, not report success")
	}
}
