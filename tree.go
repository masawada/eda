package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// cmdTree prints where the worktrees of the repository diverged from each
// other: a forest whose nodes are the worktree tips and the commits at
// which they forked. Nothing is inferred about which branch was cut from
// which; the shape follows from commit ancestry alone.
func cmdTree(stdout io.Writer, args []string, cwd string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: eda tree")
	}
	ctx, err := loadRepo(cwd)
	if err != nil {
		return err
	}
	tr, err := buildTree(ctx)
	if err != nil {
		return err
	}
	// The current-location marker is cosmetic: when the invoking directory
	// has no working tree (a bare primary checkout), render without it
	// instead of failing the whole command.
	cwdTop := ""
	if top, code, _, err := runGitExit(cwd, "rev-parse", "--show-toplevel"); err != nil {
		return err
	} else if code == 0 {
		if resolved, err := filepath.EvalSymlinks(strings.TrimSuffix(top, "\n")); err == nil {
			cwdTop = resolved
		}
	}
	return renderTree(stdout, ctx, tr, cwdTop)
}

// treeNode is one commit of the forest: a worktree tip, a fork point, or
// both. entries lists the worktrees whose HEAD is this commit (empty for a
// pure fork point). dist is the number of commits between the parent node
// and this one.
type treeNode struct {
	commit   string
	entries  []int
	parent   int
	dist     int
	children []int
}

// worktreeTree is the forest over the worktree entries of a repository.
// standalone lists entries that carry no topology (bare, prunable, unborn)
// and are shown as rows of their own.
type worktreeTree struct {
	nodes      []treeNode
	roots      []int
	standalone []int
}

// buildTree computes the forest. The node set is the distinct worktree
// tips plus the merge base of every pair of nodes; the parent of a node is
// its nearest ancestor within that set. Git history is not always a tree:
// a criss-cross merge leaves several equally good merge bases (the one
// with the smallest object id is taken), and a node that merged two tips
// has both as nearest ancestors (the one fewer commits away wins, then
// the smaller object id). Both rules only decide how a DAG is drawn as a
// tree, deterministically, without pretending the history is one.
func buildTree(ctx *repoContext) (*worktreeTree, error) {
	tr := &worktreeTree{}
	index := map[string]int{}
	node := func(commit string) int {
		if i, ok := index[commit]; ok {
			return i
		}
		index[commit] = len(tr.nodes)
		tr.nodes = append(tr.nodes, treeNode{commit: commit, parent: -1})
		return index[commit]
	}
	for i, e := range ctx.Entries {
		if e.Bare || e.Prunable || !hasCommit(e.Head) {
			tr.standalone = append(tr.standalone, i)
			continue
		}
		n := node(e.Head)
		tr.nodes[n].entries = append(tr.nodes[n].entries, i)
	}
	// Every pair of nodes is compared once; the merge bases found on the
	// way join the node set and are compared in turn, so the loop ends
	// with a set closed under merge-base. The same call tells whether one
	// node is an ancestor of the other: then it is the only merge base.
	ancestor := map[[2]int]bool{}
	for j := 0; j < len(tr.nodes); j++ {
		for i := 0; i < j; i++ {
			mb, related, err := mergeBase(ctx.PrimaryPath, tr.nodes[i].commit, tr.nodes[j].commit)
			if err != nil {
				return nil, err
			}
			if !related {
				continue
			}
			k := node(mb)
			ancestor[[2]int{i, j}] = k == i
			ancestor[[2]int{j, i}] = k == j
		}
	}
	for j := range tr.nodes {
		best := -1
		for i := range tr.nodes {
			if !ancestor[[2]int{i, j}] {
				continue
			}
			nearest := true
			for k := range tr.nodes {
				if ancestor[[2]int{i, k}] && ancestor[[2]int{k, j}] {
					nearest = false
					break
				}
			}
			if !nearest {
				continue
			}
			dist, err := countCommits(ctx.PrimaryPath, tr.nodes[i].commit, tr.nodes[j].commit)
			if err != nil {
				return nil, err
			}
			cur := &tr.nodes[j]
			if best == -1 || dist < cur.dist || (dist == cur.dist && tr.nodes[i].commit < tr.nodes[best].commit) {
				best, cur.parent, cur.dist = i, i, dist
			}
		}
		if best == -1 {
			tr.roots = append(tr.roots, j)
		} else {
			tr.nodes[best].children = append(tr.nodes[best].children, j)
		}
	}
	return tr, nil
}

// hasCommit reports whether a porcelain HEAD value names a real commit.
// Bare entries have no HEAD at all, and an unborn branch (e.g. from
// `git worktree add --orphan`) reports the all-zero object id, which no
// revision walk accepts.
func hasCommit(head string) bool {
	return head != "" && strings.Trim(head, "0") != ""
}

