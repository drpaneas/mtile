package transpiler

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

// Small immutable libraries can keep their complete reachable OBJ layouts in
// VRAM. Unlike streaming windows, the bank is shared by all draw occurrences.
// Every packed object's row-major pixels must remain contiguous; equal runs
// (including runs inside a larger object) may share storage exactly.
type mode0ResidentPosePlan struct {
	base     int
	words    []uint32
	index    []runtimeapi.PoseOBJRange
	parts    []runtimeapi.PoseOBJ
	palettes []uint8
}

func planMode0ResidentPoses(chunk *luaparser.Chunk, ops []mode0DrawOp, opts Options) *mode0ResidentPosePlan {
	// Keep established streaming scenes and their small staging allocations.
	// Residency is an exact alternative when occurrence windows or their
	// worst-case visible uploads would otherwise exceed a hardware budget.
	if len(opts.PoseVariantNames) > 0 {
		return buildMode0ResidentPosePlan(chunk, ops, opts)
	}
	sites := mode0PsprSitesFromOps(ops)
	if len(sites) == 0 {
		return nil
	}
	tiles := 0
	for _, site := range sites {
		tiles += site.uniqueMax
	}
	needsResident := len(sites) > mode0PsprSiteLimit || mode0PsprWindowStart(ops)+tiles > mode0PsprTileLimit(chunk, ops, opts)
	if !needsResident {
		maps, err := validateMapContract(chunk, opts)
		if err != nil || tiles == 0 || mode0PoseCommitCycles(chunk, opts, tiles, mapMaskCount(maps.drawMask), 0) <= 67*1232 {
			return nil
		}
	}
	return buildMode0ResidentPosePlan(chunk, ops, opts)
}

func buildMode0ResidentPosePlan(chunk *luaparser.Chunk, ops []mode0DrawOp, opts Options) *mode0ResidentPosePlan {
	if len(opts.PoseVariantNames) > 0 {
		for _, name := range []string{"spr", "sspr", "print", "pal", "palt", "sset"} {
			if chunkCallsBuiltin(chunk, name) {
				return nil
			}
		}
	}
	if len(opts.PoseIndex) == 0 || chunkCallsBuiltin(chunk, "sset") {
		return nil
	}
	sheet, width, height := opts.Sheet, opts.SheetW, opts.SheetH
	if opts.ObjSheetW != 0 {
		sheet, width, height = opts.ObjSheet, opts.ObjSheetW, opts.ObjSheetH
	}
	if width < 8 || height < 8 || width%8 != 0 || height%8 != 0 || len(sheet) < width*height {
		return nil
	}
	base, limit := mode0PsprWindowStart(ops), mode0PsprTileLimit(chunk, ops, opts)
	if base >= limit {
		return nil
	}
	reachable := make([]bool, len(opts.PoseIndex))
	for _, op := range ops {
		if op.name == "pspr" {
			for _, id := range op.psprIDs {
				if id >= 0 && id < len(reachable) {
					reachable[id] = true
				}
			}
		}
	}
	plan := &mode0ResidentPosePlan{base: base, index: make([]runtimeapi.PoseOBJRange, len(opts.PoseIndex))}
	var layouts map[[2]int]mode0PaletteLayout
	if len(opts.PoseVariantNames) > 0 {
		layouts, plan.palettes = mode0PaletteLayouts(opts, reachable)
		if layouts == nil {
			return nil
		}
	}

	type objectPixels struct {
		part   int
		pixels []byte
	}
	var runs []objectPixels
	for id, pose := range opts.PoseIndex {
		if !reachable[id] {
			continue
		}
		start, end := int(pose.Start), int(pose.Start)+int(pose.Count)
		if start < 0 || end < start || end > len(opts.PoseParts) {
			return nil
		}
		objects, tiles := runtimeapi.BuildPoseOBJPlan(opts.PoseParts[start:end])
		plan.index[id] = runtimeapi.PoseOBJRange{Start: uint32(len(plan.parts)), Count: uint32(len(objects))}
		for objectIndex, object := range objects {
			count := int(object.Width) * int(object.Height) / 64
			pixels := make([]byte, count*64)
			for cell, tile := range tiles[int(object.TileOffset) : int(object.TileOffset)+count] {
				// Invalid source cells are transparent in both renderers.
				if int(tile) >= width/8*(height/8) {
					continue
				}
				x, y := int(tile)%(width/8)*8, int(tile)/(width/8)*8
				for row := 0; row < 8; row++ {
					copy(pixels[cell*64+row*8:cell*64+row*8+8], sheet[(y+row)*width+x:(y+row)*width+x+8])
				}
			}
			if len(opts.PoseVariantNames) > 0 {
				layout, ok := layouts[[2]int{id % opts.PoseVariantBaseCount, objectIndex}]
				if !ok {
					return nil
				}
				pixels = layout.pixels
				object.Palette = layout.banks[id/opts.PoseVariantBaseCount]
			}
			runs = append(runs, objectPixels{part: len(plan.parts), pixels: pixels})
			plan.parts = append(plan.parts, object)
		}
	}
	if len(runs) == 0 {
		return nil
	}
	// Larger runs first let smaller objects reuse tiles within them. Stable
	// ordering makes the exact allocation independent of map iteration order.
	sort.SliceStable(runs, func(i, j int) bool { return len(runs[i].pixels) > len(runs[j].pixels) })
	var bank []byte
	unit := 64
	if len(plan.palettes) > 0 {
		unit = 32
	}
	for _, run := range runs {
		offset := -1
		for at := 0; at+len(run.pixels) <= len(bank); at += unit {
			if bytes.Equal(bank[at:at+len(run.pixels)], run.pixels) {
				offset = at
				break
			}
		}
		if offset < 0 {
			// Reuse a matching suffix before appending the remainder. This is
			// exact sequence sharing, never a pixel/shape approximation.
			overlap := min(len(bank), len(run.pixels))
			for overlap > 0 && !bytes.Equal(bank[len(bank)-overlap:], run.pixels[:overlap]) {
				overlap -= unit
			}
			offset = len(bank) - overlap
			bank = append(bank, run.pixels[overlap:]...)
			if (len(bank)+63)/64 > limit-base {
				return nil
			}
		}
		plan.parts[run.part].TileOffset = uint16(offset / unit)
	}
	for id := range plan.index {
		if reachable[id] {
			plan.index[id].TileCount = uint32(len(bank) / unit)
		}
	}
	if len(bank)%64 != 0 {
		bank = append(bank, make([]byte, 64-len(bank)%64)...)
	}
	plan.words = make([]uint32, len(bank)/4)
	for i := range plan.words {
		plan.words[i] = binary.LittleEndian.Uint32(bank[i*4:])
	}
	return plan
}

func mode0ResidentPosesForChunk(chunk *luaparser.Chunk, opts Options) *mode0ResidentPosePlan {
	for _, stmt := range chunk.Body {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok && functionKey(fn) == "_draw" {
			ops, _, err := mode0WalkDraw(chunk, fn, newMode0SpriteIDAnalysis(chunk), opts)
			if err == nil {
				return planMode0ResidentPoses(chunk, ops, opts)
			}
		}
	}
	return nil
}
