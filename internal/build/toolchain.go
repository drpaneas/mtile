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

// ToolchainEnvironment returns the Go executable and child-process environment
// for a selected TinyGo. Packaged TinyGo runs go by name and reads its own SDK
// files by path. Keep both lookups inside the relocated desktop bundle without
// changing the environment of the editor or an explicitly selected source tool.
func ToolchainEnvironment(tinygoBin string) (string, []string) {
	env := os.Environ()
	tinyBinDir := filepath.Dir(tinygoBin)
	tinyRoot := filepath.Dir(tinyBinDir)
	toolsDir := filepath.Dir(tinyRoot)
	if filepath.Base(tinyBinDir) != "bin" || filepath.Base(tinyRoot) != "tinygo" || filepath.Base(toolsDir) != "toolchains" {
		return "go", env
	}
	goRoot := filepath.Join(toolsDir, "go")
	goBinDir := filepath.Join(goRoot, "bin")
	goName := "go"
	if runtime.GOOS == "windows" {
		goName += ".exe"
	}
	goBin := filepath.Join(goBinDir, goName)
	// Older bundles only shipped TinyGo and used the system Go installation.
	if _, err := os.Stat(goRoot); os.IsNotExist(err) {
		return "go", env
	}
	path := goBinDir
	if runtime.GOOS == "windows" {
		// TinyGo falls back to cmd.exe's mklink /J when Developer Mode is
		// disabled. A clean desktop launch still needs that OS tool.
		if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
			path += string(os.PathListSeparator) + filepath.Join(systemRoot, "System32")
		}
	}
	if inherited := os.Getenv("PATH"); inherited != "" {
		path += string(os.PathListSeparator) + inherited
	}
	env = setEnv(env, "PATH", path)
	env = setEnv(env, "GOROOT", goRoot)
	env = setEnv(env, "TINYGOROOT", tinyRoot)
	env = setEnv(env, "GOTOOLCHAIN", "local")
	env = setEnv(env, "GOENV", "off")
	env = setEnv(env, "GOPROXY", "off")
	return goBin, env
}
