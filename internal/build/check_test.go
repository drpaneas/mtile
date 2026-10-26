package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func checkProjectFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project with spaces")
	if err := CreateProject(root, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCheckProjectRendererMatchesBuildAndUsesUnsavedModules(t *testing.T) {
	root := checkProjectFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	entry := "require(\"view\")\nfunction _draw()\n cls(0)\n draw_scene()\nend\n"
	module := "function draw_scene()\n  circfill(4,4,1,7)\nend\n"
	if err := os.WriteFile(filepath.Join(root, "game.lua"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	// The overlay module need not be saved yet.
	service := Service{UseProjectRenderer: true, TinyGoBin: filepath.Join(root, "missing-tinygo")}
	analysis := service.CheckProject(root, map[string]string{"src/view.lua": module})
	if !analysis.RendererReady || analysis.Renderer.Selected != transpiler.RendererMode4 || len(analysis.Diagnostics) != 1 {
		t.Fatalf("analysis = %+v", analysis)
	}
	d := analysis.Diagnostics[0]
	if d.Source != "src/view.lua" || d.Line != 2 || d.Column != 3 || d.EndColumn != 11 || d.Severity != DiagnosticInformation || d.Code != "M0_MODE4_BUILTIN" {
		t.Fatalf("fallback diagnostic = %+v", d)
	}
	if _, err := os.Stat(filepath.Join(root, "src/view.lua")); !os.IsNotExist(err) {
		t.Fatal("analysis wrote unsaved module")
	}
	if _, err := os.Stat(filepath.Join(root, "game.gba")); !os.IsNotExist(err) {
		t.Fatal("analysis built a ROM")
	}
	if err := os.WriteFile(filepath.Join(root, "src/view.lua"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	var buildFallback string
	service.Trace = func(event Event) {
		if strings.HasPrefix(event.Message, "Renderer fallback: ") {
			buildFallback = strings.TrimPrefix(event.Message, "Renderer fallback: ")
		}
	}
	if _, err := service.Build(root, ""); err == nil {
		t.Fatal("missing toolchain succeeded")
	}
	if buildFallback != analysis.Renderer.Diagnostic || buildFallback != d.Message {
		t.Fatalf("build fallback %q differs from live %q", buildFallback, d.Message)
	}
	project, err := assetsync.LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	project.Renderer = "mode0"
	if err := assetsync.SaveProject(root, project); err != nil {
		t.Fatal(err)
	}
	forced := service.CheckProject(root, nil)
	if forced.RendererReady || len(forced.Diagnostics) != 1 || forced.Diagnostics[0].Severity != DiagnosticError || forced.Diagnostics[0].Code != d.Code || forced.Diagnostics[0].Source != d.Source {
		t.Fatalf("forced Mode 0 = %+v", forced)
	}
}

func TestCheckProjectSuppressesSelectionAfterInvalidEdits(t *testing.T) {
	root := checkProjectFixture(t)
	service := Service{UseProjectRenderer: true}
	for _, tc := range []struct{ name, source, code string }{
		{"syntax", "function _draw(\n", "LUA_SYNTAX"},
		{"semantic", "function _draw() cls(0) btn() end\n", "TRANSPILE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			analysis := service.CheckProject(root, map[string]string{"game.lua": tc.source})
			if analysis.RendererReady || len(analysis.Diagnostics) != 1 || analysis.Diagnostics[0].Code != tc.code || analysis.Diagnostics[0].Severity != DiagnosticError {
				t.Fatalf("analysis = %+v", analysis)
			}
		})
	}
	analysis := service.CheckProject(root, map[string]string{"game.lua": "function _draw() cls(0) end\n"})
	if !analysis.RendererReady || analysis.Renderer.Selected != transpiler.RendererMode0 || len(analysis.Diagnostics) != 0 {
		t.Fatalf("valid analysis = %+v", analysis)
	}
	if err := os.Remove(filepath.Join(root, "assets", "spritesheet.json")); err != nil {
		t.Fatal(err)
	}
	analysis = service.CheckProject(root, nil)
	if analysis.RendererReady || len(analysis.Diagnostics) != 1 || analysis.Diagnostics[0].Code != "ASSET" {
		t.Fatalf("missing assets = %+v", analysis)
	}
}

func TestDiagnosticLocationsDistinguishPrefixFromWindowsDrive(t *testing.T) {
	for _, tc := range []struct{ message, source string }{
		{"transpiler: src/view.lua:2:26: detail", "src/view.lua"},
		{"transpiler: C:\\My Games\\view.lua:2:26: detail", "C:\\My Games\\view.lua"},
		{"/tmp/my project/game.lua:2:26: detail", "/tmp/my project/game.lua"},
	} {
		match := diagnosticLocation.FindStringSubmatch(tc.message)
		if match == nil || strings.TrimSpace(match[1]) != tc.source {
			t.Fatalf("location in %q = %v", tc.message, match)
		}
	}
}
