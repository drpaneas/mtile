package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func developPreviewROMPath(projectDir string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate preview cache: %w", err)
	}
	return prepareDevelopPreviewROM(developPreviewCacheDir(cache), projectDir)
}

func developPreviewCacheDir(userCache string) string {
	return filepath.Join(userCache, "mtile", "preview")
}

func prepareDevelopPreviewROM(cacheDir, projectDir string) (string, error) {
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolve preview project: %w", err)
	}
	projectDir, err = filepath.EvalSymlinks(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolve preview project: %w", err)
	}
	key := sha256.Sum256([]byte(projectDir))
	projectCache := filepath.Join(cacheDir, fmt.Sprintf("%x", key))
	if err := os.MkdirAll(projectCache, 0o700); err != nil {
		return "", fmt.Errorf("create preview cache: %w", err)
	}
	dir, err := os.MkdirTemp(projectCache, "run-")
	if err != nil {
		return "", fmt.Errorf("create preview directory: %w", err)
	}
	return filepath.Join(dir, "preview.gba"), nil
}

// chooseDevelopExportROMPath returns an empty path when the dialog is dismissed.
func chooseDevelopExportROMPath(parent context.Context, projectDir string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolve export project: %w", err)
	}
	const title = "Export Release ROM"
	const name = "game.gba"
	const filter = "GBA ROM | *.gba *.GBA"
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		const script = `on run argv
	set destinationFolder to (POSIX file (item 1 of argv)) as alias
	set destination to choose file name with prompt "Export Release ROM" default name "game.gba" default location destinationFolder
	return (POSIX path of destination) & (character id 0)
end run`
		cmd = exec.CommandContext(ctx, "osascript", "-e", script, projectDir)
	case "windows":
		const script = `$ErrorActionPreference = 'Stop';
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8;
Add-Type -AssemblyName System.Windows.Forms;
$picker = New-Object System.Windows.Forms.SaveFileDialog;
$picker.Title = 'Export Release ROM';
$picker.InitialDirectory = $env:MTILE_ROM_EXPORT_DIR;
$picker.FileName = 'game.gba';
$picker.Filter = 'GBA ROM|*.gba';
$picker.DefaultExt = 'gba';
$picker.OverwritePrompt = $true;
if ($picker.ShowDialog() -eq 'OK') { [Console]::Write($picker.FileName) }`
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
		cmd.Env = append(cmd.Environ(), "MTILE_ROM_EXPORT_DIR="+projectDir)
	default:
		destination := filepath.Join(projectDir, name)
		if tool, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.CommandContext(ctx, tool, "--file-selection", "--save", "--confirm-overwrite", "--title="+title, "--filename="+destination, "--file-filter="+filter)
		} else if tool, err := exec.LookPath("kdialog"); err == nil {
			cmd = exec.CommandContext(ctx, tool, "--getsavefilename", destination, "*.gba *.GBA", "--title", title)
		} else {
			return "", errors.New("the export file picker needs zenity or kdialog; use mtile build to export from the terminal")
		}
	}
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			detail := strings.TrimSpace(string(exit.Stderr))
			unixCancel := runtime.GOOS != "darwin" && runtime.GOOS != "windows" && exit.ExitCode() == 1 &&
				(detail == "" || strings.Contains(detail, "--confirm-overwrite is deprecated"))
			if strings.Contains(detail, "(-128)") || unixCancel {
				return "", nil
			}
			if detail != "" {
				return "", fmt.Errorf("open export file picker: %s", detail)
			}
		}
		return "", fmt.Errorf("open export file picker: %w", err)
	}
	path, _, _ := strings.Cut(strings.TrimSuffix(string(output), "\n"), "\x00")
	if path == "" {
		return "", nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate preview cache: %w", err)
	}
	return validateDevelopExportROMPath(developPreviewCacheDir(cache), path)
}

func validateDevelopExportROMPath(cacheDir, path string) (string, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".gba") {
		return "", errors.New("choose an absolute export path with a .gba file extension")
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return "", errors.New("choose a .gba file, not a directory")
	}
	preview, err := os.Stat(cacheDir)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("check preview cache: %w", err)
	}
	destinationDir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("resolve export directory: %w", err)
	}
	for dir := destinationDir; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(dir); err == nil && os.SameFile(preview, info) {
			return "", errors.New("choose an export location outside the editor preview cache")
		}
		if filepath.Dir(dir) == dir {
			return path, nil
		}
	}
}

func revealDevelopROM(parent context.Context, romPath string) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	info, err := os.Stat(romPath)
	if err != nil {
		return fmt.Errorf("reveal exported ROM: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("the exported ROM is no longer a regular file")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", "-R", romPath)
	case "windows":
		const script = `$ErrorActionPreference = 'Stop'; Start-Process explorer.exe -ArgumentList ('/select,"' + $env:MTILE_ROM_EXPORT_FILE + '"')`
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", script)
		cmd.Env = append(cmd.Environ(), "MTILE_ROM_EXPORT_FILE="+romPath)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", filepath.Dir(romPath))
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reveal exported ROM: %w", err)
	}
	return nil
}
