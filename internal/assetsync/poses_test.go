package assetsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPosesMissingFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	poses, err := LoadPoses(dir)
	if err != nil {
		t.Fatal(err)
	}
	if poses == nil || poses.Version != 1 || len(poses.Poses) != 0 {
		t.Fatalf("missing poses.json = %+v", poses)
	}
}

func TestLoadPosesRejectsBadVersionAndDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "poses.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"poses":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPoses(dir); err == nil || !strings.Contains(err.Error(), "poses.json") || !strings.Contains(err.Error(), "version") {
		t.Fatalf("version error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"poses":[{"name":"a","parts":[]},{"name":"a","parts":[]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPoses(dir); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
}
