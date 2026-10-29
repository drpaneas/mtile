package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestLumenMode0FitsWithSystemPauseOverlay(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "lumen")
	analysis := (Service{Renderer: transpiler.RendererMode0}).CheckProject(root, nil)
	if !analysis.RendererReady {
		t.Fatalf("Lumen must build in forced Mode 0: %+v", analysis.Diagnostics)
	}
	if analysis.Renderer.Selected != transpiler.RendererMode0 || analysis.Renderer.Mode0OBJLineCycles > 1162 || analysis.Renderer.Mode0OAMEntries > 127 {
		t.Fatalf("unsafe renderer budget: %+v", analysis.Renderer)
	}
	// Restoring the original five decorative motes must still fail: this proves
	// the example fix cannot be replaced by relaxing the hardware/pause limit.
	source, err := os.ReadFile(filepath.Join(root, "src", "draw.lua"))
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(string(source), "for i = 1, 3 do\n    local x = (i*79+age/20-scroll)", "for i = 1, 5 do\n    local x = (i*79+age/20-scroll)", 1)
	rejected := (Service{Renderer: transpiler.RendererMode0}).CheckProject(root, map[string]string{"src/draw.lua": old})
	if rejected.RendererReady || len(rejected.Diagnostics) == 0 || rejected.Diagnostics[0].Code != "M0_OBJ_BUDGET" {
		t.Fatal("unsafe original scene no longer rejected")
	}
}
