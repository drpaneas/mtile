// Package hostclipboard bridges the editor to the desktop clipboard. Text
// operations keep their lightweight host-command path, while images use a
// native cross-platform clipboard implementation that normalizes them to PNG.
package hostclipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	systemclipboard "golang.design/x/clipboard"
)

// ErrNoImage reports the ordinary case where the clipboard is available but
// does not currently contain a raster image.
var ErrNoImage = errors.New("system clipboard does not contain an image")

var (
	imageClipboardInitOnce sync.Once
	imageClipboardInitErr  error
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

// ReadImagePNG reads the desktop clipboard's image representation. The
// cross-platform bridge normalizes native clipboard formats (including the
// TIFF representation commonly used on macOS) to PNG bytes.
func ReadImagePNG() ([]byte, error) {
	imageClipboardInitOnce.Do(func() {
		imageClipboardInitErr = systemclipboard.Init()
	})
	if imageClipboardInitErr != nil {
		return nil, fmt.Errorf("initialize image clipboard: %w", imageClipboardInitErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	data, err := systemclipboard.Read(ctx, systemclipboard.FmtImage)
	if errors.Is(err, systemclipboard.ErrNoData) {
		return nil, ErrNoImage
	}
	if err != nil {
		return nil, fmt.Errorf("read image clipboard: %w", err)
	}
	if len(data) == 0 {
		return nil, ErrNoImage
	}
	return data, nil
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
