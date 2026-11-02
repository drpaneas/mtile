// Package hostfiles provides the desktop's native file pickers.
package hostfiles

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func ChooseAudio() (string, error) {
	paths, err := choose(false, false)
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func ChooseAudioFiles() ([]string, error) { return choose(false, true) }

func ChooseConverter() (string, error) {
	paths, err := choose(true, false)
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func choose(converter, multiple bool) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	separator := "\x00"
	switch runtime.GOOS {
	case "darwin":
		script := `set chosenFile to choose file with prompt "Choose WAV, MOD, XM, S3M or IT audio"
return (POSIX path of chosenFile) & (character id 0)`
		if multiple {
			script = `set chosenFiles to choose file with prompt "Choose WAV, MOD, XM, S3M or IT audio" with multiple selections allowed
set chosenText to ""
repeat with chosenFile in chosenFiles
set chosenText to chosenText & (POSIX path of chosenFile) & (character id 0)
end repeat
return chosenText`
		}
		if converter {
			script = `set chosenFile to choose file with prompt "Choose the mmutil converter executable"
return (POSIX path of chosenFile) & (character id 0)`
		}
		cmd = exec.CommandContext(ctx, "osascript", "-e", script)
	case "windows":
		script := `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms; $picker = New-Object System.Windows.Forms.OpenFileDialog; $picker.Filter = 'GBA audio|*.wav;*.mod;*.xm;*.s3m;*.it|All files|*.*'; if ($picker.ShowDialog() -eq 'OK') { foreach ($file in $picker.FileNames) { [Console]::Write($file); [Console]::Write([char]0) } }`
		if multiple {
			script = strings.Replace(script, "$picker.Filter =", "$picker.Multiselect = $true; $picker.Filter =", 1)
		}
		if converter {
			script = strings.Replace(script, "GBA audio|*.wav;*.mod;*.xm;*.s3m;*.it|All files|*.*", "mmutil converter|*.exe|All files|*.*", 1)
		}
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
	default:
		separator = "\n"
		if tool, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.CommandContext(ctx, tool, "--file-selection", "--title=Choose GBA audio", "--file-filter=Audio | *.wav *.WAV *.mod *.MOD *.xm *.XM *.s3m *.S3M *.it *.IT")
			if multiple {
				separator = "\x1f"
				cmd.Args = append(cmd.Args, "--multiple", "--separator="+separator)
			}
			if converter {
				cmd = exec.CommandContext(ctx, tool, "--file-selection", "--title=Choose mmutil executable")
			}
		} else if tool, err := exec.LookPath("kdialog"); err == nil {
			cmd = exec.CommandContext(ctx, tool, "--getopenfilename", ".", "*.wav *.WAV *.mod *.MOD *.xm *.XM *.s3m *.S3M *.it *.IT")
			if multiple {
				cmd.Args = append(cmd.Args, "--multiple", "--separate-output")
			}
			if converter {
				cmd = exec.CommandContext(ctx, tool, "--getopenfilename", ".", "*")
			}
		} else {
			return nil, fmt.Errorf("native Browse needs zenity or kdialog; paste a file path into the PCM Editor")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && (runtime.GOOS != "darwin" && exit.ExitCode() == 1 || strings.Contains(string(exit.Stderr), "(-128)")) {
			return nil, nil
		}
		return nil, fmt.Errorf("Browse failed: %w; paste a file path instead", err)
	}
	return parsePickerPaths(out, separator), nil
}

// Native NUL/US delimiters keep spaces and Unicode intact. Trim only the
// program's output newline, never whitespace belonging to the selected name.
func parsePickerPaths(out []byte, separator string) []string {
	text := strings.TrimSuffix(string(out), "\n")
	if separator == "\n" {
		text = strings.TrimSuffix(text, "\r")
	}
	var paths []string
	for _, path := range strings.Split(text, separator) {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}
