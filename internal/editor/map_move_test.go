package editor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func mapMoveTestModel() *Model {
	return &Model{
		Project:       assetsync.Project{ActivePalette: "test"},
		Sheet:         assetsync.Spritesheet{Width: 32, Height: 32},
		Map:           assetsync.Map{Width: 5, Height: 4, Cells: make([]int, 20)},
		ActivePalette: editorPalette("test", 16),
	}
}

func TestMoveMapAreaRelocatesOverlappingCellsAtomically(t *testing.T) {
	m := mapMoveTestModel()
	m.Map.Cells = []int{
		1, 2, 3, 0, 0,
		4, 5, 6, 0, 0,
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
	}

	changed, err := m.MoveMapArea(0, 0, 0, 3, 2, 1, 1)
	if err != nil || !changed {
		t.Fatalf("move changed=%t error=%v", changed, err)
	}
	want := []int{
		0, 0, 0, 0, 0,
		0, 1, 2, 3, 0,
		0, 4, 5, 6, 0,
		0, 0, 0, 0, 0,
	}
	if !reflect.DeepEqual(m.Map.Cells, want) {
		t.Fatalf("moved map = %v, want %v", m.Map.Cells, want)
	}
}

func TestMoveMapAreaChangesOnlyRequestedLayer(t *testing.T) {
	m := mapMoveTestModel()
	m.Map.Cells[0] = 7
	if _, err := m.SetMapCell(2, 1, 1, 9); err != nil {
		t.Fatal(err)
	}
	bg0Before := append([]int(nil), m.Map.LayerCells(0)...)

	changed, err := m.MoveMapArea(2, 1, 1, 1, 1, 3, 2)
	if err != nil || !changed {
		t.Fatalf("layer move changed=%t error=%v", changed, err)
	}
	if m.MapCell(2, 1, 1) != 0 || m.MapCell(2, 3, 2) != 9 {
		t.Fatalf("BG2 source/destination = %d/%d, want 0/9", m.MapCell(2, 1, 1), m.MapCell(2, 3, 2))
	}
	if !reflect.DeepEqual(m.Map.LayerCells(0), bg0Before) {
		t.Fatal("BG2 move changed BG0")
	}
}

func TestCopyMapAreaPreservesSource(t *testing.T) {
	m := mapMoveTestModel()
	m.Map.Cells[0], m.Map.Cells[1] = 1, 2

	changed, err := m.CopyMapArea(0, 0, 0, 2, 1, 2, 2)
	if err != nil || !changed {
		t.Fatalf("copy changed=%t error=%v", changed, err)
	}
	if m.MapCell(0, 0, 0) != 1 || m.MapCell(0, 1, 0) != 2 ||
		m.MapCell(0, 2, 2) != 1 || m.MapCell(0, 3, 2) != 2 {
		t.Fatalf("copy source/destination cells were not preserved: %v", m.Map.Cells)
	}
}

func TestCopyMapAreaSnapshotsOverlappingCells(t *testing.T) {
	m := mapMoveTestModel()
	m.Map.Cells = []int{
		1, 2, 3, 0, 0,
		4, 5, 6, 0, 0,
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
	}

	changed, err := m.CopyMapArea(0, 0, 0, 3, 2, 1, 1)
	if err != nil || !changed {
		t.Fatalf("copy changed=%t error=%v", changed, err)
	}
	want := []int{
		1, 2, 3, 0, 0,
		4, 1, 2, 3, 0,
		0, 4, 5, 6, 0,
		0, 0, 0, 0, 0,
	}
	if !reflect.DeepEqual(m.Map.Cells, want) {
		t.Fatalf("copied map = %v, want %v", m.Map.Cells, want)
	}
}

func TestMoveMapAreaToLayerClearsSourceAndKeepsCoordinates(t *testing.T) {
	m := mapMoveTestModel()
	m.Map.Cells[1], m.Map.Cells[7] = 3, 4

	changed, err := m.MoveMapAreaToLayer(0, 1, 0, 0, m.Map.Width, m.Map.Height, 0, 0, false)
	if err != nil || !changed {
		t.Fatalf("cross-layer move changed=%t error=%v", changed, err)
	}
	if !m.Map.IsLayered() {
		t.Fatal("BG0 to BG1 move did not promote the legacy map")
	}
	if m.MapCell(0, 1, 0) != 0 || m.MapCell(0, 2, 1) != 0 {
		t.Fatal("cross-layer move did not clear BG0")
	}
	if m.MapCell(1, 1, 0) != 3 || m.MapCell(1, 2, 1) != 4 {
		t.Fatal("cross-layer move did not preserve coordinates on BG1")
	}
}

func TestCopyMapAreaToLayerMergesTransparentCells(t *testing.T) {
	m := mapMoveTestModel()
	m.Map.Cells[0] = 5
	if _, err := m.SetMapCell(1, 3, 2, 8); err != nil {
		t.Fatal(err)
	}

	changed, err := m.CopyMapAreaToLayer(0, 1, 0, 0, 2, 1, 2, 2, false)
	if err != nil || !changed {
		t.Fatalf("cross-layer copy changed=%t error=%v", changed, err)
	}
	if m.MapCell(0, 0, 0) != 5 || m.MapCell(1, 2, 2) != 5 || m.MapCell(1, 3, 2) != 8 {
		t.Fatalf("transparent merge source/destination = %d/%d/%d, want 5/5/8", m.MapCell(0, 0, 0), m.MapCell(1, 2, 2), m.MapCell(1, 3, 2))
	}
}

func TestMoveMapAreaToLayerRequiresExplicitCollisionOverwrite(t *testing.T) {
	m := mapMoveTestModel()
	m.Map.Cells[0] = 5
	if _, err := m.SetMapCell(1, 2, 2, 8); err != nil {
		t.Fatal(err)
	}
	before := cloneMap(m.Map)

	changed, err := m.MoveMapAreaToLayer(0, 1, 0, 0, 1, 1, 2, 2, false)
	if err == nil || !strings.Contains(err.Error(), "occupied") || changed {
		t.Fatalf("unconfirmed collision changed=%t error=%v", changed, err)
	}
	if !reflect.DeepEqual(m.Map, before) {
		t.Fatal("rejected cross-layer collision mutated the map")
	}

	changed, err = m.MoveMapAreaToLayer(0, 1, 0, 0, 1, 1, 2, 2, true)
	if err != nil || !changed || m.MapCell(0, 0, 0) != 0 || m.MapCell(1, 2, 2) != 5 {
		t.Fatalf("confirmed collision changed=%t source=%d destination=%d error=%v", changed, m.MapCell(0, 0, 0), m.MapCell(1, 2, 2), err)
	}
}

func TestMoveMapAreaRejectsInvalidDestinationWithoutMutation(t *testing.T) {
	m := mapMoveTestModel()
	m.Map.Cells[0], m.Map.Cells[1] = 1, 2
	before := cloneMap(m.Map)

	changed, err := m.MoveMapArea(0, 0, 0, 2, 1, 4, 3)
	if err == nil || !strings.Contains(err.Error(), "destination") || changed {
		t.Fatalf("invalid move changed=%t error=%v", changed, err)
	}
	if !reflect.DeepEqual(m.Map, before) {
		t.Fatal("invalid move partially mutated the map")
	}
}
