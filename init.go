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
	var script string
	switch args[0] {
	case "zsh":
		script = zshScript
	case "bash":
		script = bashScript
	default:
		return fmt.Errorf("unsupported shell %q (zsh and bash are supported)", args[0])
	}
	// The emitted script is eval'd by the shell; a partial write must not
	// look like success.
	_, err := fmt.Fprint(stdout, script)
	return err
}
