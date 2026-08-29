package build

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// resolveTinyGo accepts an explicit managed toolchain before PATH. Release
// archives place TinyGo next to the Mtile executables, so users never need a
// separate TinyGo installation. Source developers can still use their PATH.
func resolveTinyGo() (string, error) {
	if explicit := os.Getenv("MTILE_TINYGO"); explicit != "" {
		return tinyGoExecutable(explicit)
	}
	name := "tinygo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if self, err := os.Executable(); err == nil {
		directory := filepath.Dir(self)
		candidates := []string{
			filepath.Join(directory, name),
			filepath.Join(directory, "toolchains", "tinygo", "bin", name),
			filepath.Join(directory, "..", "toolchains", "tinygo", "bin", name),
		}
		for _, candidate := range candidates {
			if path, err := tinyGoExecutable(candidate); err == nil {
				return path, nil
			}
		}
	}
	if path, err := exec.LookPath("tinygo"); err == nil {
		return filepath.Abs(path)
	}
	return "", errors.New("TinyGo toolchain is unavailable; use the Mtile release bundle, install TinyGo, or set MTILE_TINYGO")
}

func tinyGoExecutable(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("is a directory")
		}
		return "", err
	}
	return absolute, nil
}
