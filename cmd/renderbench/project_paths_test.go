//go:build runtimeperfdiagnostic

package main

import "path/filepath"

// Paths are relative to the repository root. Local Celeste research is
// intentionally still optional; checked-in engineering fixtures live separately.
func diagnosticProjectPath(name string) string {
	switch name {
	case "testio", "color", "platformer", "audio-gb-editor", "physics-stress", "function-physics-stress", "multi-return-physics-stress", "noalloc-call-stress", "parallax-rectangles":
		return filepath.Join("testdata", "projects", name)
	case "loop", "celeste-mode0", "celeste-parallax":
		return filepath.Join("research", name)
	default:
		return filepath.Join("examples", name)
	}
}
