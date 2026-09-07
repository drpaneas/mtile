//go:build mtile_tilecanvas && !mtile_mode0

package runtime

import (
	"math/bits"
	"unsafe"
)

const (
	objectCanvasColumns       = ScreenWidth / 16
	objectCanvasRows          = ScreenHeight / 16
	objectCanvasObjectLimit   = 96
	objectCanvasTileBytes     = objectCanvasObjectLimit * 256
	objectCanvasCoverageWords = FramebufferSize / 32
	objectCanvasOAMBytes      = objectCanvasObjectLimit * 8
	objectCanvasDirtyWords    = (objectCanvasObjectLimit + 31) / 32
	objectCanvasRunLimit      = (objectCanvasObjectLimit + 1) / 2
)

type objectCanvasPackStatus uint8

const (
	objectCanvasPackOK objectCanvasPackStatus = iota
	objectCanvasTooManyObjects
	objectCanvasTooManyColors
)

type objectCanvasPackResult struct {
	status  objectCanvasPackStatus
	objects int
	// Logical color zero is opaque at this physical OBJ palette index. Zero
	// here means logical zero was absent; hardware index zero is always a hole.
	opaqueZero byte
	changed    [objectCanvasDirtyWords]uint32 // One bit per densely packed 256-byte object block.
}

// packObjectCanvas builds a sparse grid of nonoverlapping 16x16 8bpp hardware
// OBJs. It does not reserve a particular logical color or know a game's art.
// Pixels covered by coverage are opaque, including color zero. Nil coverage
// instead interprets framebuffer index zero as transparent. A candidates mask
// may conservatively restrict the 15x10 block scan; nil visits all blocks.
//
// Preflight rejects hardware capacity or palette exhaustion before changing
// either destination. The caller can retain the prior frame or choose another
// renderer. A successful result identifies changed physical blocks; unused
// tile slots retain their contents, and inactive OAM entries are disabled.
// All buffers must have the declared extents and must not overlap. No heap
// allocation occurs. Palette RAM must map opaqueZero back to logical zero;
// all other nonzero physical indices retain their matching logical indices.
func packObjectCanvas(src []byte, coverage *[objectCanvasCoverageWords]uint32, candidates *[objectCanvasRows]uint16,
	tiles []byte, oam []byte) objectCanvasPackResult {
	src = src[:FramebufferSize]
	tiles = tiles[:objectCanvasTileBytes]
	oam = oam[:objectCanvasOAMBytes]
	var result objectCanvasPackResult
	var active [objectCanvasRows]uint16
	var colors [256]byte
	result.objects = objectCanvasScan(src, coverage, candidates, &active, &colors)
	if result.objects > objectCanvasObjectLimit {
		result.status = objectCanvasTooManyObjects
		return result
	}
	if colors[0] != 0 {
		// Find an unused physical index for opaque zero; no source color is reserved.
		for color := 255; color > 0; color-- {
			if colors[color] == 0 {
				result.opaqueZero = byte(color)
				break
			}
		}
		if result.opaqueZero == 0 {
			result.status = objectCanvasTooManyColors
			return result
		}
	}

	slot := 0
	for by, row := range active {
		for row != 0 {
			bx := bits.TrailingZeros16(row)
			row &= row - 1
			changed := objectCanvasPackBlock(src, coverage, tiles, by*16*ScreenWidth+bx*16, slot*256, result.opaqueZero)
			if changed {
				result.changed[slot>>5] |= 1 << uint(slot&31)
			}
			offset := slot * 8
			objectCanvasOAM16(oam, offset, uint16(by*16)|0x2000)
			objectCanvasOAM16(oam, offset+2, uint16(bx*16)|0x4000)
			objectCanvasOAM16(oam, offset+4, uint16(slot*8))
			objectCanvasOAM16(oam, offset+6, 0)
			slot++
		}
	}
	for slot < objectCanvasObjectLimit {
		objectCanvasOAM16(oam, slot*8, 0x0200)
		slot++
	}
	return result
}

// A byte lookup avoids a software count-trailing-zeros call per covered pixel
// on ARM7TDMI. This scan also preflights capacity before changing tile/OAM data.
//
//go:noinline
//go:section .iwram
func objectCanvasScan(src []byte, coverage *[objectCanvasCoverageWords]uint32, candidates *[objectCanvasRows]uint16, active *[objectCanvasRows]uint16, colors *[256]byte) int {
	objects := 0
	for by := 0; by < objectCanvasRows; by++ {
		row := uint16(1<<objectCanvasColumns) - 1
		if candidates != nil {
			row &= candidates[by]
		}
		for bx := 0; bx < objectCanvasColumns; bx++ {
			if row&(1<<uint(bx)) == 0 {
				continue
			}
			any := false
			for py := 0; py < 16; py++ {
				offset := (by*16+py)*ScreenWidth + bx*16
				mask := objectCanvasRowCoverage(src, coverage, offset)
				if mask == 0 {
					continue
				}
				any = true
				for mask != 0 {
					if mask&1 != 0 {
						colors[src[offset]] = 1
					}
					offset++
					mask >>= 1
				}
			}
			if any {
				active[by] |= 1 << uint(bx)
				objects++
			}
		}
	}
	return objects
}

