package editor

import (
	"github.com/drpaneas/mtile/internal/assetsync"
	"testing"
)

func TestMapRoomEdgesPreserveAllLayers(t *testing.T) {
	for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		m, err := Open(editorFixture(t))
		if err != nil {
			t.Fatal(err)
		}
		m.Map, err = assetsync.NewLayeredMap(60, 20)
		if err != nil {
			t.Fatal(err)
		}
		for bg := 0; bg < 4; bg++ {
			m.Map.LayerCells(bg)[3*60+4] = bg + 1
		}
		if err = m.AddMapRoomEdge(d[0], d[1]); err != nil {
			t.Fatal(err)
		}
		x, y := 4, 3
		if d[0] < 0 {
			x += 30
		}
		if d[1] < 0 {
			y += 20
		}
		for bg := 0; bg < 4; bg++ {
			if got := m.Map.LayerCells(bg)[y*m.Map.Width+x]; got != bg+1 {
				t.Fatalf("direction %v layer %d: %d", d, bg, got)
			}
		}
	}
}
func TestMapRoomDeletion(t *testing.T) {
	for _, vertical := range []bool{false, true} {
		m, err := Open(editorFixture(t))
		if err != nil {
			t.Fatal(err)
		}
		w, h := 90, 20
		if vertical {
			w, h = 30, 60
		}
		m.Map, err = assetsync.NewLayeredMap(w, h)
		if err != nil {
			t.Fatal(err)
		}
		for bg := 0; bg < 4; bg++ {
			for i := range m.Map.LayerCells(bg) {
				x, y := i%w, i/w
				room := x / 30
				if vertical {
					room = y / 20
				}
				m.Map.LayerCells(bg)[i] = room + 1
			}
		}
		col, row := 1, 0
		if vertical {
			col, row = 0, 1
		}
		if err = m.DeleteMapRoom(col, row); err != nil {
			t.Fatal(err)
		}
		for bg := 0; bg < 4; bg++ {
			for i, v := range m.Map.LayerCells(bg) {
				room := (i % m.Map.Width) / 30
				if vertical {
					room = (i / m.Map.Width) / 20
				}
				want := 1
				if room == 1 {
					want = 3
				}
				if v != want {
					t.Fatalf("compaction: got %d want %d", v, want)
				}
			}
		}
	}
}
func TestMapRoomClearPreservesNeighbors(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	m.Map, err = assetsync.NewLayeredMap(60, 40)
	if err != nil {
		t.Fatal(err)
	}
	for bg := 0; bg < 4; bg++ {
		for i := range m.Map.LayerCells(bg) {
			m.Map.LayerCells(bg)[i] = 1
		}
	}
	if err = m.DeleteMapRoom(1, 0); err != nil {
		t.Fatal(err)
	}
	for bg := 0; bg < 4; bg++ {
		for i, v := range m.Map.LayerCells(bg) {
			want := 1
			if i%60 >= 30 && i/60 < 20 {
				want = 0
			}
			if v != want {
				t.Fatal("neighbor changed")
			}
		}
	}
	if err = m.DeleteMapRoom(2, 0); err == nil {
		t.Fatal("outside room accepted")
	}
}
