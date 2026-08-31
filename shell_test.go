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

// TestBashCompletionKeepsRefNamesLiteral guards against feeding ref names to
// compgen -W, which expands its word list and would execute code embedded in
// a hostile branch name (fetched refs are attacker-controlled).
func TestBashCompletionKeepsRefNamesLiteral(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	repo := newTestRepo(t)
	marker := filepath.Join(t.TempDir(), "pwned")
	evil := "x-$(true>" + marker + ")"
	gitT(t, repo, "branch", evil)

	scriptFile := filepath.Join(t.TempDir(), "eda.bash")
	if err := os.WriteFile(scriptFile, []byte(bashScript), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "source " + shellQuote(scriptFile) + "\n" +
		"cd " + shellQuote(repo) + "\n" +
		"COMP_WORDS=(eda switch x)\n" +
		"COMP_CWORD=2\n" +
		"_eda\n" +
		`printf '%s\n' "${COMPREPLY[@]}"`
	out, err := exec.Command("bash", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("completion run failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), evil) {
		t.Errorf("completion must offer the branch name literally, got:\n%s", out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("completion executed code embedded in a ref name (marker %s exists)", marker)
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
