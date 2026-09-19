package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestBuildMode0AudioCommitGuardMapsOriginalModulePosition(t *testing.T) {
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererAuto, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			dir := mode0AudioCommitBuildProject(t, "function _init() gba_note(1,69,0,24) end\nfunction _update() change_palette() end\nfunction _draw() cls() map() end\n")
			var fallback string
			reachedToolchain := false
			service := Service{Renderer: renderer, TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"), RuntimeDir: t.TempDir(), Trace: func(event Event) {
				if event.Phase == PhaseToolchain {
					reachedToolchain = true
				}
				if strings.HasPrefix(event.Message, "Renderer fallback:") {
					fallback = event.Message
				}
			}}
			_, err := service.Build(dir, "")
			if err == nil {
				t.Fatal("missing toolchain unexpectedly succeeded")
			}
			diagnostic := fallback
			if renderer == transpiler.RendererMode0 {
				if reachedToolchain {
					t.Fatal("forced rejection reached toolchain")
				}
				diagnostic = err.Error()
			} else if !reachedToolchain {
				t.Fatalf("renderer did not reach toolchain: %v", err)
			}
			if renderer == transpiler.RendererMode4 {
				if fallback != "" {
					t.Fatalf("explicit Mode4 fallback=%q", fallback)
				}
				return
			}
			for _, want := range []string{"src/palette.lua:2:3", "[M0_RESOURCE_LIMIT]", "four materialized BGs", "VBlank"} {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("diagnostic=%q missing %q", diagnostic, want)
				}
			}
		})
	}
}

func TestBuildMode0AudioCommitGuardKeepsInitAndUnusedControls(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"init-only", "function _init() gba_note(1,69,0,24) change_palette() sset(8,0,2) end\nfunction _draw() cls() map() end\n"},
		{"unused-audio-assets", "function _update() change_palette() sset(8,0,2) end\nfunction _draw() cls() map() end\n"},
		{"one-drawn-bg", "function _init() gba_note(1,69,0,24) end\nfunction _update() change_palette() sset(8,0,2) end\nfunction _draw() cls() map(0) end\n"},
		{"four-dirty-bg-audio", "function _init() gba_note(1,69,0,24) end\nfunction _update() mset(0,0,0,1) mset(1,0,0,1) mset(2,0,0,1) mset(3,0,0,1) end\nfunction _draw() cls() map() end\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := mode0AudioCommitBuildProject(t, tc.source)
			for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererAuto} {
				reachedToolchain := false
				fallback := ""
				service := Service{Renderer: renderer, TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"), RuntimeDir: t.TempDir(), Trace: func(event Event) {
					if event.Phase == PhaseToolchain {
						reachedToolchain = true
					}
					if strings.HasPrefix(event.Message, "Renderer fallback:") {
						fallback = event.Message
					}
				}}
				_, err := service.Build(dir, "")
				if err == nil || !reachedToolchain || fallback != "" {
					t.Fatalf("renderer=%s err=%v reachedToolchain=%t fallback=%q", renderer, err, reachedToolchain, fallback)
				}
			}
		})
	}
}

func mode0AudioCommitBuildProject(t *testing.T, source string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "audio-commit")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 16, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	writeNativeCH1Fixture(t, dir)
	m := &assetsync.Map{Version: assetsync.MapSchemaVersion, Width: 1, Height: 1}
	for layer := 0; layer < 4; layer++ {
		m.Layers = append(m.Layers, assetsync.MapLayer{ID: layer, Cells: []int{1}})
	}
	if err := assetsync.SaveMap(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src/palette.lua"), []byte("function change_palette()\n  pal(1,2)\nend\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte("require(\"palette\")\n"+source), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}
