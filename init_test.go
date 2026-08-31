package main

import (
	"strings"
	"testing"
)

func TestRunInit(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"init", "-", "zsh"}, []string{"eda()", "compdef _eda eda", "command eda"}},
		{[]string{"init", "zsh"}, []string{"eda()", "compdef _eda eda", "command eda"}},
		{[]string{"init", "-", "bash"}, []string{"eda()", "complete -F _eda eda", "command eda"}},
		{[]string{"init", "bash"}, []string{"eda()", "complete -F _eda eda", "command eda"}},
	}
	for _, tt := range tests {
		code, stdout, stderr := runEda(t, t.TempDir(), "", tt.args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d stderr=%q", tt.args, code, stderr)
		}
		for _, want := range tt.want {
			if !strings.Contains(stdout, want) {
				t.Errorf("%v output missing %q", tt.args, want)
			}
		}
	}
}

func TestRunInitUnsupportedShell(t *testing.T) {
	code, stdout, _ := runEda(t, t.TempDir(), "", "init", "-", "fish")
	if code == 0 {
		t.Error("unsupported shell must fail")
	}
	if stdout != "" {
		t.Errorf("stdout must stay empty on error, got %q", stdout)
	}
}
