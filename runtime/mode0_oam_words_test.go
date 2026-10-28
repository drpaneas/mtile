//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"bytes"
	"github.com/drpaneas/mtile/runtime/hw"
	"testing"
	"unsafe"
)

// Frozen general-cell loop, independent of the new single-cell path.
func (c *Context) referenceMode0RenderSprite(id, x, y, w, h int, flipX, flipY bool) {
	if c == nil || c.cart == nil || id < 0 || w <= 0 || h <= 0 {
		return
	}
	cols := c.cart.SpriteCols
	if cols <= 0 {
		return
	}
	if id == 0 && w == 1 && h == 1 && !c.mode0ObjectZeroVisible {
		return
	}
	if w == 1 && h == 1 {
		// A single cell cannot cross a source row. Validated sheet dimensions
		// fit in an int, so its linear bound needs only one multiplication.
		// Avoid the software id/cols division on every ordinary 8x8 OBJ.
		if id >= cols*c.cart.SpriteRows {
			return
		}
	} else {
		baseCellX, baseCellY := id%cols, id/cols
		if baseCellX+w > cols || baseCellY+h > c.cart.SpriteRows {
			return
		}
	}
	baseX, baseY := x-c.camX, y-c.camY
	packedCells := 0
	if w == h && (w == 2 || w == 4) {
		packedCells = w
	}
	if packedCells != 0 && c.mode0ObjectCount < mode0ObjectLimit {
		if tile, ok := c.mode0PackedObjectTile(id, packedCells); ok {
			// Populate the deterministic packing cache even for an offscreen
			// first use, then suppress the OAM entry before coordinate masking
			// can alias it back into the viewport.
			if !c.mode0ObjectIntersectsClip(baseX, baseY, w*8, h*8) {
				return
			}
			slot := mode0ObjectLimit - 1 - c.mode0ObjectCount
			offset := slot * 8
			attr0 := uint16(baseY&0xff) | 0x2000 // 8bpp, square
			attr1 := uint16(baseX&0x1ff) | uint16(packedCells/2)<<14
			if flipX {
				attr1 |= 1 << 12
			}
			if flipY {
				attr1 |= 1 << 13
			}
			mode0PutOAM16(&c.mode0OAM, offset, attr0)
			mode0PutOAM16(&c.mode0OAM, offset+2, attr1)
			mode0PutOAM16(&c.mode0OAM, offset+4, uint16(tile*2)|c.mode0ObjectPriorityBits())
			mode0PutOAM16(&c.mode0OAM, offset+6, 0)
			c.mode0ObjectCount++
			c.mode0OAMDirty = true
			return
		}
	}
	if !c.mode0ObjectIntersectsClip(baseX, baseY, w*8, h*8) {
		return
	}
	if c.mode0ObjectCount+w*h > mode0ObjectLimit {
		return
	}
	wroteObject := false
	for drawY := 0; drawY < h; drawY++ {
		for drawX := 0; drawX < w; drawX++ {
			objectX, objectY := baseX+drawX*8, baseY+drawY*8
			if !c.mode0ObjectIntersectsClip(objectX, objectY, 8, 8) {
				continue
			}
			sourceX, sourceY := drawX, drawY
			if flipX {
				sourceX = w - 1 - drawX
			}
			if flipY {
				sourceY = h - 1 - drawY
			}
			sourceID := id + sourceY*cols + sourceX
			if sourceID < 0 || sourceID >= c.cart.SpriteN || sourceID >= mode0ObjectTileLimit {
				continue
			}
			if sourceID == 0 && !c.mode0ObjectZeroVisible {
				continue
			}
			slot := mode0ObjectLimit - 1 - c.mode0ObjectCount
			offset := slot * 8
			attr0 := uint16(objectY&0xff) | 0x2000 // 8bpp, square 8x8
			attr1 := uint16(objectX & 0x1ff)
			if flipX {
				attr1 |= 1 << 12
			}
			if flipY {
				attr1 |= 1 << 13
			}
			attr2 := uint16(sourceID*2) | c.mode0ObjectPriorityBits() // 8bpp uses two 32-byte tile-number units
			mode0PutOAM16(&c.mode0OAM, offset, attr0)
			mode0PutOAM16(&c.mode0OAM, offset+2, attr1)
			mode0PutOAM16(&c.mode0OAM, offset+4, attr2)
			mode0PutOAM16(&c.mode0OAM, offset+6, 0)
			c.mode0ObjectCount++
			wroteObject = true
		}
	}
	if wroteObject {
		c.mode0OAMDirty = true
	}
}

