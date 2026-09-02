package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// bashCompletion runs _eda from the bash script for `eda switch <cur>` in
// repo and returns the COMPREPLY entries.
func bashCompletion(t *testing.T, repo, cur string) []string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	scriptFile := filepath.Join(t.TempDir(), "eda.bash")
	if err := os.WriteFile(scriptFile, []byte(bashScript), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "source " + shellQuote(scriptFile) + "\n" +
		"cd " + shellQuote(repo) + "\n" +
		"COMP_WORDS=(eda switch " + shellQuote(cur) + ")\n" +
		"COMP_CWORD=2\n" +
		"_eda\n" +
		`printf '%s\n' "${COMPREPLY[@]}"`
	out, err := exec.Command("bash", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("completion run failed: %v\n%s", err, out)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
}

// bashEval returns what bash makes of entry when it is inserted on the
// command line, i.e. after quote removal and expansions.
func bashEval(t *testing.T, entry string) string {
	t.Helper()
	out, err := exec.Command("bash", "-c", "v="+entry+"\nprintf '%s' \"$v\"").CombinedOutput()
	if err != nil {
		t.Fatalf("eval of completion entry %q failed: %v\n%s", entry, err, out)
	}
	return string(out)
}

// TestBashCompletionKeepsRefNamesLiteral guards against feeding ref names to
// compgen -W, which expands its word list and would execute code embedded in
// a hostile branch name (fetched refs are attacker-controlled). The entry is
// shell-quoted, so it must round-trip through eval to the literal name.
func TestBashCompletionKeepsRefNamesLiteral(t *testing.T) {
	repo := newTestRepo(t)
	marker := filepath.Join(t.TempDir(), "pwned")
	evil := "x-$(true>" + marker + ")"
	gitT(t, repo, "branch", evil)

	entries := bashCompletion(t, repo, "x")
	found := false
	for _, entry := range entries {
		if bashEval(t, entry) == evil {
			found = true
		}
	}
	if !found {
		t.Errorf("completion must offer the branch name, got:\n%s", strings.Join(entries, "\n"))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("completion executed code embedded in a ref name (marker %s exists)", marker)
	}
}

// TestBashCompletionQuotesRefNames guards against inserting ref names
// unquoted: bash puts function-generated completions on the command line
// verbatim, so an unquoted `$(...)` in a branch name would run on Enter.
// Names without special characters must stay untouched.
func TestBashCompletionQuotesRefNames(t *testing.T) {
	repo := newTestRepo(t)
	marker := filepath.Join(t.TempDir(), "pwned")
	// ${IFS} stands in for the space that a ref name may not contain.
	evil := "x-$(touch${IFS}" + marker + ")"
	gitT(t, repo, "branch", evil)
	gitT(t, repo, "branch", "feat/a")

	entries := bashCompletion(t, repo, "")
	want := map[string]bool{"main": false, "feat/a": false, evil: false}
	plain := false
	for _, entry := range entries {
		if entry == "feat/a" {
			plain = true
		}
		got := bashEval(t, entry)
		if _, ok := want[got]; !ok {
			t.Errorf("entry %q evaluates to unexpected %q", entry, got)
		}
		want[got] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("completion must offer %q, got:\n%s", name, strings.Join(entries, "\n"))
		}
	}
	if !plain {
		t.Errorf("plain name feat/a must be emitted unchanged, got:\n%s", strings.Join(entries, "\n"))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("inserting the completion executed code embedded in a ref name (marker %s exists)", marker)
	}
}

// TestWrappersSurviveNounset ensures plain `eda` under set -u forwards to
// the binary instead of dying on an unbound $1.
func TestWrappersSurviveNounset(t *testing.T) {
	tests := []struct {
		shell  string
		nouns  string
		script string
	}{
		{"bash", "set -u", bashScript},
		{"zsh", "setopt nounset", zshScript},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			if _, err := exec.LookPath(tt.shell); err != nil {
				t.Skipf("%s not available: %v", tt.shell, err)
			}
			scriptFile := filepath.Join(t.TempDir(), "integration")
			if err := os.WriteFile(scriptFile, []byte(tt.script), 0o644); err != nil {
				t.Fatal(err)
			}
			// The eda binary is not on PATH here: the call is expected to
			// fail with command-not-found, but never with an unbound
			// parameter error from the wrapper itself.
			script := tt.nouns + "\nsource " + shellQuote(scriptFile) + "\neda"
			out, _ := exec.Command(tt.shell, "-c", script).CombinedOutput()
			lower := strings.ToLower(string(out))
			if strings.Contains(lower, "unbound") || strings.Contains(lower, "parameter not set") {
				t.Errorf("wrapper is not nounset-safe:\n%s", out)
			}
		})
	}
}
