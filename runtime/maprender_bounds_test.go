//go:build !mtile_mode0

package runtime

import (
	"fmt"
	"math/rand"
	"testing"
)

// Compare the map walk against independent sprite calls, including fractional
// tile intersections, negative source cells, and requests past the map edges.
func TestMapVisibleBoundsMatchSprites(t *testing.T) {
	const mapW, mapH = 64, 64
	sheet := make([]byte, 32*8)
	for i := range sheet {
		sheet[i] = byte(i % 17)
	}
	mapData := make([]uint16, mapW*mapH)
	for i := range mapData {
		mapData[i] = uint16(i % 4)
	}
	img := &AssetImage{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4,
		MapW: mapW, MapH: mapH, Sheet: sheet, Map: mapData, Flags: []byte{0, 1, 3, 2},
	}
	newContext := func() *Context {
		ram := &AssetMemory{
			SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4,
			MapW: mapW, MapH: mapH, Sheet: make([]byte, len(sheet)),
			Map: make([]uint16, len(mapData)), Flags: make([]byte, 4),
		}
		c := &Context{}
		if !c.Init(img, ram) {
			t.Fatal("Init returned false")
		}
		return c
	}
	mapped, sprites := newContext(), newContext()
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 256; trial++ {
		t.Run(fmt.Sprint(trial), func(t *testing.T) {
			cellX, cellY := rng.Intn(75)-5, rng.Intn(75)-5
			screenX, screenY := rng.Intn(65)-32, rng.Intn(65)-32
			camX, camY := rng.Intn(600)-32, rng.Intn(600)-32
			clipX, clipY := rng.Intn(260)-10, rng.Intn(180)-10
			clipW, clipH := rng.Intn(250), rng.Intn(170)
			cellsW, cellsH := rng.Intn(80), rng.Intn(80)
			if trial%4 == 0 {
				cellsW, cellsH = 0, 0
			}
			layer := trial % 4
			for _, c := range []*Context{mapped, sprites} {
				c.Cls(200)
				c.Camera(camX, camY)
				c.Clip(clipX, clipY, clipW, clipH)
				c.Pal(7, 23, 0)
				c.PaltColor(0, trial%2 == 0)
			}
			mapped.Map(cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
			// Default extents reach the far clip edges; drawing every remaining
			// source cell has the same pixels and gives an independent oracle.
			if cellsW <= 0 {
				cellsW = mapW - cellX
			}
			if cellsH <= 0 {
				cellsH = mapH - cellY
			}
			for y := cellY; y < cellY+cellsH; y++ {
				for x := cellX; x < cellX+cellsW; x++ {
					if x < 0 || x >= mapW || y < 0 || y >= mapH {
						continue
					}
					id := int(mapData[y*mapW+x])
					if id != 0 && img.Flags[id]&byte(layer) == byte(layer) {
						sprites.Spr(id, screenX+x*8, screenY+y*8, 1, 1, false, false)
					}
				}
			}
			assertFramesMatch(t, mapped, sprites, "visible map bounds")
		})
	}
}

func BenchmarkMapClippedExtent(b *testing.B) {
	for _, tc := range []struct {
		name       string
		camX, camY int
		cells      int
	}{
		{"OriginDefault", 0, 0, 0},
		{"FarCameraDefault", 1603, 1605, 0},
		{"OutsideMapDefault", 32000, 32000, 0},
		{"OversizedExplicit", 0, 0, 32767},
	} {
		b.Run(tc.name, func(b *testing.B) {
			c := newBenchStreamingSparseContext(b)
			c.Camera(tc.camX, tc.camY)
			c.Map(0, 0, 0, 0, tc.cells, tc.cells, 0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.Map(0, 0, 0, 0, tc.cells, tc.cells, 0)
			}
		})
	}
}
