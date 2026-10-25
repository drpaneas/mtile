package transpiler

import (
	"math/rand"
	"strings"
	"testing"

	runtimeapi "github.com/drpaneas/mtile/runtime"
)

// tileLinkOpts links tile 1 to pose 0 (8x16, hanging one cell down) and
// tile 2 to clip "sway". Linked cells sit at (4,2), (9,2), and (4,7).
func tileLinkOpts(renderer Renderer) Options {
	const w, h = 30, 20
	m := make([]uint16, w*h)
	for x := 0; x < w; x++ {
		m[(h-1)*w+x] = 3
	}
	m[2*w+4], m[2*w+9], m[7*w+4] = 1, 2, 1
	sheet := make([]byte, 32*8)
	for i := range sheet {
		sheet[i] = 5
	}
	return Options{
		Renderer: renderer, SheetW: 32, SheetH: 8, MapW: w, MapH: h, MapLayers: 1,
		Sheet: sheet, Map: m, Flags: make([]byte, 4),
		PoseIndex:       []runtimeapi.PoseRange{{Start: 0, Count: 2, FlipOffsetX: 8, FlipOffsetY: 16}},
		PoseParts:       []runtimeapi.PosePart{{Tile: 2}, {Tile: 3, Y: 8}},
		AnimationClips:  []runtimeapi.AnimationClip{{Name: "sway", Count: 1, FPS: 6, Loop: true, FlipOffsetX: 8, FlipOffsetY: 16}},
		AnimationFrames: []uint16{0},
		TileLinks:       []runtimeapi.TileLink{{Tile: 1, ID: 0}, {Tile: 2, Clip: true, ID: 0}},
	}
}

func TestTileLinkInstancesRowsAndReach(t *testing.T) {
	opts := tileLinkOpts(RendererMode4)
	g := &generator{opts: opts, spriteN: 4}
	instances, rows := g.tileLinkInstances()
	want := []runtimeapi.TileLinkInstance{{X: 4, Y: 2}, {X: 9, Y: 2}, {X: 4, Y: 7}}
	if len(instances) != len(want) {
		t.Fatalf("instances = %+v, want %+v", instances, want)
	}
	for i := range want {
		if instances[i] != want[i] {
			t.Fatalf("instance %d = %+v, want %+v (row-major order)", i, instances[i], want[i])
		}
	}
	if len(rows) != opts.MapH+1 || rows[2] != 0 || rows[3] != 2 || rows[7] != 2 || rows[8] != 3 || rows[opts.MapH] != 3 {
		t.Fatalf("row index = %v", rows)
	}
	if reach := tileLinkReach(opts); reach != (runtimeapi.TileLinkReach{Bottom: 8}) {
		t.Fatalf("reach = %+v, want only Bottom 8", reach)
	}
}

func TestTileLinksGenerateTablesAndUpdateClock(t *testing.T) {
	got, err := Transpile("draw.lua", "function _draw()\n  cls()\n  map()\nend\n", tileLinkOpts(RendererMode4))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"var tileLinkOfROM = [ 4 ]uint8{",
		"var tileLinkInstancesROM = [ 3 ]rt.TileLinkInstance{",
		"cartData.TileLinkReach = rt.TileLinkReach{Left: 0, Top: 0, Right: 0, Bottom: 8}",
		// A linked clip needs the update clock even though Lua never calls anim().
		"ctx.AdvanceTick()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated code is missing %q", want)
		}
	}
	if strings.Contains(got, "SetTileLinkSites") {
		t.Error("Mode 4 must not emit Mode 0 object allocations")
	}
}

func TestMode0TileLinksAllocateObjectSites(t *testing.T) {
	got, report, err := TranspileWithReport("draw.lua", "function _draw()\n  cls()\n  map()\nend\n", tileLinkOpts(RendererMode0))
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 with objects", report)
	}
	sites, mapCall := strings.Index(got, "c.SetTileLinkSites("), strings.Index(got, "c.MapAll(")
	if sites < 0 || mapCall < 0 || sites > mapCall {
		t.Fatal("the map() call is not preceded by its linked-object allocation")
	}
	// Three linked cells fit on one screen, each pose one 8x16 object.
	if report.Mode0OAMEntries != 3 {
		t.Fatalf("OAM entries = %d, want 3", report.Mode0OAMEntries)
	}
}

func TestMode0TileLinksRejectMsetOfLinkedTile(t *testing.T) {
	const placing = "function _update()\n  mset(3, 3, 1)\nend\nfunction _draw()\n  cls()\n  map()\nend\n"
	_, err := Transpile("draw.lua", placing, tileLinkOpts(RendererMode0))
	if err == nil || !strings.Contains(err.Error(), "[M0_TILE_LINKS]") || !strings.Contains(err.Error(), "draw.lua:2:") {
		t.Fatalf("forced Mode 0 error = %v, want a positioned M0_TILE_LINKS diagnostic", err)
	}
	_, report, err := TranspileWithReport("draw.lua", placing, tileLinkOpts(RendererAuto))
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "M0_TILE_LINKS") {
		t.Fatalf("AUTO report = %+v, want Mode 4 with an M0_TILE_LINKS diagnostic", report)
	}
	// Clearing a linked cell keeps the bound sound.
	const clearing = "function _update()\n  mset(4, 2, 0)\nend\nfunction _draw()\n  cls()\n  map()\nend\n"
	if _, err := Transpile("draw.lua", clearing, tileLinkOpts(RendererMode0)); err != nil {
		t.Fatalf("mset of an unlinked literal: %v", err)
	}
}

