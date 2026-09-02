package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// copyWorktreeInclude copies files from src into dst according to the
// .worktreeinclude file at the root of src. The file uses .gitignore syntax,
// and only files that both match a pattern and are gitignored are copied, so
// tracked files are never duplicated. Only regular files are copied; symbolic
// links are skipped rather than followed. This mirrors Claude Code's native
// .worktreeinclude, which is not processed when a WorktreeCreate hook is
// configured, and it also serves worktrees humans create with `eda switch`.
func copyWorktreeInclude(src, dst string) error {
	includeFile := filepath.Join(src, ".worktreeinclude")
	if _, err := os.Stat(includeFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Let git do the .gitignore-syntax matching: intersect the untracked
	// files that are ignored with the untracked files matching the
	// .worktreeinclude patterns.
	ignored, err := lsFiles(src, "--others", "--ignored", "--exclude-standard")
	if err != nil {
		return err
	}
	matched, err := lsFiles(src, "--others", "--ignored", "--exclude-from=.worktreeinclude")
	if err != nil {
		return err
	}
	ignoredSet := make(map[string]bool, len(ignored))
	for _, f := range ignored {
		ignoredSet[f] = true
	}
	var candidates []string
	for _, rel := range matched {
		if ignoredSet[rel] {
			candidates = append(candidates, rel)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	// Preflight against the destination: the checked-out branch may track a
	// candidate path (never overwrite it) or no longer ignore it (copying
	// would create an immediately dirty worktree).
	tracked, err := lsFiles(dst)
	if err != nil {
		return err
	}
	trackedSet := make(map[string]bool, len(tracked))
	for _, f := range tracked {
		trackedSet[f] = true
	}
	dstIgnored, err := checkIgnore(dst, candidates)
	if err != nil {
		return err
	}
	for _, rel := range candidates {
		if trackedSet[rel] || !dstIgnored[rel] {
			continue
		}
		if err := copyFile(filepath.Join(src, rel), filepath.Join(dst, rel)); err != nil {
			return fmt.Errorf("copy %s: %w", rel, err)
		}
	}
	return nil
}

// checkIgnore reports which of the given relative paths are gitignored in
// dir, using one `git check-ignore` invocation.
func checkIgnore(dir string, rels []string) (map[string]bool, error) {
	input := strings.Join(rels, "\x00") + "\x00"
	out, code, stderrMsg, err := runGitExitInput(dir, input, "check-ignore", "--stdin", "-z")
	if err != nil {
		return nil, err
	}
	// Exit 1 means no path is ignored; anything above is an error.
	if code > 1 {
		return nil, fmt.Errorf("git check-ignore: exit status %d: %s", code, stderrMsg)
	}
	result := make(map[string]bool, len(rels))
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			result[f] = true
		}
	}
	return result, nil
}

// lsFiles runs `git ls-files -z` with the given options and returns the
// relative paths.
func lsFiles(dir string, opts ...string) ([]string, error) {
	args := append([]string{"ls-files", "-z"}, opts...)
	out, err := runGit(dir, args...)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

func copyFile(src, dst string) error {
	// Lstat so that symlinks are seen as such and skipped: the copy never
	// follows a link, and a dangling one cannot fail the whole copy.
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	// Exclusive creation: even after the preflight, never clobber a file
	// (or follow a symlink) that appeared at the destination.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
