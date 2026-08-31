package main

import (
	_ "embed"
	"fmt"
	"io"
)

// The integration scripts are emitted by `eda init - <shell>` for eval in
// the shell's rc file, rbenv style. They live in separate files for shell
// syntax highlighting and are embedded so the binary is self-contained.
//
//go:embed shell/eda.zsh
var zshScript string

//go:embed shell/eda.bash
var bashScript string

func cmdInit(stdout io.Writer, args []string) error {
	// Accept the rbenv-style `init - <shell>` as well as `init <shell>`.
	if len(args) > 0 && args[0] == "-" {
		args = args[1:]
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: eda init - <shell>")
	}
	switch args[0] {
	case "zsh":
		fmt.Fprint(stdout, zshScript)
	case "bash":
		fmt.Fprint(stdout, bashScript)
	default:
		return fmt.Errorf("unsupported shell %q (zsh and bash are supported)", args[0])
	}
	return nil
}
