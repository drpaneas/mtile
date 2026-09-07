package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoRendererFallbackReportsOriginalModulePosition(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "module-renderer")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"src/view.lua": "function draw_scene()\n  circfill(4,4,1,7)\nend\n",
		"game.lua":     "require(\"view\")\nfunction _draw()\n cls(0)\n draw_scene()\nend\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var fallback string
	service := Service{
		UseProjectRenderer: true,
		TinyGoBin:          filepath.Join(t.TempDir(), "missing-tinygo"),
		RuntimeDir:         t.TempDir(),
		Trace: func(event Event) {
			if strings.HasPrefix(event.Message, "Renderer fallback:") {
				fallback = event.Message
			}
		},
	}
	// Renderer selection and its diagnostic precede toolchain discovery.
	// This regression needs no native build to verify the source mapping.
	if _, err := service.Build(dir, ""); err == nil {
		t.Fatal("missing toolchain unexpectedly succeeded")
	}
	if !strings.Contains(fallback, "src/view.lua:2:3") || !strings.Contains(fallback, "[M0_MODE4_BUILTIN]") || !strings.Contains(fallback, "circfill()") {
		t.Fatalf("fallback diagnostic = %q, want original module position and unchanged reason", fallback)
	}
	if strings.Contains(fallback, "game.lua:") {
		t.Fatalf("fallback retained the bundled position: %q", fallback)
	}
}
