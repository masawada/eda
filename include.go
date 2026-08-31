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
// tracked files are never duplicated. This mirrors Claude Code's native
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
	for _, rel := range matched {
		if !ignoredSet[rel] {
			continue
		}
		if err := copyFile(filepath.Join(src, rel), filepath.Join(dst, rel)); err != nil {
			return fmt.Errorf("copy %s: %w", rel, err)
		}
	}
	return nil
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
	info, err := os.Stat(src)
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
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
