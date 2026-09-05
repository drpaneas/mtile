package editor

import (
	"slices"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestEditorSavePreservesIWRAMFunctions(t *testing.T) {
	dir := editorFixture(t)
	project, err := assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	project.IWRAMFunctions = []string{"draw_background", "update_smoke"}
	if err := assetsync.SaveProject(dir, project); err != nil {
		t.Fatal(err)
	}
	model, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := assetsync.LoadProject(dir)
	if err != nil || !slices.Equal(loaded.IWRAMFunctions, project.IWRAMFunctions) {
		t.Fatalf("saved placement = %v, %v", loaded.IWRAMFunctions, err)
	}
}
