package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

func newTileLinkGame(t *testing.T) *Game {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "tile-links"))); err != nil {
		t.Fatal(err)
	}
	g, err := newGame(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.closeNativePerformance)
	g.mode = modeSprites
	return g
}

func TestSpriteLabLinkRowLinksUnlinksAndUndoes(t *testing.T) {
	g := newTileLinkGame(t)
	g.currentSprite, g.gridCols, g.gridRows = 2, 1, 1 // the dirt tile, unlinked
	g.syncTileLinkChoice()
	targets := g.tileLinkTargets()
	clip := -1
	for i, target := range targets {
		if target.pose < 0 && target.clip == "sway" {
			clip = i
		}
	}
	if clip < 0 {
		t.Fatalf("targets %+v do not offer clip sway", targets)
	}
	g.tileLinkChoice = clip
	g.linkSelectedTiles(targets)
	if link, ok := g.model.TileLinkFor(2); !ok || link.Animation != "sway" {
		t.Fatalf("tile 2 link = %+v, %t; want clip sway", link, ok)
	}
	if !g.tileLinked(2) {
		t.Fatal("Map Lab would not highlight the newly linked tile")
	}

	// Selecting an already linked tile points the selector at its target.
	g.currentSprite = 5 // lamp marker, linked to pose 3
	g.syncTileLinkChoice()
	if got := targets[g.tileLinkChoice]; got.pose != 3 {
		t.Fatalf("selector after selecting the lamp tile = %+v, want pose 3", got)
	}

	g.currentSprite = 2
	g.unlinkSelectedTiles()
	if _, ok := g.model.TileLinkFor(2); ok {
		t.Fatal("UNLINK left tile 2 linked")
	}
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("unlink pushed no undo state")
	}
	g.restore(snapshot)
	if link, ok := g.model.TileLinkFor(2); !ok || link.Animation != "sway" {
		t.Fatalf("undo did not restore the link: %+v, %t", link, ok)
	}
}

func TestSpriteLabRefusesToLinkTheMapEraser(t *testing.T) {
	g := newTileLinkGame(t)
	g.currentSprite, g.gridCols, g.gridRows = 0, 1, 1
	g.linkSelectedTiles(g.tileLinkTargets())
	if _, ok := g.model.TileLinkFor(0); ok {
		t.Fatal("linked the transparent map eraser")
	}
	if g.statusMsg == "" {
		t.Fatal("no explanation was shown")
	}
}

// The link row shares the flags panel with the flag checkboxes. It must stay
// inside the panel at every text size and on both sides of each layout
// breakpoint, without growing the panel (which would shrink the canvas).
func TestSpriteLabLinkRowFitsFlagsPanel(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for textSize := textCompact; textSize < interfaceTextSizeCount; textSize++ {
		setInterfaceTextSize(textSize)
		for height := minimumScreenHeight; height <= 1250; height += 7 {
			setEditorLayout(minimumScreenWidth, height)
			panel := spriteFlagsPanelRect
			for _, r := range []editor.Rect{tileLinkPrevRect(), tileLinkTargetRect(), tileLinkNextRect(), tileLinkLinkRect(), tileLinkUnlinkRect()} {
				if r.X < panel.X || r.X+r.W > panel.X+panel.W || r.Y+r.H > panel.Y+panel.H {
					t.Fatalf("%s at height %d: link control %+v leaves flags panel %+v", textSize, height, r, panel)
				}
				if r.Y < flagCheckboxRect(0).Y+flagCheckboxRect(0).H {
					t.Fatalf("%s at height %d: link row overlaps the flag checkboxes", textSize, height)
				}
			}
		}
	}
}

func TestSpriteLabLinkButtonsEnableOnlyApplicableAction(t *testing.T) {
	g := newTileLinkGame(t)
	for _, test := range []struct {
		name               string
		tile, cols         int
		canLink, canUnlink bool
	}{
		{"linked flower", 4, 1, false, true},
		{"unlinked dirt", 2, 1, true, false},
		{"map eraser", 0, 1, false, false},
		{"mixed selection", 3, 2, true, true}, // cursor (unlinked) and flower (linked)
	} {
		g.currentSprite, g.gridCols, g.gridRows = test.tile, test.cols, 1
		canLink, canUnlink := g.tileLinkActions()
		if canLink != test.canLink || canUnlink != test.canUnlink {
			t.Errorf("%s: LINK %t UNLINK %t, want %t %t", test.name, canLink, canUnlink, test.canLink, test.canUnlink)
		}
	}
}

// Regression: opening Poses in a project whose poses use shared tiles creates
// an object sheet and remaps pose parts. Undoing a later edit used to restore
// the shared-sheet pose IDs while the repacked objsheet.json stayed on disk,
// so every pose drew the wrong tiles.
func TestUndoAfterObjSheetCreationKeepsPosesConsistent(t *testing.T) {
	g := newTestGame(t, 64, 16, 4, 4)
	for tile, color := range map[int]int{1: 3, 2: 9, 3: 12, 4: 14} {
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if err := g.model.SetSpritePixel(tile, x, y, color); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	// A legacy layout: the pose is built from shared tiles 3 and 4.
	g.model.Poses.Poses = []assetsync.PoseRecord{{Name: "p", Parts: []assetsync.PosePartRecord{{Tile: 3}, {Tile: 4, X: 8}}}}
	if err := g.model.Save(); err != nil {
		t.Fatal(err)
	}
	g.undo.reset(g.snapshot())
	pixels := func(m *editor.Model) []int {
		sheet := &m.Sheet
		if m.ObjSheet != nil {
			sheet = m.ObjSheet
		}
		var colors []int
		for _, part := range m.Poses.Poses[0].Parts {
			for _, sprite := range sheet.Sprites {
				if sprite.ID == part.Tile {
					colors = append(colors, sprite.Pixels[0][0])
				}
			}
		}
		return colors
	}
	want := pixels(g.model)
	g.mode = modeSprites
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.undo.forcePush(g.snapshot()) // as the Poses tab does after creating the sheet
	if err := g.model.Save(); err != nil {
		t.Fatal(err)
	}
	if err := g.setObjEditing(false); err != nil {
		t.Fatal(err)
	}
	g.currentSprite, g.gridCols, g.gridRows = 1, 1, 1
	g.linkSelectedTiles(g.tileLinkTargets())
	for {
		s, ok := g.undo.undo()
		if !ok {
			break
		}
		g.restore(s)
		if err := g.model.Save(); err != nil {
			t.Fatal(err)
		}
		reopened, err := editor.Open(g.model.Dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := pixels(reopened); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("after undo and reload the pose draws colors %v, want %v", got, want)
		}
	}
}
