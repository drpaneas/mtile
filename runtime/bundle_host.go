//go:build !tinygo

package runtime

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// runtimeSource contains the compact source module required by generated GBA
// builds. This lets an installed editor materialize its runtime without
// relying on the original Mtile source checkout.
//
//go:embed go.mod LICENSE crt0/gba.s crt0/gba.ld *.go audio/*.go hw/*.go maxmod
//go:embed third_party/aurora/README.md third_party/aurora/COPYING third_party/aurora/pfunc_pal_Aurora11.lua
var runtimeSource embed.FS

// MaterializeSource writes the embedded runtime module below destination.
// Existing files are replaced atomically enough for a versioned private cache;
// callers should use a destination unique to the Mtile release.
func MaterializeSource(destination string) error {
	return fs.WalkDir(runtimeSource, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return os.MkdirAll(destination, 0o755)
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		contents, err := runtimeSource.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, contents, 0o644); err != nil {
			return fmt.Errorf("materialize runtime %s: %w", path, err)
		}
		return nil
	})
}
