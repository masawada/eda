package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const usage = `usage: eda <command> [arguments]

commands:
  switch <branch>          resolve or create the worktree for a branch and print its path
  path <branch>            print the worktree path for a branch (never creates)
  list                     list worktrees of the current repository
  remove [--force] <branch>  remove a worktree and its branch as a pair
  root                     print the primary checkout path
  status                   print current repository, worktree, and branch
  init - <shell>           print the shell integration script (zsh, bash)
  hook worktree-create     Claude Code WorktreeCreate hook entrypoint (stdin JSON)
  hook worktree-remove     Claude Code WorktreeRemove hook entrypoint (stdin JSON)
`

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "eda: %v\n", err)
		os.Exit(1)
	}
	os.Exit(run(os.Stdin, os.Stdout, os.Stderr, os.Args[1:], cwd))
}

// run dispatches subcommands. Commands that print a path emit exactly one
// absolute path line on stdout and nothing else; all diagnostics go to
// stderr. Shell integration and the Claude Code hooks rely on this contract.
func run(stdin io.Reader, stdout, stderr io.Writer, args []string, cwd string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "switch":
		err = cmdSwitch(stdout, rest, cwd)
	case "path":
		err = cmdPath(stdout, rest, cwd)
	case "list":
		err = cmdList(stdout, rest, cwd)
	case "remove":
		err = cmdRemove(stderr, rest, cwd)
	case "root":
		err = cmdRoot(stdout, rest, cwd)
	case "status":
		err = cmdStatus(stdout, rest, cwd)
	case "init":
		err = cmdInit(stdout, rest)
	case "hook":
		err = cmdHook(stdin, stdout, stderr, rest, cwd)
	default:
		fmt.Fprintf(stderr, "eda: unknown command %q\n%s", cmd, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "eda: %v\n", err)
		return 1
	}
	return 0
}

func singleBranchArg(cmd string, args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: eda %s <branch>", cmd)
	}
	return args[0], nil
}

// printPath writes the single path line that shell integration and the
// hooks consume. A write failure must fail the command: reporting success
// with a missing or partial path would break the stdout contract.
func printPath(w io.Writer, path string) error {
	if _, err := fmt.Fprintln(w, path); err != nil {
		return fmt.Errorf("write path: %w", err)
	}
	return nil
}

func cmdSwitch(stdout io.Writer, args []string, cwd string) error {
	branch, err := singleBranchArg("switch", args)
	if err != nil {
		return err
	}
	ctx, err := loadRepo(cwd)
	if err != nil {
		return err
	}
	dir, err := resolveWorktree(ctx, cwd, branch)
	if err != nil {
		return err
	}
	return printPath(stdout, dir)
}

func cmdPath(stdout io.Writer, args []string, cwd string) error {
	branch, err := singleBranchArg("path", args)
	if err != nil {
		return err
	}
	ctx, err := loadRepo(cwd)
	if err != nil {
		return err
	}
	for _, e := range ctx.Entries {
		if !e.Bare && !e.Detached && e.Branch == branch {
			if e.Prunable {
				return fmt.Errorf("worktree registration for %q at %s is stale; run `git worktree prune`", branch, e.Path)
			}
			return printPath(stdout, e.Path)
		}
	}
	return fmt.Errorf("no worktree for branch %q", branch)
}

func cmdList(stdout io.Writer, args []string, cwd string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: eda list")
	}
	ctx, err := loadRepo(cwd)
	if err != nil {
		return err
	}
	for i, e := range ctx.Entries {
		name := e.Branch
		var notes []string
		if i == 0 {
			notes = append(notes, "primary")
		} else if !managedWorktree(ctx, e.Path) {
			notes = append(notes, "external")
		}
		if e.Bare {
			name = "(bare)"
		}
		if e.Detached {
			name = "(detached)"
		}
		if e.Locked {
			notes = append(notes, "locked")
		}
		if e.Prunable {
			notes = append(notes, "prunable")
		}
		line := fmt.Sprintf("%s\t%s", name, e.Path)
		if len(notes) > 0 {
			line += "\t[" + strings.Join(notes, ",") + "]"
		}
		fmt.Fprintln(stdout, line)
	}
	return nil
}

func cmdRemove(stderr io.Writer, args []string, cwd string) error {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "remove even when the worktree is dirty or the branch has unmerged commits")
	if err := fs.Parse(args); err != nil {
		return err
	}
	branch, err := singleBranchArg("remove [--force]", fs.Args())
	if err != nil {
		return err
	}
	ctx, err := loadRepo(cwd)
	if err != nil {
		return err
	}
	return removeWorktree(ctx, branch, *force)
}

func cmdRoot(stdout io.Writer, args []string, cwd string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: eda root")
	}
	ctx, err := loadRepo(cwd)
	if err != nil {
		return err
	}
	return printPath(stdout, ctx.PrimaryPath)
}

func cmdStatus(stdout io.Writer, args []string, cwd string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: eda status")
	}
	ctx, err := loadRepo(cwd)
	if err != nil {
		return err
	}
	top, err := runGit(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	branch, err := runGit(cwd, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "primary %s\n", ctx.PrimaryPath)
	fmt.Fprintf(stdout, "worktree %s\n", strings.TrimSpace(top))
	fmt.Fprintf(stdout, "branch %s\n", strings.TrimSpace(branch))
	return nil
}

// hookInput is the JSON Claude Code writes to worktree hooks. The cwd field
// is the session's current directory; it decides the base of new branches so
// a subagent spawned inside a worktree stacks on that worktree. The path
// field is speculative for WorktreeRemove, whose real schema could not be
// observed in the step0 spike (the hook never fired for hook-created
// worktrees).
type hookInput struct {
	Name string `json:"name"`
	Cwd  string `json:"cwd"`
	Path string `json:"path"`
}

func cmdHook(stdin io.Reader, stdout, stderr io.Writer, args []string, cwd string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: eda hook <worktree-create|worktree-remove>")
	}
	body, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read hook input: %w", err)
	}
	var in hookInput
	if err := json.Unmarshal(body, &in); err != nil {
		return fmt.Errorf("decode hook input: %w", err)
	}
	// Fall back to the hook process working directory when cwd is absent;
	// both were observed to be the session directory in the step0 spike.
	dir := in.Cwd
	if dir == "" {
		dir = cwd
	}

	switch args[0] {
	case "worktree-create":
		if in.Name == "" {
			return fmt.Errorf("hook input has no worktree name")
		}
		ctx, err := loadRepo(dir)
		if err != nil {
			return err
		}
		wt, err := resolveWorktree(ctx, dir, in.Name)
		if err != nil {
			return err
		}
		return printPath(stdout, wt)
	case "worktree-remove":
		ctx, err := loadRepo(dir)
		if err != nil {
			return err
		}
		branch := in.Name
		if branch == "" && in.Path != "" {
			for _, e := range ctx.Entries {
				if e.Path == in.Path {
					branch = e.Branch
					break
				}
			}
		}
		if branch == "" {
			return fmt.Errorf("hook input identifies no worktree (name and path are empty or unknown)")
		}
		if err := removeWorktree(ctx, branch, false); err != nil {
			var refusal refusalError
			if errors.As(err, &refusal) {
				// Keeping a worktree that still holds work is a valid
				// outcome for the hook, not a failure.
				fmt.Fprintf(stderr, "eda: worktree kept: %v\n", err)
				return nil
			}
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown hook %q", args[0])
	}
}