// mergeBase returns the merge base of two commits. When several are
// equally good (criss-cross merges) git leaves the choice unspecified, so
// the one with the smallest object id is taken to keep the result stable.
// related is false when the two histories share nothing (git merge-base
// exits 1).
func mergeBase(dir, a, b string) (mb string, related bool, err error) {
	out, code, stderrMsg, err := runGitExit(dir, "merge-base", "--all", a, b)
	if err != nil {
		return "", false, err
	}
	switch code {
	case 0:
		bases := strings.Fields(out)
		if len(bases) == 0 {
			return "", false, fmt.Errorf("git merge-base --all %s %s: no output", a, b)
		}
		return slices.Min(bases), true, nil
	case 1:
		return "", false, nil
	default:
		if stderrMsg == "" {
			stderrMsg = fmt.Sprintf("exit status %d", code)
		}
		return "", false, fmt.Errorf("git merge-base: %s", stderrMsg)
	}
}

// countCommits returns how many commits of b are not reachable from a.
func countCommits(dir, a, b string) (int, error) {
	out, err := runGit(dir, "rev-list", "--count", a+".."+b)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("parse rev-list count: %w", err)
	}
	return n, nil
}

func renderTree(w io.Writer, ctx *repoContext, tr *worktreeTree, cwdTop string) error {
	labels := make([]string, len(tr.nodes))
	keys := make([]string, len(tr.nodes))
	for i := range tr.nodes {
		entries := tr.nodes[i].entries
		sort.Slice(entries, func(a, b int) bool {
			return entryName(ctx, entries[a]) < entryName(ctx, entries[b])
		})
		label, key, err := nodeLabel(ctx, tr.nodes[i], cwdTop)
		if err != nil {
			return err
		}
		labels[i], keys[i] = label, key
	}
	sortSiblings := func(s []int) {
		sort.Slice(s, func(a, b int) bool { return keys[s[a]] < keys[s[b]] })
	}
	sortSiblings(tr.roots)
	for i := range tr.nodes {
		sortSiblings(tr.nodes[i].children)
	}
	var subtree func(i int, linePrefix, childPrefix string) error
	subtree = func(i int, linePrefix, childPrefix string) error {
		if _, err := fmt.Fprintln(w, linePrefix+labels[i]); err != nil {
			return err
		}
		for k, c := range tr.nodes[i].children {
			connector, indent := "├── ", "│   "
			if k == len(tr.nodes[i].children)-1 {
				connector, indent = "└── ", "    "
			}
			if err := subtree(c, childPrefix+connector, childPrefix+indent); err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range tr.roots {
		if err := subtree(r, "", ""); err != nil {
			return err
		}
	}
	standalone := append([]int(nil), tr.standalone...)
	sort.Slice(standalone, func(a, b int) bool {
		return entryName(ctx, standalone[a]) < entryName(ctx, standalone[b])
	})
	for _, e := range standalone {
		if _, err := fmt.Fprintln(w, entryLabel(ctx, e, cwdTop)); err != nil {
			return err
		}
	}
	return nil
}

// nodeLabel renders one node: the worktrees sitting on the commit (or the
// commit itself for a pure fork point), then the distance from the parent.
// It also returns the key that orders siblings. Porcelain lists linked
// worktrees in path order, which is a random name here — meaningless to a
// reader — so worktree rows come first by name, and fork commits follow in
// the order they were committed (object id as the tie-breaker).
func nodeLabel(ctx *repoContext, n treeNode, cwdTop string) (label, key string, err error) {
	if len(n.entries) == 0 {
		out, err := runGit(ctx.PrimaryPath, "log", "-1", "--format=%ct%n%h %s", n.commit)
		if err != nil {
			return "", "", err
		}
		stamp, oneline, _ := strings.Cut(strings.TrimSpace(out), "\n")
		label = oneline
		key = fmt.Sprintf("1%020s%s", stamp, n.commit)
	} else {
		parts := make([]string, 0, len(n.entries))
		for _, e := range n.entries {
			parts = append(parts, entryLabel(ctx, e, cwdTop))
		}
		label = strings.Join(parts, ", ")
		key = "0" + entryName(ctx, n.entries[0])
	}
	if n.parent != -1 {
		label += fmt.Sprintf(" +%d", n.dist)
	}
	return label, key, nil
}

// entryName is the display name of a worktree entry: the branch, or a
// placeholder for bare and detached worktrees.
func entryName(ctx *repoContext, i int) string {
	e := ctx.Entries[i]
	if e.Bare {
		return "(bare)"
	}
	if e.Detached {
		head := e.Head
		if len(head) > 7 {
			head = head[:7]
		}
		return "(detached) " + head
	}
	return e.Branch
}

// entryLabel renders one worktree: its name, the same notes eda list
// prints, and a marker on the worktree the command ran in. Notes and the
// marker stick to the name so that a row listing several worktrees stays
// unambiguous.
func entryLabel(ctx *repoContext, i int, cwdTop string) string {
	e := ctx.Entries[i]
	label := entryName(ctx, i)
	var notes []string
	if i == 0 {
		notes = append(notes, "primary")
	} else if !managedWorktree(ctx, e.Path) {
		notes = append(notes, "external")
	}
	if e.Locked {
		notes = append(notes, "locked")
	}
	if e.Prunable {
		notes = append(notes, "prunable")
	}
	if len(notes) > 0 {
		label += "[" + strings.Join(notes, ",") + "]"
	}
	if e.Path == cwdTop {
		label += "*"
	}
	return label
}
