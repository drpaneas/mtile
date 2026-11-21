package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Maxmod is compiled by TinyGo's bundled Clang only for cartridges using it.
// No ARM GCC installation or binary library is required on the user's host.
func addDirectAudioTarget(tinygoRoot, runtimeDir, targetPath string, format int) error {
	raw, err := os.ReadFile(targetPath)
	if err != nil {
		return err
	}
	var target map[string]any
	if err = json.Unmarshal(raw, &target); err != nil {
		return err
	}
	root := filepath.Join(runtimeDir, "maxmod")
	files := []string{"bridge.c", "src/core/mas.c", "src/core/mas_arm.c", "src/core/effect.c", "src/gba/main_gba.c", "src/gba/mixer.c", "src/gba/mixer_asm.S"}
	extra, _ := target["extra-files"].([]any)
	flags, _ := target["cflags"].([]any)
	bundled := len(extra) > 0 && extra[0] == "targets/mtile/startup.S"
	if bundled {
		flags = append(flags, "-I"+runtimeDir)
	}
	for _, file := range files {
		path := filepath.Join(root, filepath.FromSlash(file))
		if _, err = os.Stat(path); err != nil {
			return fmt.Errorf("locate Maxmod source: %w", err)
		}
		if bundled {
			extra = append(extra, "targets/mtile/"+filepath.Base(file))
		} else {
			rel, err := filepath.Rel(tinygoRoot, path)
			if err != nil {
				return err
			}
			extra = append(extra, filepath.ToSlash(rel))
		}
	}
	target["extra-files"] = extra
	for _, flag := range []string{"-std=gnu23", "-D__GBA__", "-I" + filepath.Join(root, "include"), "-Wno-unknown-attributes", "-Wno-ignored-attributes"} {
		flags = append(flags, flag)
	}
	if format == 0x18 {
		flags = append(flags, "-DMM_MMUTIL_DEVKITPRO_COMPAT")
	}
	target["cflags"] = flags
	encoded, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(targetPath, append(encoded, '\n'), 0o644)
}