func objectCanvasRowCoverage(src []byte, coverage *[objectCanvasCoverageWords]uint32, offset int) uint16 {
	if coverage != nil {
		return uint16(coverage[offset>>5] >> uint(offset&31))
	}
	var mask uint16
	for x := 0; x < 16; x++ {
		if src[offset+x] != 0 {
			mask |= 1 << uint(x)
		}
	}
	return mask
}

func objectCanvasOAM16(data []byte, offset int, value uint16) {
	data[offset], data[offset+1] = byte(value), byte(value>>8)
}

// Keep the conversion loop in IWRAM. Each word covers four adjacent pixels
// within one 8x8 tile; fully covered nonzero words can be copied directly.
var objectCanvasByteMasks = [16]uint32{
	0, 0xff, 0xff00, 0xffff, 0xff0000, 0xff00ff, 0xffff00, 0xffffff,
	0xff000000, 0xff0000ff, 0xff00ff00, 0xff00ffff, 0xffff0000, 0xffff00ff, 0xffffff00, 0xffffffff,
}

//go:noinline
//go:section .iwram
func objectCanvasPackBlock(src []byte, coverage *[objectCanvasCoverageWords]uint32, tiles []byte, sourceBase, targetBase int, zero byte) bool {
	sourcePtr, targetPtr := unsafe.Pointer(&src[0]), unsafe.Pointer(&tiles[0])
	if (uintptr(sourcePtr)|uintptr(targetPtr))&3 != 0 {
		return objectCanvasPackBlockUnaligned(src, coverage, tiles, sourceBase, targetBase, zero)
	}
	changed := false
	for py := 0; py < 16; py++ {
		source := sourceBase + py*ScreenWidth
		mask := objectCanvasRowCoverage(src, coverage, source)
		target := targetBase + (py>>3)*128 + (py&7)*8
		for px := 0; px < 16; px += 4 {
			covered := mask & 15
			mask >>= 4
			word := uint32(0)
			if covered != 0 {
				word = *(*uint32)(unsafe.Add(sourcePtr, source+px))
				if zero != 0 && packedHasZeroByte(word) {
					// Each high bit below marks exactly one zero byte (no cross-byte borrow).
					zeros := ^(((word & 0x7f7f7f7f) + 0x7f7f7f7f) | word | 0x7f7f7f7f)
					word |= (zeros >> 7) * uint32(zero)
				}
				word &= objectCanvasByteMasks[covered]
			}
			dst := (*uint32)(unsafe.Add(targetPtr, target+(px>>3)*64+(px&7)))
			if *dst != word {
				*dst = word
				changed = true
			}
		}
	}
	return changed
}

func objectCanvasPackBlockUnaligned(src []byte, coverage *[objectCanvasCoverageWords]uint32, tiles []byte, sourceBase, targetBase int, zero byte) bool {
	changed := false
	aligned := (uintptr(unsafe.Pointer(&src[0]))|uintptr(unsafe.Pointer(&tiles[0])))&3 == 0
	for py := 0; py < 16; py++ {
		source := sourceBase + py*ScreenWidth
		mask := objectCanvasRowCoverage(src, coverage, source)
		for px := 0; px < 16; px += 4 {
			target := targetBase + (py>>3)*128 + (px>>3)*64 + (py&7)*8 + (px & 7)
			covered := (mask >> uint(px)) & 15
			if aligned {
				word := *(*uint32)(unsafe.Pointer(&src[source+px]))
				if covered != 15 || zero != 0 && packedHasZeroByte(word) {
					packed := uint32(0)
					for bit := 0; bit < 4; bit++ {
						if covered&(1<<uint(bit)) == 0 {
							continue
						}
						color := byte(word >> uint(bit*8))
						if color == 0 {
							color = zero
						}
						packed |= uint32(color) << uint(bit*8)
					}
					word = packed
				}
				dst := (*uint32)(unsafe.Pointer(&tiles[target]))
				if *dst != word {
					*dst = word
					changed = true
				}
			} else {
				for bit := 0; bit < 4; bit++ {
					color := byte(0)
					if covered&(1<<uint(bit)) != 0 {
						color = src[source+px+bit]
						if color == 0 {
							color = zero
						}
					}
					if tiles[target+bit] != color {
						tiles[target+bit] = color
						changed = true
					}
				}
			}
		}
	}
	return changed
}

// Consecutive dirty physical OBJ slots share one DMA. A clean 256-byte gap
// costs more than another setup, so gaps are never merged. Runs encode first
// slot in seven low bits and their nonzero length in the remaining bits.
func prepareObjectCanvasRuns(dirty *[objectCanvasDirtyWords]uint32, runs *[objectCanvasRunLimit]uint16) (count int, cost uint32, bytes uint32) {
	for slot := 0; slot < objectCanvasObjectLimit; {
		if dirty[slot>>5]&(1<<uint(slot&31)) == 0 {
			slot++
			continue
		}
		first := slot
		for slot < objectCanvasObjectLimit && dirty[slot>>5]&(1<<uint(slot&31)) != 0 {
			slot++
		}
		length := slot - first
		runs[count] = uint16(first) | uint16(length)<<7
		count++
		size := uint32(length * 256)
		bytes += size
		cost += 2*size + 192
	}
	return
}
