package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestMapAreaFromDragNormalizesAndClampsSelection(t *testing.T) {
	selection := mapAreaFromDrag(2, 4, 3, -8, 20, 6, 5)
	if selection == nil {
		t.Fatal("valid drag did not create a selection")
	}
	if selection.layer != 2 || selection.x != 0 || selection.y != 3 || selection.width != 5 || selection.height != 2 {
		t.Fatalf("selection = %#v, want BG2 (0,3 5x2)", selection)
	}
	if got := mapAreaFromDrag(0, -1, 0, 1, 1, 6, 5); got != nil {
		t.Fatalf("out-of-bounds anchor created selection %#v", got)
	}
	twoTiles := mapAreaFromDrag(1, 2, 3, 3, 3, 6, 5)
	if twoTiles == nil || twoTiles.layer != 1 || twoTiles.x != 2 || twoTiles.y != 3 || twoTiles.width != 2 || twoTiles.height != 1 {
		t.Fatalf("two-tile drag selection = %#v, want BG1 (2,3 2x1)", twoTiles)
	}
}

func TestMapModifierSelectionRequiresPaintGestureOverVisibleMap(t *testing.T) {
	tests := []struct {
		name                                    string
		tool                                    mapTool
		modifier, press, overMap, visible, want bool
	}{
		{"command drag from paint", mapToolPaint, true, true, true, true, true},
		{"no modifier", mapToolPaint, false, true, true, true, false},
		{"no mouse press", mapToolPaint, true, false, true, true, false},
		{"outside map", mapToolPaint, true, true, false, true, false},
		{"hidden layer", mapToolPaint, true, true, true, false, false},
		{"move tool already active", mapToolMoveArea, true, true, true, true, false},
		{"copy tool already active", mapToolCopyArea, true, true, true, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := mapModifierSelectionRequested(test.tool, test.modifier, test.press, test.overMap, test.visible)
			if got != test.want {
				t.Fatalf("mapModifierSelectionRequested() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestMapMovePreviewShowsClearedSourceAndMovedDestination(t *testing.T) {
	g := newTestGame(t, 32, 32, 6, 5)
	for _, cell := range []struct{ x, y, id int }{
		{1, 1, 1}, {2, 1, 2}, {1, 2, 3}, {2, 2, 4}, {4, 4, 7},
	} {
		if _, err := g.model.SetMapCell(0, cell.x, cell.y, cell.id); err != nil {
			t.Fatal(err)
		}
	}
	g.mapTool = mapToolMoveArea
	g.mapArea = &mapAreaSelection{layer: 0, x: 1, y: 1, width: 2, height: 2}
	g.mapAreaPlacing = true
	g.mapAreaTargetLayer = 0
	g.mapAreaTargetX, g.mapAreaTargetY = 3, 2

	if got := g.mapCellForDisplay(0, 1, 1); got != 0 {
		t.Fatalf("preview source cell = %d, want cleared 0", got)
	}
	for _, check := range []struct{ x, y, want int }{
		{3, 2, 1}, {4, 2, 2}, {3, 3, 3}, {4, 3, 4}, {4, 4, 7},
	} {
		if got := g.mapCellForDisplay(0, check.x, check.y); got != check.want {
			t.Fatalf("preview cell (%d,%d) = %d, want %d", check.x, check.y, got, check.want)
		}
	}
}

func TestFinishMapAreaMoveCommitsOnceAndUndoRestoresSource(t *testing.T) {
	g := newTestGame(t, 32, 32, 6, 5)
	for _, cell := range []struct{ x, y, id int }{{1, 1, 1}, {2, 1, 2}, {1, 2, 3}, {2, 2, 4}} {
		if _, err := g.model.SetMapCell(0, cell.x, cell.y, cell.id); err != nil {
			t.Fatal(err)
		}
	}
	g.undo.reset(g.snapshot())
	g.mapTool = mapToolMoveArea
	g.mapArea = &mapAreaSelection{layer: 0, x: 1, y: 1, width: 2, height: 2}
	g.mapAreaPlacing = true
	g.mapAreaTargetX, g.mapAreaTargetY = 3, 2
	undoBefore := len(g.undo.past)

	g.finishMapAreaPlacement(false)
	if g.mapAreaPlacing || g.mapArea.x != 3 || g.mapArea.y != 2 {
		t.Fatalf("finished selection = %#v placing=%t, want destination (3,2)", g.mapArea, g.mapAreaPlacing)
	}
	if got := len(g.undo.past); got != undoBefore+1 {
		t.Fatalf("move added %d undo states, want exactly 1", got-undoBefore)
	}
	for _, check := range []struct{ x, y, want int }{
		{1, 1, 0}, {2, 1, 0}, {1, 2, 0}, {2, 2, 0},
		{3, 2, 1}, {4, 2, 2}, {3, 3, 3}, {4, 3, 4},
	} {
		if got := g.mapCellAt(check.x, check.y); got != check.want {
			t.Fatalf("committed cell (%d,%d) = %d, want %d", check.x, check.y, got, check.want)
		}
	}

	beforeMove, ok := g.undo.undo()
	if !ok {
		t.Fatal("move did not create an undoable state")
	}
	g.restore(beforeMove)
	if g.mapCellAt(1, 1) != 1 || g.mapCellAt(2, 2) != 4 || g.mapCellAt(3, 2) != 0 {
		t.Fatal("undo did not restore the map area source")
	}
	if g.mapArea != nil {
		t.Fatal("undo retained stale transient map selection")
	}
}

func TestFinishMapAreaCopyPreservesSourceAndCommitsOnce(t *testing.T) {
	g := newTestGame(t, 32, 32, 6, 5)
	if _, err := g.model.SetMapCell(0, 3, 2, 7); err != nil {
		t.Fatal(err)
	}
	for _, cell := range []struct{ x, y, id int }{{1, 1, 1}, {2, 1, 2}, {1, 2, 3}, {2, 2, 4}} {
		if _, err := g.model.SetMapCell(1, cell.x, cell.y, cell.id); err != nil {
			t.Fatal(err)
		}
	}
	g.undo.reset(g.snapshot())
	g.mapTool = mapToolCopyArea
	g.mapArea = &mapAreaSelection{layer: 1, x: 1, y: 1, width: 2, height: 2}
	g.mapAreaPlacing = true
	g.mapAreaTargetLayer = 1
	g.mapAreaTargetX, g.mapAreaTargetY = 3, 2
	undoBefore := len(g.undo.past)

	g.finishMapAreaPlacement(false)
	if g.mapAreaPlacing || g.mapArea.x != 3 || g.mapArea.y != 2 {
		t.Fatalf("finished copy selection = %#v placing=%t, want destination (3,2)", g.mapArea, g.mapAreaPlacing)
	}
	if got := len(g.undo.past); got != undoBefore+1 {
		t.Fatalf("copy added %d undo states, want exactly 1", got-undoBefore)
	}
	for _, check := range []struct{ x, y, want int }{
		{1, 1, 1}, {2, 1, 2}, {1, 2, 3}, {2, 2, 4},
		{3, 2, 1}, {4, 2, 2}, {3, 3, 3}, {4, 3, 4},
	} {
		if got := g.mapCellAtLayer(1, check.x, check.y); got != check.want {
			t.Fatalf("copied cell (%d,%d) = %d, want %d", check.x, check.y, got, check.want)
		}
	}
	if got := g.mapCellAtLayer(0, 3, 2); got != 7 {
		t.Fatalf("BG1 copy changed BG0 destination to %d, want 7", got)
	}
}

func TestMapCopyPreviewKeepsSourceAndShowsDestination(t *testing.T) {
	g := newTestGame(t, 32, 32, 6, 5)
	if _, err := g.model.SetMapCell(0, 1, 1, 6); err != nil {
		t.Fatal(err)
	}
	g.mapTool = mapToolCopyArea
	g.mapArea = &mapAreaSelection{layer: 0, x: 1, y: 1, width: 1, height: 1}
	g.mapAreaPlacing = true
	g.mapAreaTargetLayer = 0
	g.mapAreaTargetX, g.mapAreaTargetY = 4, 3

	if got := g.mapCellForDisplay(0, 1, 1); got != 6 {
		t.Fatalf("copy preview source = %d, want preserved 6", got)
	}
	if got := g.mapCellForDisplay(0, 4, 3); got != 6 {
		t.Fatalf("copy preview destination = %d, want copied 6", got)
	}
}

func TestFinishMapAreaMoveRejectsOutOfBoundsWithoutMutation(t *testing.T) {
	g := newTestGame(t, 32, 32, 6, 5)
	if _, err := g.model.SetMapCell(0, 1, 1, 3); err != nil {
		t.Fatal(err)
	}
	g.undo.reset(g.snapshot())
	before := g.snapshot().clone()
	undoBefore := len(g.undo.past)
	g.mapTool = mapToolMoveArea
	g.mapArea = &mapAreaSelection{layer: 0, x: 1, y: 1, width: 2, height: 2}
	g.mapAreaPlacing = true
	g.mapAreaTargetLayer = 0
	g.mapAreaTargetX, g.mapAreaTargetY = 5, 4

	g.finishMapAreaPlacement(false)
	if !reflect.DeepEqual(g.model.Map, before.Map) || len(g.undo.past) != undoBefore {
		t.Fatal("invalid map move mutated data or undo history")
	}
	if g.mapArea == nil || g.mapArea.x != 1 || g.mapArea.y != 1 {
		t.Fatal("invalid move discarded the recoverable source selection")
	}
	if !strings.Contains(g.statusMsg, "Cannot move here") {
		t.Fatalf("invalid move status = %q", g.statusMsg)
	}
}

func TestSelectWholeMapAreaAndMoveToAnotherLayer(t *testing.T) {
	g := newTestGame(t, 32, 32, 6, 5)
	if _, err := g.model.SetMapCell(0, 2, 1, 6); err != nil {
		t.Fatal(err)
	}
	g.undo.reset(g.snapshot())
	g.mapTool = mapToolMoveArea
	g.selectWholeMapArea()
	if g.mapArea == nil || g.mapArea.layer != 0 || g.mapArea.width != 6 || g.mapArea.height != 5 {
		t.Fatalf("whole-layer selection = %#v, want BG0 6x5", g.mapArea)
	}
	if !g.setMapAreaTargetLayer(1) {
		t.Fatal("could not select BG1 as the target layer")
	}
	undoBefore := len(g.undo.past)

	g.finishMapAreaPlacement(false)
	if g.mapCellAtLayer(0, 2, 1) != 0 || g.mapCellAtLayer(1, 2, 1) != 6 {
		t.Fatal("whole-layer move did not transfer BG0 to BG1 at the same coordinates")
	}
	if g.mapActiveLayer != 1 || g.mapArea == nil || g.mapArea.layer != 1 {
		t.Fatalf("destination layer was not selected after transfer: active=%d selection=%#v", g.mapActiveLayer, g.mapArea)
	}
	if got := len(g.undo.past); got != undoBefore+1 {
		t.Fatalf("whole-layer transfer added %d undo states, want exactly 1", got-undoBefore)
	}
}

func TestCrossLayerMovePreviewAndCollisionConfirmation(t *testing.T) {
	g := newTestGame(t, 32, 32, 6, 5)
	if _, err := g.model.SetMapCell(0, 1, 1, 6); err != nil {
		t.Fatal(err)
	}
	if _, err := g.model.SetMapCell(1, 3, 2, 7); err != nil {
		t.Fatal(err)
	}
	g.undo.reset(g.snapshot())
	g.mapTool = mapToolMoveArea
	g.mapArea = &mapAreaSelection{layer: 0, x: 1, y: 1, width: 1, height: 1}
	g.mapAreaTargetLayer = 1
	g.mapAreaTargetX, g.mapAreaTargetY = 3, 2

	if got := g.mapCellForDisplay(0, 1, 1); got != 0 {
		t.Fatalf("cross-layer move preview source = %d, want cleared 0", got)
	}
	if got := g.mapCellForDisplay(1, 3, 2); got != 6 {
		t.Fatalf("cross-layer move preview destination = %d, want source 6", got)
	}
	if collisions := g.mapAreaCollisionCount(); collisions != 1 {
		t.Fatalf("collision count = %d, want 1", collisions)
	}
	before := g.snapshot().clone()
	undoBefore := len(g.undo.past)
	g.finishMapAreaPlacement(false)
	if !reflect.DeepEqual(g.model.Map, before.Map) || len(g.undo.past) != undoBefore {
		t.Fatal("unconfirmed cross-layer overwrite mutated data or undo history")
	}
	if !strings.Contains(g.statusMsg, "hold Shift") {
		t.Fatalf("collision recovery status = %q", g.statusMsg)
	}

	g.finishMapAreaPlacement(true)
	if g.mapCellAtLayer(0, 1, 1) != 0 || g.mapCellAtLayer(1, 3, 2) != 6 {
		t.Fatal("confirmed cross-layer overwrite did not transfer the selected tile")
	}
}
