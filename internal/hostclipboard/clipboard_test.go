package hostclipboard

import (
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestTrimTerminalNewline(t *testing.T) {
	for _, test := range []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"no newline", "print(1)", "print(1)"},
		{"windows tool newline", "print(1)\r\n", "print(1)"},
		{"only one newline removed", "a\r\n\r\n", "a\r\n"},
		{"unix newline is source", "a\n", "a\n"},
		{"interior newlines kept", "a\r\nb\r\n", "a\r\nb"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := TrimTerminalNewline(test.in); got != test.want {
				t.Fatalf("TrimTerminalNewline(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestHostCommandsMatchPlatform(t *testing.T) {
	read, write := readCommands(), writeCommands()
	if len(read) == 0 || len(write) == 0 {
		t.Fatalf("no clipboard commands for %s: read=%v write=%v", runtime.GOOS, read, write)
	}
	for _, command := range slices.Concat(read, write) {
		if len(command) == 0 || command[0] == "" {
			t.Fatalf("empty clipboard command on %s: %q", runtime.GOOS, command)
		}
	}
	switch runtime.GOOS {
	case "darwin":
		if read[0][0] != "pbpaste" || write[0][0] != "pbcopy" {
			t.Fatalf("darwin commands = %v / %v", read, write)
		}
	case "windows":
		// The text must travel on stdin, never inside the command string.
		if script := write[0][len(write[0])-1]; !strings.HasPrefix(script, "$input") {
			t.Fatalf("windows write script %q does not read stdin", script)
		}
	default:
		if read[0][0] != "wl-paste" || write[0][0] != "wl-copy" {
			t.Fatalf("Wayland must be tried first: %v / %v", read, write)
		}
	}
}