func TestTileLinksRejectExpandedMsetArgumentsInMode0(t *testing.T) {
	for _, tc := range []struct{ name, prelude, call string }{
		{"all arguments", "function values() return 3, 3, 1 end", "mset(values())"},
		{"coordinates and tile", "function values() return 3, 3, 1 end", "mset(0, values())"},
		{"layered tail", "function values() return 3, 1 end", "mset(0, 3, values())"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.prelude + "\nfunction _update()\n  " + tc.call + "\nend\nfunction _draw() cls() map() end\n"
			_, err := Transpile("expanded.lua", src, tileLinkOpts(RendererMode0))
			if err == nil || !strings.Contains(err.Error(), "[M0_TILE_LINKS]") || !strings.Contains(err.Error(), "expanded.lua:3:") {
				t.Fatalf("forced Mode 0 error = %v, want positioned M0_TILE_LINKS", err)
			}
			_, report, err := TranspileWithReport("expanded.lua", src, tileLinkOpts(RendererAuto))
			if err != nil || report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "M0_TILE_LINKS") {
				t.Fatalf("AUTO report = %+v, error %v", report, err)
			}
			if _, err := Transpile("expanded.lua", src, tileLinkOpts(RendererMode4)); err != nil {
				t.Fatalf("Mode 4 must preserve expanded arguments: %v", err)
			}
			// Without linked tiles, mset has no object-count restriction.
			opts := tileLinkOpts(RendererMode0)
			opts.TileLinks = nil
			if _, err := Transpile("expanded.lua", src, opts); err != nil {
				t.Fatalf("unlinked Mode 0 map rejected expanded arguments: %v", err)
			}
		})
	}
}

func TestTileLinkRowsPreserveLargeInstanceOffsets(t *testing.T) {
	opts := tileLinkOpts(RendererMode4)
	opts.MapW, opts.MapH = 512, 129
	opts.Map = make([]uint16, opts.MapW*opts.MapH)
	for i := range opts.Map {
		opts.Map[i] = 1
	}
	g := &generator{opts: opts, spriteN: 4}
	instances, rows := g.tileLinkInstances()
	if len(instances) != len(opts.Map) || len(rows) != opts.MapH+1 {
		t.Fatalf("got %d instances and %d rows", len(instances), len(rows))
	}
	for y, offset := range rows {
		if int(offset) != y*opts.MapW {
			t.Fatalf("row %d offset = %d, want %d", y, offset, y*opts.MapW)
		}
	}
	var b sourceBuilder
	g.renderTileLinkTables(&b)
	if !strings.Contains(b.String(), "var tileLinkRowsROM = [ 130 ]uint32{") {
		t.Fatal("generated row offsets must use uint32")
	}
}

func TestTileLinkFilteredMapShapesRemainMode0Eligible(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode0, RendererAuto} {
		for _, size := range []string{"0,0", "30,20"} {
			opts := tileLinkOpts(renderer)
			opts.MapW, opts.MapH = 32, 22
			opts.Map = make([]uint16, opts.MapW*opts.MapH)
			opts.Map[2*opts.MapW+30], opts.Map[20*opts.MapW+2], opts.Map[20*opts.MapW+30] = 1, 2, 1
			opts.PoseParts[0].X, opts.PoseParts[0].Y = -8, -8
			src := "mask=0\nfunction _update() mask=(mask+1)%4 mset(0,0,0) end\nfunction _draw() cls() map(0,0,0,0,0," + size + ",mask) end"
			_, report, err := TranspileWithReport("filtered.lua", src, opts)
			if err != nil || report.Selected != RendererMode0 || !report.Mode0Objects {
				t.Fatalf("renderer %s size %s: report %+v, error %v", renderer, size, report, err)
			}
		}
	}
}

func TestMode0TileLinkWindowMaxMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	table := []uint8{0, 1}
	for trial := 0; trial < 40; trial++ {
		w, h := 1+rng.Intn(40), 1+rng.Intn(25)
		opts := Options{MapW: w, MapH: h, Map: make([]uint16, w*h)}
		for i := range opts.Map {
			if rng.Intn(4) == 0 {
				opts.Map[i] = 1
			}
		}
		ww, wh := 1+rng.Intn(35), 1+rng.Intn(22)
		want := 0
		for y0 := 0; y0 < h; y0++ {
			for x0 := 0; x0 < w; x0++ {
				count := 0
				for y := y0; y < min(h, y0+wh); y++ {
					for x := x0; x < min(w, x0+ww); x++ {
						count += int(opts.Map[y*w+x])
					}
				}
				want = max(want, count)
			}
		}
		if got := mode0TileLinkWindowMax(opts, table, 0, ww, wh); got != want {
			t.Fatalf("trial %d (%dx%d map, %dx%d window): got %d, want %d", trial, w, h, ww, wh, got, want)
		}
	}
}
