package main

import (
	"strings"
	"testing"
)

func TestRunInitZsh(t *testing.T) {
	for _, args := range [][]string{{"init", "-", "zsh"}, {"init", "zsh"}} {
		code, stdout, stderr := runEda(t, t.TempDir(), "", args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d stderr=%q", args, code, stderr)
		}
		for _, want := range []string{"eda()", "compdef _eda eda", "command eda"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%v output missing %q", args, want)
			}
		}
	}
}

func TestRunInitUnsupportedShell(t *testing.T) {
	code, stdout, _ := runEda(t, t.TempDir(), "", "init", "-", "bash")
	if code == 0 {
		t.Error("unsupported shell must fail")
	}
	if stdout != "" {
		t.Errorf("stdout must stay empty on error, got %q", stdout)
	}
}
