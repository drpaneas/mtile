package transpiler

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
)

func residentFixtureOptions(cells int) Options {
	opts := psprMode0Opts()
	opts.ObjSheetW, opts.ObjSheetH = 8, cells*8
	opts.ObjSheet = make([]byte, cells*64)
	opts.PoseIndex = nil
	for id := 0; id < cells; id++ {
		// More than 256 distinct patterns, with a transparent pixel too.
		cell := opts.ObjSheet[id*64 : (id+1)*64]
		cell[0], cell[1], cell[2] = byte(id), byte(id>>8), 17
		opts.PoseParts = append(opts.PoseParts, rt.PosePart{Tile: uint16(id)})
		opts.PoseIndex = append(opts.PoseIndex, rt.PoseRange{Start: uint32(id), Count: 1})
	}
	return opts
}

func residentFixturePlan(t *testing.T, opts Options, source string) *mode0ResidentPosePlan {
	t.Helper()
	chunk, err := luaparser.ParseString("resident.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range chunk.Body {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok && functionKey(fn) == "_draw" {
			ops, _, err := mode0WalkDraw(chunk, fn, newMode0SpriteIDAnalysis(chunk), opts)
			if err != nil {
				t.Fatal(err)
			}
			return buildMode0ResidentPosePlan(chunk, ops, opts)
		}
	}
	t.Fatal("missing draw")
	return nil
}

func TestMode0ResidentPoseCapacityAndImmutableFallback(t *testing.T) {
	const source = "function _draw() cls() pspr(frame,0,0) end"
	for _, cells := range []int{511, 512, 513} {
		opts := residentFixtureOptions(cells)
		plan := residentFixturePlan(t, opts, source)
		if cells <= 512 {
			if plan == nil || len(plan.words) != cells*16 || plan.base != 0 {
				t.Fatalf("%d cells: plan=%+v", cells, plan)
			}
		} else if plan != nil {
			t.Fatal("oversized bank must retain the streaming proof")
		}
	}
	for _, source := range []string{
		"function _init() sset(0,0,1) end " + source,
		"function _update() sset(0,0,1) end " + source,
	} {
		if residentFixturePlan(t, residentFixtureOptions(2), source) != nil {
			t.Fatal("mutable sources must retain the existing streaming path")
		}
	}
	opts := residentFixtureOptions(513)
	if plan := residentFixturePlan(t, opts, "function _draw() cls() pspr(512,0,0) end"); plan == nil || len(plan.words) != 16 || plan.index[0].Count != 0 {
		t.Fatal("only reachable poses should occupy the resident bank")
	}
}

func TestMode0ResidentPoseDeduplicatesPackedPixelsAndPreservesLayout(t *testing.T) {
	opts := residentFixtureOptions(8)
	copy(opts.ObjSheet[4*64:], opts.ObjSheet[:4*64])
	opts.PoseParts = []rt.PosePart{
		{Tile: 0}, {Tile: 1, X: 8}, {Tile: 2, X: 16}, {Tile: 3, X: 24},
		{Tile: 4, X: -8}, {Tile: 5}, {Tile: 6, X: 8}, {Tile: 7, X: 16},
		{Tile: 1, X: 3, Y: -2, Flags: rt.PoseFlipX}, {Tile: 5, X: 4, Y: -1, Flags: rt.PoseFlipY},
	}
	opts.PoseIndex = []rt.PoseRange{{Count: 4}, {Start: 4, Count: 4}, {Start: 8, Count: 2}}
	plan := residentFixturePlan(t, opts, "function _draw() cls() pspr(frame,0,0) end")
	if plan == nil || len(plan.words) != 4*16 {
		t.Fatalf("equal packed source-pixel runs must share exactly four tiles: %+v", plan)
	}
	bank := make([]byte, len(plan.words)*4)
	for i, word := range plan.words {
		binary.LittleEndian.PutUint32(bank[i*4:], word)
	}
	for id, pose := range opts.PoseIndex {
		objects, tiles := rt.BuildPoseOBJPlan(opts.PoseParts[pose.Start : int(pose.Start)+int(pose.Count)])
		r := plan.index[id]
		got := plan.parts[r.Start : r.Start+r.Count]
		for index, before := range objects {
			after := got[index]
			if after.X != before.X || after.Y != before.Y || after.Width != before.Width || after.Height != before.Height || after.Flags != before.Flags {
				t.Fatalf("pose%d object%d layout changed", id, index)
			}
			count := int(before.Width) * int(before.Height) / 64
			for cell := 0; cell < count; cell++ {
				want := opts.ObjSheet[int(tiles[int(before.TileOffset)+cell])*64:][:64]
				pixels := bank[(int(after.TileOffset)+cell)*64:][:64]
				if !bytes.Equal(want, pixels) {
					t.Fatalf("pose%d object%d tile%d changed pixels", id, index, cell)
				}
			}
		}
	}
}

