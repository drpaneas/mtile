package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestBuildNamedPoseLibraryEditsAndModuleDiagnostics(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "named-poses")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"game.lua":     "require(\"view\")\nfunction _draw() cls() draw_actor() end\n",
		"src/view.lua": "function draw_actor()\n pspr(\"hero_idle\",20,30)\nend\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, names := range [][]string{{"prop", "hero_idle"}, {"hero_idle", "prop"}, {"hero_idle"}, {"prop"}} {
		file := assetsync.PoseFile{Version: 1}
		for _, name := range names {
			file.Poses = append(file.Poses, assetsync.PoseRecord{Name: name, Parts: []assetsync.PosePartRecord{{Tile: 0}}})
		}
		if err := assetsync.SavePoses(dir, &file); err != nil {
			t.Fatal(err)
		}
		for _, renderer := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0, transpiler.RendererAuto} {
			selected := false
			s := Service{TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"), RuntimeDir: t.TempDir(), Renderer: renderer, Trace: func(e Event) { selected = selected || strings.HasPrefix(e.Message, "Renderer: requested") }}
			_, err := s.Build(dir, "")
			if len(names) == 1 && names[0] == "prop" {
				if err == nil || !strings.Contains(err.Error(), "[PSPR_UNKNOWN_POSE]") || !strings.Contains(err.Error(), "src/view.lua:2:7") || selected {
					t.Fatalf("deleted named pose: %v", err)
				}
			} else if !selected || err == nil || strings.Contains(err.Error(), "[PSPR_") {
				t.Fatalf("names %v renderer %s: selected=%t err=%v", names, renderer, selected, err)
			}
		}
	}
}

func TestBuildPoseNumericWarningRemapsModulePosition(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "warning")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"game.lua":     "require(\"view\")\nfunction _draw() cls() draw_actor() end\n",
		"src/view.lua": "function draw_actor()\n pspr(99,0,0)\nend\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var warnings []string
	s := Service{TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"), RuntimeDir: t.TempDir(), Trace: func(e Event) {
		if strings.HasPrefix(e.Message, "Warning: ") {
			warnings = append(warnings, e.Message)
		}
	}}
	result, _ := s.Build(dir, "")
	if len(warnings) != 1 || len(result.Warnings) != 1 || !strings.Contains(warnings[0], "src/view.lua:2:7: [PSPR_ID_OUT_OF_RANGE]") {
		t.Fatalf("warnings=%v result=%v", warnings, result.Warnings)
	}
}
