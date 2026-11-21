//go:build !windows

package hostexec

import "os/exec"

func Background(*exec.Cmd) {}
