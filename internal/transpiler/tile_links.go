package transpiler

import (
	"fmt"
	"strings"

	runtimeapi "github.com/drpaneas/mtile/runtime"
)

// tileLinkTable returns the per-tile lookup table: link index plus one, or
// zero for unlinked tiles.
func tileLinkTable(links []runtimeapi.TileLink, spriteN int) []uint8 {
	if len(links) == 0 {
		return nil
	}
	table := make([]uint8, spriteN)
	for i, link := range links {
		if int(link.Tile) < len(table) {
			table[link.Tile] = uint8(i + 1)
		}
	}
	return table
}

func (g *generator) tileLinkOf() []uint8 { return tileLinkTable(g.opts.TileLinks, g.spriteN) }

// tileLinkInstances lists every linked cell of each drawable logical plane in
// layer, row, column order, with the row index the per-row strategy uses.
func (g *generator) tileLinkInstances() ([]runtimeapi.TileLinkInstance, []uint32) {
	table := g.tileLinkOf()
	if table == nil {
		return nil, nil
	}
	w, h, layers := g.opts.MapW, g.opts.MapH, max(1, g.opts.MapLayers)
	planeCells := w * h
	var instances []runtimeapi.TileLinkInstance
	rows := make([]uint32, 0, layers*h+1)
	for bg := 0; bg < layers; bg++ {
		present := g.opts.MapPlaneMask == 0 && bg == 0 || g.opts.MapPlaneMask&(1<<uint(bg)) != 0
		for y := 0; y < h; y++ {
			rows = append(rows, uint32(len(instances)))
			if !present {
				continue
			}
			for x := 0; x < w; x++ {
				index := bg*planeCells + y*w + x
				if index >= len(g.opts.Map) {
					continue
				}
				id := g.opts.Map[index]
				if int(id) < len(table) && table[id] != 0 {
					instances = append(instances, runtimeapi.TileLinkInstance{X: uint16(x), Y: uint16(y), Layer: uint8(bg)})
				}
			}
		}
	}
	rows = append(rows, uint32(len(instances)))
	return instances, rows
}

// tileLinkReach is how far any linked pose, or any frame of a linked clip,
// extends beyond the 8x8 cell it is drawn from.
func tileLinkReach(opts Options) runtimeapi.TileLinkReach {
	left, top, right, bottom := 0, 0, 0, 0
	include := func(poseID int) {
		if poseID < 0 || poseID >= len(opts.PoseIndex) {
			return
		}
		r := opts.PoseIndex[poseID]
		end := int(r.Start) + int(r.Count)
		if end > len(opts.PoseParts) {
			return
		}
		for _, part := range opts.PoseParts[r.Start:end] {
			left = max(left, -int(part.X))
			top = max(top, -int(part.Y))
			right = max(right, int(part.X))
			bottom = max(bottom, int(part.Y))
		}
	}
	for _, link := range opts.TileLinks {
		if !link.Clip {
			include(int(link.ID))
			continue
		}
		if int(link.ID) >= len(opts.AnimationClips) {
			continue
		}
		clip := opts.AnimationClips[link.ID]
		for _, frame := range opts.AnimationFrames[clip.Start : clip.Start+clip.Count] {
			include(int(frame))
		}
	}
	return runtimeapi.TileLinkReach{Left: int16(left), Top: int16(top), Right: int16(right), Bottom: int16(bottom)}
}

func (g *generator) renderTileLinkTables(b *sourceBuilder) {
	table := g.tileLinkOf()
	if table == nil {
		return
	}
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var tileLinkOfROM = [ %d ]uint8{%s}", len(table), byteLiteral(table, len(table))))
	var links strings.Builder
	for _, link := range g.opts.TileLinks {
		fmt.Fprintf(&links, "{Tile:%d,Clip:%t,ID:%d},", link.Tile, link.Clip, link.ID)
	}
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var tileLinksROM = [ %d ]rt.TileLink{%s}", len(g.opts.TileLinks), links.String()))
	instances, rows := g.tileLinkInstances()
	var lit strings.Builder
	for _, inst := range instances {
		fmt.Fprintf(&lit, "{X:%d,Y:%d,Layer:%d},", inst.X, inst.Y, inst.Layer)
	}
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var tileLinkInstancesROM = [ %d ]rt.TileLinkInstance{%s}", len(instances), lit.String()))
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var tileLinkRowsROM = [ %d ]uint32{%s}", len(rows), u32Literal(rows, len(rows))))
}

func (g *generator) renderTileLinkBindings(b *sourceBuilder) {
	if len(g.opts.TileLinks) == 0 {
		return
	}
	reach := tileLinkReach(g.opts)
	b.line("\tcartData.TileLinkOf, cartData.TileLinks = tileLinkOfROM[:], tileLinksROM[:]")
	b.line("\tcartData.TileLinkInstances, cartData.TileLinkRows = tileLinkInstancesROM[:], tileLinkRowsROM[:]")
	b.line(fmt.Sprintf("\tcartData.TileLinkReach = rt.TileLinkReach{Left: %d, Top: %d, Right: %d, Bottom: %d}", reach.Left, reach.Top, reach.Right, reach.Bottom))
}

// tileLinksUseClips reports whether map() can play a clip, which needs the
// update clock that anim() otherwise enables.
func tileLinksUseClips(links []runtimeapi.TileLink) bool {
	for _, link := range links {
		if link.Clip {
			return true
		}
	}
	return false
}
