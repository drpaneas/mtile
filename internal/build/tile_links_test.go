package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestBuildRejectsExpandedLinkedTileWriteBeforeToolchain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "linked")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 32, SheetHeight: 8, MapWidth: 30, MapHeight: 20}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SavePoses(dir, &assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Parts: []assetsync.PosePartRecord{{Tile: 0}}}}}); err != nil {
		t.Fatal(err)
	}
	pose := 0
	if err := assetsync.SaveTileLinks(dir, &assetsync.TileLinkFile{Version: 1, Links: []assetsync.TileLink{{Tile: 1, Pose: &pose}}}); err != nil {
		t.Fatal(err)
	}
	source := "function values() return 3,3,1 end\nfunction _update()\n mset(values())\nend\nfunction _draw() cls() map() end\n"
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	service := Service{Renderer: transpiler.RendererMode0, TinyGoBin: filepath.Join(t.TempDir(), "must-not-run"), RuntimeDir: t.TempDir()}
	_, err := service.Build(dir, "")
	if err == nil || !strings.Contains(err.Error(), "[M0_TILE_LINKS]") || !strings.Contains(err.Error(), "game.lua:3:") {
		t.Fatalf("build error = %v, want positioned M0_TILE_LINKS before TinyGo", err)
	}
}
