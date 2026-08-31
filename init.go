package main

import (
	_ "embed"
	"fmt"
	"io"
)

// zshScript is emitted by `eda init - zsh` for eval in .zshrc, rbenv style.
// It lives in a separate file for shell syntax highlighting and is embedded
// so the binary is self-contained.
//
//go:embed shell/eda.zsh
var zshScript string

func cmdInit(stdout io.Writer, args []string) error {
	// Accept the rbenv-style `init - <shell>` as well as `init <shell>`.
	if len(args) > 0 && args[0] == "-" {
		args = args[1:]
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: eda init - <shell>")
	}
	if args[0] != "zsh" {
		return fmt.Errorf("unsupported shell %q (only zsh is supported)", args[0])
	}
	fmt.Fprint(stdout, zshScript)
	return nil
}
