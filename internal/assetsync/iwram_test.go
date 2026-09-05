package assetsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProjectIWRAMFunctionsRoundTripAndValidation(t *testing.T) {
	dir := t.TempDir()
	project, err := LoadProject(dir)
	if err != nil || len(project.IWRAMFunctions) != 0 {
		t.Fatalf("default placement = %v, %v", project.IWRAMFunctions, err)
	}
	if err := SaveProject(dir, project); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "iwramFunctions") {
		t.Fatal("unset setting was serialized")
	}
	project.IWRAMFunctions = []string{"draw_particles", "move2"}
	if err := SaveProject(dir, project); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProject(dir)
	if err != nil || !slices.Equal(loaded.IWRAMFunctions, project.IWRAMFunctions) {
		t.Fatalf("round trip = %v, %v", loaded.IWRAMFunctions, err)
	}
	for _, names := range [][]string{{"hot", "hot"}, {"_init"}, {"_update"}, {"_draw"}, {""}, {"bad-name"}, {"actor.draw"}, {"1hot"}, {"end"}} {
		project.IWRAMFunctions = names
		if err := SaveProject(dir, project); err == nil || !strings.Contains(err.Error(), "iwramFunctions") {
			t.Fatalf("Save %v: %v", names, err)
		}
		data, err := json.Marshal(project)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "project.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadProject(dir); err == nil || !strings.Contains(err.Error(), "iwramFunctions") {
			t.Fatalf("Load %v: %v", names, err)
		}
	}
}
