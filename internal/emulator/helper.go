package emulator

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const helperVersion = "mgba-0.10.3-v4"

// ResolveHelper finds the release-bundled sibling first. Source checkouts can
// build the pinned helper into the user's cache on first use, which keeps
// generated native files out of the repository.
func ResolveHelper() (string, error) {
	if explicit := os.Getenv("MTILE_PLAYER"); explicit != "" {
		return executableFile(explicit)
	}
	name := "mtile-player"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if self, err := os.Executable(); err == nil {
		if path, err := executableFile(filepath.Join(filepath.Dir(self), name)); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return filepath.Abs(path)
	}
	cache, err := helperCachePath(name)
	if err == nil {
		if path, fileErr := executableFile(cache); fileErr == nil {
			return path, nil
		}
	}
	root, ok := sourceRoot()
	if !ok {
		return "", errors.New("bundled mtile-player is missing; reinstall Mtile or set MTILE_PLAYER")
	}
	if err := buildSourceHelper(root, cache); err != nil {
		return "", err
	}
	return executableFile(cache)
}

func helperCachePath(name string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "mtile", "player", helperVersion, runtime.GOOS+"-"+runtime.GOARCH, name), nil
}

func executableFile(path string) (string, error) {
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

func sourceRoot() (string, bool) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "cmd", "mtile-player", "CMakeLists.txt")); err != nil {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(root, "third_party", "mgba", "CMakeLists.txt")); err != nil {
		return "", false
	}
	return root, true
}

func buildSourceHelper(root, destination string) error {
	cmake, err := exec.LookPath("cmake")
	if err != nil {
		return errors.New("mtile-player is not bundled and CMake is unavailable for the source-checkout fallback")
	}
	buildDir := filepath.Join(filepath.Dir(destination), "build")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return err
	}
	configure := exec.Command(cmake, "-S", filepath.Join(root, "cmd", "mtile-player"), "-B", buildDir, "-DCMAKE_BUILD_TYPE=Release")
	if output, err := configure.CombinedOutput(); err != nil {
		return fmt.Errorf("configure bundled emulator: %w: %s", err, output)
	}
	compile := exec.Command(cmake, "--build", buildDir, "--target", "mtile-player", "--config", "Release")
	if output, err := compile.CombinedOutput(); err != nil {
		return fmt.Errorf("build bundled emulator: %w: %s", err, output)
	}
	built := filepath.Join(buildDir, "mtile-player")
	if runtime.GOOS == "windows" {
		built += ".exe"
	}
	input, err := os.ReadFile(built)
	if err != nil {
		return fmt.Errorf("read emulator helper: %w", err)
	}
	if err := os.WriteFile(destination, input, 0o755); err != nil {
		return fmt.Errorf("install emulator helper: %w", err)
	}
	return nil
}
