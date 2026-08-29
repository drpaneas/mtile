package build

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GenerateTarget writes the project-specific TinyGo target used by the
// native build. TinyGo resolves extra-files relative to TINYGOROOT, while
// the linker script is intentionally absolute so a temporary build directory
// can use the checked-in runtime without copying the linker inputs.
func GenerateTarget(tinygoRoot, runtimeDir, outputPath string) error {
	return generateTarget(tinygoRoot, runtimeDir, "gba.s", outputPath)
}

// GenerateSmokeTarget writes the equivalent target for the startup smoke ROM.
// Keeping this separate prevents the poison-fill startup from ever
// entering a production build while preserving one target-generation policy.
func GenerateSmokeTarget(tinygoRoot, runtimeDir, outputPath string) error {
	return generateTarget(tinygoRoot, runtimeDir, "gba-smoke.s", outputPath)
}

func generateTarget(tinygoRoot, runtimeDir, startupName, outputPath string) error {
	tinygoRoot, err := filepath.Abs(tinygoRoot)
	if err != nil {
		return fmt.Errorf("resolve TinyGo root: %w", err)
	}
	runtimeDir, err = filepath.Abs(runtimeDir)
	if err != nil {
		return fmt.Errorf("resolve runtime directory: %w", err)
	}
	basePath := filepath.Join(tinygoRoot, "targets", "gameboy-advance.json")
	base, err := os.ReadFile(basePath)
	if err != nil {
		return fmt.Errorf("read TinyGo GBA target: %w", err)
	}
	var target map[string]any
	if err := json.Unmarshal(base, &target); err != nil {
		return fmt.Errorf("decode TinyGo GBA target: %w", err)
	}
	if target == nil {
		return errors.New("TinyGo GBA target must be a JSON object")
	}
	// TinyGo appends inherited extra-files. Removing inherits is therefore
	// required: otherwise stock gameboy-advance.s and our _start coexist.
	delete(target, "inherits")
	target["gc"] = "none"

	linker := filepath.Join(runtimeDir, "crt0", "gba.ld")
	startup := filepath.Join(runtimeDir, "crt0", startupName)
	for _, path := range []string{linker, startup, filepath.Join(tinygoRoot, "src", "runtime", "asm_arm.S")} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			if err == nil {
				err = errors.New("is a directory")
			}
			return fmt.Errorf("target input %q: %w", path, err)
		}
	}
	relStartup, err := filepath.Rel(tinygoRoot, startup)
	if err != nil {
		return fmt.Errorf("make startup path relative to TinyGo root: %w", err)
	}
	target["extra-files"] = []string{filepath.ToSlash(relStartup), "src/runtime/asm_arm.S"}
	target["linkerscript"] = linker

	encoded, err := json.MarshalIndent(target, "", "\t")
	if err != nil {
		return fmt.Errorf("encode target: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := atomicWrite(outputPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write target %q: %w", outputPath, err)
	}
	return nil
}

func discoverTinyGoRoot(tinygoBin string) (string, error) {
	cmd := exec.Command(tinygoBin, "env", "TINYGOROOT")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tinygo env TINYGOROOT: %w", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", errors.New("tinygo env TINYGOROOT returned an empty path")
	}
	return filepath.Abs(root)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mtile-*"+filepath.Ext(path))
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

// targetHasNoInheritedStartup is kept small and testable: generated targets
// must never accidentally reintroduce TinyGo's stock crt0.
func targetHasNoInheritedStartup(data []byte) error {
	var target map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &target); err != nil {
		return err
	}
	if target == nil {
		return errors.New("generated target must be a JSON object")
	}
	if _, ok := target["inherits"]; ok {
		return errors.New("generated target still has inherits")
	}
	return nil
}