func TestMode0ResidentPoseHonorsLegacyTextAndPackingReservations(t *testing.T) {
	opts := residentFixtureOptions(480)
	opts.SheetW, opts.SheetH = 128, 32
	for _, tc := range []struct {
		source string
		want   bool
	}{
		{"pspr(frame,0,0) print(\"A\",0,0,7)", true},
		{"spr(0,0,0) pspr(frame,0,0) print(\"A\",0,0,7)", false},
		{"spr(0,0,0,4,4) pspr(frame,0,0)", false},
	} {
		plan := residentFixturePlan(t, opts, "function _draw() cls() "+tc.source+" end")
		if (plan != nil) != tc.want {
			t.Fatalf("reserved range mismatch for %s: present=%t", tc.source, plan != nil)
		}
	}
}

func TestMode0ResidentPoseAdmitsFourBackgroundsAndNativeAudio(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode0, RendererAuto} {
		opts := mode0AudioCommitOptions(t)
		poses := residentFixtureOptions(2)
		opts.PoseIndex, opts.PoseParts = poses.PoseIndex, poses.PoseParts
		opts.ObjSheetW, opts.ObjSheetH, opts.ObjSheet = poses.ObjSheetW, poses.ObjSheetH, poses.ObjSheet
		opts.Renderer = renderer
		opts.AnimationClips = []rt.AnimationClip{{Name: "loop", Count: 2, FPS: 8, Loop: true}}
		opts.AnimationFrames = []uint16{0, 1}
		code, report, err := TranspileWithReport("resident.lua", `function _init() music(0) end
function _draw()
 cls()
 map()
 for i=1,100 do anim("loop",i,20) end
end`, opts)
		if err != nil {
			t.Fatal(err)
		}
		if report.Selected != RendererMode0 || !report.Mode0PoseResident || report.Mode0PoseStreaming || report.Mode0OAMEntries != 100 || report.Mode0Layers != 4 {
			t.Fatalf("report=%+v", report)
		}
		if !strings.Contains(code, "PoseOBJResident: poseOBJResidentROM[:]") || strings.Contains(code, "var poseOBJStageRAM") || strings.Contains(code, "c.AnimIDSite") {
			t.Fatal("resident code should use a ROM bank without per-occurrence streaming windows")
		}
	}
}

func TestMode0ResidentPoseRetainsOAMAndUnboundedRejections(t *testing.T) {
	opts := residentFixtureOptions(1)
	for _, source := range []string{
		"function _draw() cls() for i=1,129 do pspr(0,i,0) end end",
		"function _draw() cls() for p in all(particles) do pspr(0,p.x,p.y) end end",
	} {
		if _, _, err := TranspileWithReport("resident.lua", source, opts); err == nil {
			t.Fatal("resident art does not relax finite draw or hardware OAM limits")
		}
	}
}

func TestMode0ResidentPoseKeepsAlreadyProvenStreamingScenes(t *testing.T) {
	opts := residentFixtureOptions(2)
	code, report, err := TranspileWithReport("small.lua", "function _draw() cls() pspr(frame,0,0) end", opts)
	if err != nil || report.Mode0PoseResident || !report.Mode0PoseStreaming || !strings.Contains(code, "var poseOBJStageRAM") {
		t.Fatalf("small proven stream must retain its allocation: report=%+v err=%v", report, err)
	}
}
