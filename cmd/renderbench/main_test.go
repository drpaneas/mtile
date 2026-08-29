package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectFingerprintIsStableAndIgnoresRunOutputs(t *testing.T) {
	directory := t.TempDir()
	writeBenchmarkFile(t, filepath.Join(directory, "game.lua"), "function _draw() cls(1) end\n")
	writeBenchmarkFile(t, filepath.Join(directory, "assets", "map.json"), "{}\n")

	first, err := projectFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	writeBenchmarkFile(t, filepath.Join(directory, "game.gba"), "generated ROM")
	writeBenchmarkFile(t, filepath.Join(directory, "game.sav"), "generated save")
	second, err := projectFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("run outputs changed project fingerprint: %s != %s", first, second)
	}

	writeBenchmarkFile(t, filepath.Join(directory, "game.lua"), "function _draw() cls(2) end\n")
	third, err := projectFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("source change did not change project fingerprint")
	}
}

func TestProjectFingerprintIncludesRelativePaths(t *testing.T) {
	left := t.TempDir()
	right := t.TempDir()
	writeBenchmarkFile(t, filepath.Join(left, "a.lua"), "same")
	writeBenchmarkFile(t, filepath.Join(right, "b.lua"), "same")

	leftSHA, err := projectFingerprint(left)
	if err != nil {
		t.Fatal(err)
	}
	rightSHA, err := projectFingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftSHA == rightSHA {
		t.Fatal("relative path was not part of project fingerprint")
	}
}

func TestStageProjectWithSourceOverridesGameAndSkipsRunOutputs(t *testing.T) {
	project := t.TempDir()
	workload := filepath.Join(t.TempDir(), "workload.lua")
	writeBenchmarkFile(t, filepath.Join(project, "game.lua"), "old")
	writeBenchmarkFile(t, filepath.Join(project, "project.json"), "{}")
	writeBenchmarkFile(t, filepath.Join(project, "assets", "map.json"), "map")
	writeBenchmarkFile(t, filepath.Join(project, "game.gba"), "rom")
	writeBenchmarkFile(t, filepath.Join(project, "game.sav"), "save")
	writeBenchmarkFile(t, workload, "new")

	staged, err := stageProjectWithSource(project, workload)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(staged)
	game, err := os.ReadFile(filepath.Join(staged, "game.lua"))
	if err != nil || string(game) != "new" {
		t.Fatalf("staged game = %q, %v", game, err)
	}
	if _, err := os.Stat(filepath.Join(staged, "assets", "map.json")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"game.gba", "game.sav"} {
		if _, err := os.Stat(filepath.Join(staged, name)); !os.IsNotExist(err) {
			t.Fatalf("staged run output %s: %v", name, err)
		}
	}
}

func writeBenchmarkFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