func TestMode0PackedOAMEntryMatchesBytesAtEveryAlignment(t *testing.T) {
	for alignment := 0; alignment < 4; alignment++ {
		for slot := -1; slot <= 128; slot++ {
			storage := bytes.Repeat([]byte{0xa5}, hw.OAMBytes+8)
			got := (*[hw.OAMBytes]byte)(unsafe.Pointer(&storage[4+alignment]))
			want := *got
			if slot >= 0 && slot < mode0ObjectLimit {
				at := slot * 8
				mode0PutOAM16(&want, at, 0xabcd)
				mode0PutOAM16(&want, at+2, 0xfedc)
				mode0PutOAM16(&want, at+4, 0x1234)
				mode0PutOAM16(&want, at+6, 0)
			}
			mode0PutOAMEntry(got, slot, 0xabcd, 0xfedc, 0x1234)
			if *got != want {
				t.Fatalf("alignment=%d slot=%d: bytes differ", alignment, slot)
			}
			for i, v := range storage {
				if (i < 4+alignment || i >= 4+alignment+hw.OAMBytes) && v != 0xa5 {
					t.Fatal("canary overwritten")
				}
			}
		}
	}
}
func TestMode0WordCompactionPreservesEveryCount(t *testing.T) {
	for count := -1; count <= 129; count++ {
		var c Context
		for i := range c.mode0OAM {
			c.mode0OAM[i] = byte(i*37 + i/8)
		}
		c.mode0ObjectCount = count
		want := c.mode0OAM
		n := max(0, min(count, mode0ObjectLimit))
		copy(want[:n*8], want[(mode0ObjectLimit-n)*8:mode0ObjectLimit*8])
		for i := max(n, mode0ObjectLimit-n); i < mode0ObjectLimit; i++ {
			mode0PutOAM16(&want, i*8, mode0ObjectDisable)
		}
		c.compactMode0OAM()
		if c.mode0OAM != want {
			t.Fatalf("count=%d: overlap/disable changed", count)
		}
	}
}
func TestMode0SingleCellMatchesGeneralLoop(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 30*8, 20*8, 1, 1, make([]byte, 30*20*64), []uint16{0}, make([]byte, 30*20))
	c.Cls(0)
	for _, clip := range [][4]int{{0, 0, 240, 160}, {17, 23, 61, 49}, {0, 0, 0, 0}} {
		for _, pos := range [][2]int{{-512, -256}, {-8, -8}, {-1, -1}, {0, 0}, {18, 24}, {239, 159}, {240, 160}, {511, 255}} {
			for _, id := range []int{-1, 0, 1, 29, 30, 447, 479, 511, 512, 599, 600} {
				for _, count := range []int{0, 63, 64, 127, 128} {
					for bits := 0; bits < 8; bits++ {
						c.clipX0, c.clipY0, c.clipX1, c.clipY1 = clip[0], clip[1], clip[2], clip[3]
						c.camX, c.camY = 3, -4
						c.mode0MapCalls = bits % 5
						c.mode0ObjectZeroVisible = bits&4 != 0
						c.mode0ObjectCount = count
						c.mode0OAMDirty = false
						c.mode0OAM = [hw.OAMBytes]byte{}
						want := *c
						c.renderSprite(id, pos[0], pos[1], 1, 1, bits&1 != 0, bits&2 != 0)
						want.referenceMode0RenderSprite(id, pos[0], pos[1], 1, 1, bits&1 != 0, bits&2 != 0)
						if c.mode0OAM != want.mode0OAM || c.mode0ObjectCount != want.mode0ObjectCount || c.mode0OAMDirty != want.mode0OAMDirty {
							t.Fatalf("clip%v pos%v id%d count%d bits%d: object semantics changed", clip, pos, id, count, bits)
						}
					}
				}
			}
		}
	}
}

func BenchmarkMode0OAMSingleCell(b *testing.B) {
	for _, original := range []bool{true, false} {
		name := "word-single-cell"
		if original {
			name = "general-cell-loop"
		}
		b.Run(name, func(b *testing.B) {
			var c Context
			c.cart = &AssetMemory{SpriteCols: 30, SpriteRows: 20, SpriteN: 600}
			c.clipX1, c.clipY1 = 240, 160
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				c.mode0ObjectCount = 0
				if original {
					c.referenceMode0RenderSprite(3, 90, 70, 1, 1, true, false)
				} else {
					c.renderSprite(3, 90, 70, 1, 1, true, false)
				}
			}
		})
	}
}
