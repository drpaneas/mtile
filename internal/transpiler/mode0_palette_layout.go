package transpiler

import (
	"sort"

	rt "github.com/drpaneas/mtile/runtime"
)

type mode0PaletteLayout struct {
	pixels []byte
	banks  []uint8
}
type mode0PaletteObject struct {
	key    [2]int
	pixels []byte
	colors []byte
	group  int
}

// Allocate stable local indices from canonical artwork. Every variant then
// reuses the same packed bytes, including when two source colors become equal.
func mode0PaletteLayouts(opts Options, reachable []bool) (map[[2]int]mode0PaletteLayout, []uint8) {
	count := opts.PoseVariantBaseCount
	if count <= 0 || count*(len(opts.PoseVariantNames)+1) != len(opts.PoseIndex) || len(reachable) < len(opts.PoseIndex) {
		return nil, nil
	}
	var objects []mode0PaletteObject
	for id, pose := range opts.PoseIndex[:count] {
		if !reachable[id] {
			continue
		}
		start, end := int(pose.Start), int(pose.Start)+int(pose.Count)
		if start < 0 || end > len(opts.PoseParts) {
			return nil, nil
		}
		plans, tiles := rt.BuildPoseOBJPlan(opts.PoseParts[start:end])
		for index, object := range plans {
			pixels := mode0ObjectPixels(opts, object, tiles)
			var seen [256]bool
			var colors []byte
			for _, c := range pixels {
				if c != 0 && !seen[c] {
					seen[c] = true
					colors = append(colors, c)
				}
			}
			if len(colors) > 15 {
				return nil, nil
			}
			objects = append(objects, mode0PaletteObject{key: [2]int{id, index}, pixels: pixels, colors: colors})
		}
	}
	sort.SliceStable(objects, func(i, j int) bool { return len(objects[i].colors) > len(objects[j].colors) })
	var groups [][256]uint8
	var sizes []int
	for i := range objects {
		object := &objects[i]
		group := -1
		for g, slots := range groups {
			missing := 0
			for _, c := range object.colors {
				if slots[c] == 0 {
					missing++
				}
			}
			if sizes[g]+missing <= 15 {
				group = g
				break
			}
		}
		if group < 0 {
			group = len(groups)
			groups = append(groups, [256]uint8{})
			sizes = append(sizes, 0)
		}
		object.group = group
		for _, c := range object.colors {
			if groups[group][c] == 0 {
				sizes[group]++
				groups[group][c] = uint8(sizes[group])
			}
		}
	}
	// Recover the validated simultaneous replacements from matching canonical
	// and expanded source parts. This is a build-time scan, never frame work.
	remaps := make([][256]uint8, len(opts.PoseVariantNames)+1)
	for v := range remaps {
		for c := range remaps[v] {
			remaps[v][c] = uint8(c)
		}
		if v == 0 {
			continue
		}
		for id, pose := range opts.PoseIndex[:count] {
			variant := opts.PoseIndex[v*count+id]
			for j := 0; j < int(pose.Count); j++ {
				source := opts.PoseParts[int(pose.Start)+j].Tile
				target := opts.PoseParts[int(variant.Start)+j].Tile
				for y := 0; y < 8; y++ {
					for x := 0; x < 8; x++ {
						a, ok := mode0ObjectSourcePixel(opts, int(source), x, y)
						if !ok {
							continue
						}
						b, ok := mode0ObjectSourcePixel(opts, int(target), x, y)
						if ok {
							remaps[v][a] = b
						}
					}
				}
			}
		}
	}
	var paletteBytes []uint8
	paletteIDs := map[[16]uint8]uint8{}
	bankIDs := make([][]uint8, len(groups))
	for g, slots := range groups {
		bankIDs[g] = make([]uint8, len(remaps))
		for v, remap := range remaps {
			var palette [16]uint8
			for c, slot := range slots {
				if slot != 0 {
					palette[slot] = remap[c]
				}
			}
			bank, ok := paletteIDs[palette]
			if !ok {
				if len(paletteIDs) == 16 {
					return nil, nil
				}
				bank = uint8(len(paletteIDs) + 1)
				paletteIDs[palette] = bank
				paletteBytes = append(paletteBytes, palette[:]...)
			}
			bankIDs[g][v] = bank
		}
	}
	layouts := map[[2]int]mode0PaletteLayout{}
	for _, object := range objects {
		packed := make([]byte, len(object.pixels)/2)
		slots := groups[object.group]
		for i := range packed {
			packed[i] = slots[object.pixels[i*2]] | slots[object.pixels[i*2+1]]<<4
		}
		layouts[object.key] = mode0PaletteLayout{pixels: packed, banks: bankIDs[object.group]}
	}
	return layouts, paletteBytes
}
func mode0ObjectSourcePixel(opts Options, tile, x, y int) (byte, bool) {
	width, height, sheet := opts.ObjSheetW, opts.ObjSheetH, opts.ObjSheet
	if width < 8 || tile < 0 || tile >= width/8*(height/8) {
		return 0, false
	}
	at := (tile/(width/8)*8+y)*width + tile%(width/8)*8 + x
	if at >= len(sheet) {
		return 0, false
	}
	return sheet[at], true
}
func mode0ObjectPixels(opts Options, object rt.PoseOBJ, tiles []uint16) []byte {
	count := int(object.Width) * int(object.Height) / 64
	pixels := make([]byte, count*64)
	for cell, tile := range tiles[int(object.TileOffset) : int(object.TileOffset)+count] {
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				c, _ := mode0ObjectSourcePixel(opts, int(tile), x, y)
				pixels[cell*64+y*8+x] = c
			}
		}
	}
	return pixels
}
