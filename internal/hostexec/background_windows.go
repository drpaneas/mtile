// Package hostexec configures noninteractive child programs of the desktop editor.
package hostexec

import (
	"os/exec"
	"syscall"
)

// Background keeps a console application launched from the graphical editor
// hidden. Standard streams remain available for diagnostics and the player.
func Background(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
