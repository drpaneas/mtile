// Package hostclipboard provides a dependency-free bridge to the desktop
// clipboard. The editor keeps an internal fallback when the host command is
// unavailable (for example on a minimal Linux installation).
package hostclipboard

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

func Read() (string, error) {
	for _, command := range readCommands() {
		output, err := exec.Command(command[0], command[1:]...).Output()
		if err == nil {
			return string(output), nil
		}
	}
	return "", errors.New("system clipboard is unavailable")
}

func Write(value string) error {
	for _, command := range writeCommands() {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdin = bytes.NewBufferString(value)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return errors.New("system clipboard is unavailable")
}

func readCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbpaste"}}
	case "windows":
		return [][]string{{"powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw"}}
	default:
		return [][]string{{"wl-paste", "--no-newline"}, {"xclip", "-selection", "clipboard", "-o"}}
	}
}

func writeCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "windows":
		// PowerShell's pipeline preserves newlines and avoids embedding user
		// source in a command string.
		return [][]string{{"powershell", "-NoProfile", "-Command", "$input | Set-Clipboard"}}
	default:
		return [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard", "-i"}}
	}
}

// TrimTerminalNewline removes the newline some clipboard tools append while
// retaining all intentional source formatting.
func TrimTerminalNewline(value string) string {
	return strings.TrimSuffix(value, "\r\n")
}
