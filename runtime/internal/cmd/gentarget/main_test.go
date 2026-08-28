package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTinygoRel_isNotAbsolute(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	proj := t.TempDir()
	gba := filepath.Join(proj, "gba.s")
	if err := os.WriteFile(gba, []byte(".word 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := tinygoRel(root, gba)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(rel) {
		t.Fatalf("extra-files path is absolute: %q", rel)
	}
	joined := filepath.Clean(filepath.Join(root, rel))
	got, err := filepath.EvalSymlinks(joined)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(gba)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Join(TINYGOROOT, %q) = %s, want %s", rel, got, want)
	}
}

func TestTinygoRel_missingFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	missing := filepath.Join(root, "missing.s")
	if _, err := tinygoRel(root, missing); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteTarget_rejectsAbsoluteExtraFile(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "target.json")
	err := writeTarget(map[string]any{"gc": "none"}, []string{"/abs/gba.s"}, dest)
	if err == nil {
		t.Fatal("expected error for absolute extra-files path")
	}
}

func TestHasPoisonImmediate_ignoresComments(t *testing.T) {
	t.Parallel()
	src := []byte("    // poison 0xF1F1F1F1 in a comment\n    mov     r2, #0\n")
	if hasPoisonImmediate(src) {
		t.Fatal("comment must not count as poison")
	}
}

func TestHasPoisonImmediate_detectsLdr(t *testing.T) {
	t.Parallel()
	src := []byte("    ldr     r2, =0xF1F1F1F1\n")
	if !hasPoisonImmediate(src) {
		t.Fatal("ldr poison immediate not detected")
	}
}
