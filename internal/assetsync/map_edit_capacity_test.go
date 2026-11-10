package assetsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectMapEditCapacity(t *testing.T) {
	dir := t.TempDir()
	for _, capacity := range []int{0, 512, 4096} {
		if err := SaveProject(dir, Project{MapEditCapacity: capacity}); err != nil {
			t.Fatal(err)
		}
		project, err := LoadProject(dir)
		if err != nil || project.MapEditCapacity != capacity {
			t.Fatalf("capacity %d did not round-trip: %+v, %v", capacity, project, err)
		}
	}
	if err := SaveProject(dir, Project{MapEditCapacity: 1024}); err == nil || !strings.Contains(err.Error(), "mapEditCapacity") {
		t.Fatalf("invalid save: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"mapEditCapacity":1024}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProject(dir); err == nil || !strings.Contains(err.Error(), "mapEditCapacity") {
		t.Fatalf("invalid load: %v", err)
	}
}
