//go:build mtile_tilecanvas && !mtile_mode0

package runtime

import (
	"bytes"
	"fmt"
	"testing"
	"unsafe"
)

// Keep this byte-based oracle independent of the word packing fast path.
func referenceTileCanvas(src []byte, pal *[256]byte, fourBPP bool) []byte {
	n := FramebufferSize
	if fourBPP {
		n /= 2
	}
	dst := make([]byte, n)
	for y := 0; y < 160; y++ {
		for x := 0; x < 240; x++ {
			tile := (y/8)*30 + x/8
			pixel := (y%8)*8 + x%8
			v := src[y*240+x]
			if fourBPP {
				dst[tile*32+pixel/2] |= (pal[v] & 15) << uint(4*(pixel%2))
			} else {
				dst[tile*64+pixel] = v
			}
		}
	}
	return dst
}

func tileCanvasAlignedBytes(n, offset int) []byte {
	words := make([]uint32, (n+offset+3)/4)
	return unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*4)[offset : offset+n]
}

func assertTileCanvasDirty(t *testing.T, bits [19]uint32, tiles ...int) {
	t.Helper()
	var want [19]uint32
	for _, tile := range tiles {
		want[tile/32] |= 1 << uint(tile%32)
	}
	if bits != want {
		t.Fatalf("dirty bits = %08x, want %08x", bits, want)
	}
}

func TestTileCanvasIndexedLayoutAndAlignment(t *testing.T) {
	for _, fourBPP := range []bool{false, true} {
		for sourceAlignment := 0; sourceAlignment < 4; sourceAlignment++ {
			for targetAlignment := 0; targetAlignment < 4; targetAlignment++ {
				t.Run(fmt.Sprintf("4bpp=%t/src=%d/dst=%d", fourBPP, sourceAlignment, targetAlignment), func(t *testing.T) {
					src := tileCanvasAlignedBytes(FramebufferSize+8, sourceAlignment)
					for i := range src {
						src[i] = byte(i*73 + i/240*19)
					}
					originalSource := bytes.Clone(src)
					var pal [256]byte
					for i := range pal {
						pal[i] = byte(i*11 + 7) // Includes high bits that must be discarded.
					}
					want := referenceTileCanvas(src, &pal, fourBPP)
					dst := tileCanvasAlignedBytes(len(want)+8, targetAlignment)
					for i := range dst {
						dst[i] = 0xa5
					}
					var bits [19]uint32
					if got := packTileCanvas(src, dst, &bits, &pal, fourBPP); got != 600 {
						t.Fatalf("changed tiles = %d, want 600", got)
					}
					if !bytes.Equal(dst[:len(want)], want) {
						t.Fatal("tile-major pixels differ from independent byte oracle")
					}
					if !bytes.Equal(dst[len(want):], bytes.Repeat([]byte{0xa5}, 8)) {
						t.Fatal("wrote beyond tiled backing")
					}
					if !bytes.Equal(src, originalSource) {
						t.Fatal("modified source framebuffer")
					}
					for i, word := range bits {
						expect := ^uint32(0)
						if i == 18 {
							expect = (1 << 24) - 1
						}
						if word != expect {
							t.Fatalf("dirty word %d = %08x, want %08x", i, word, expect)
						}
					}
					pending := bits
					if got := packTileCanvas(src, dst, &bits, &pal, fourBPP); got != 0 || bits != pending {
						t.Fatalf("unchanged frame: changed=%d, pending flags were altered=%t", got, bits != pending)
					}
					bits = [19]uint32{}
					if got := packTileCanvas(src, dst, &bits, &pal, fourBPP); got != 0 || bits != [19]uint32{} {
						t.Fatal("unchanged frame added dirty flags")
					}
				})
			}
		}
	}
}

func TestTileCanvasMotionErasureAndDirtyAccumulation(t *testing.T) {
	for _, fourBPP := range []bool{false, true} {
		t.Run(fmt.Sprintf("4bpp=%t", fourBPP), func(t *testing.T) {
			src := tileCanvasAlignedBytes(FramebufferSize, 0)
			dst := tileCanvasAlignedBytes(FramebufferSize, 0)
			var pal [256]byte
			pal[206] = 1
			var bits [19]uint32
			src[0] = 206
			if got := packTileCanvas(src, dst, &bits, &pal, fourBPP); got != 1 {
				t.Fatalf("first pixel changed %d tiles", got)
			}
			assertTileCanvasDirty(t, bits, 0)
			bits = [19]uint32{}
			src[0] = 0
			src[159*240+239] = 206
			if got := packTileCanvas(src, dst, &bits, &pal, fourBPP); got != 2 {
				t.Fatalf("moving pixel changed %d tiles, want old and new tiles", got)
			}
			assertTileCanvasDirty(t, bits, 0, 599)
			want := referenceTileCanvas(src, &pal, fourBPP)
			if !bytes.Equal(dst[:len(want)], want) {
				t.Fatal("old pixel was not erased or final screen pixel was not packed")
			}
			// Pending old/new flags survive later changes, including a contiguous
			// run crossing the uint32 bitset word boundary.
			for _, tile := range []int{30, 31, 32, 33} {
				src[(tile/30)*8*240+(tile%30)*8] = 206
			}
			if got := packTileCanvas(src, dst, &bits, &pal, fourBPP); got != 4 {
				t.Fatalf("new run changed %d tiles, want 4", got)
			}
			assertTileCanvasDirty(t, bits, 0, 30, 31, 32, 33, 599)
			src[159*240+239] = 0
			if got := packTileCanvas(src, dst, &bits, &pal, fourBPP); got != 1 {
				t.Fatalf("already-pending tile change counted %d, want 1", got)
			}
			assertTileCanvasDirty(t, bits, 0, 30, 31, 32, 33, 599)
		})
	}
}

func TestTileCanvasPaletteChangesAndSolidRows(t *testing.T) {
	src := tileCanvasAlignedBytes(FramebufferSize, 0)
	dst := tileCanvasAlignedBytes(FramebufferSize/2, 0)
	var pal [256]byte
	pal[0], pal[206], pal[207] = 3, 1, 1
	// Exercise zero, uniform nonzero, and mixed source rows.
	for x := 0; x < 16; x++ {
		src[x] = 206
	}
	src[8] = 207
	var bits [19]uint32
	if got := packTileCanvas(src, dst, &bits, &pal, true); got != 600 {
		t.Fatalf("nonzero mapping for source zero changed %d tiles", got)
	}
	if want := referenceTileCanvas(src, &pal, true); !bytes.Equal(dst, want) {
		t.Fatal("solid/mixed mapped rows disagree with byte oracle")
	}
	if dst[0] != 0x11 || dst[4] != 0x33 {
		t.Fatalf("solid cloud/zero rows = %02x/%02x, want 11/33", dst[0], dst[4])
	}
	bits = [19]uint32{}
	src[0] = 207
	if got := packTileCanvas(src, dst, &bits, &pal, true); got != 0 {
		t.Fatalf("equivalent palette indices changed %d tiles", got)
	}
	pal[206] = 2
	if got := packTileCanvas(src, dst, &bits, &pal, true); got != 2 {
		t.Fatalf("palette remapping changed %d tiles, want 2", got)
	}
	assertTileCanvasDirty(t, bits, 0, 1)
	if want := referenceTileCanvas(src, &pal, true); !bytes.Equal(dst, want) {
		t.Fatal("palette edit did not update previously packed pixels")
	}
	// Even x is the low nibble; odd x is the high nibble.
	if dst[0] != 0x21 {
		t.Fatalf("first pixel pair = %02x, want 21", dst[0])
	}
}

func TestTileCanvasPackingAllocatesNothing(t *testing.T) {
	src := tileCanvasAlignedBytes(FramebufferSize, 0)
	dst := tileCanvasAlignedBytes(FramebufferSize, 0)
	var bits [19]uint32
	var pal [256]byte
	pal[206] = 1
	for _, fourBPP := range []bool{false, true} {
		allocs := testing.AllocsPerRun(10, func() {
			src[0] ^= 206
			packTileCanvas(src, dst, &bits, &pal, fourBPP)
		})
		if allocs != 0 {
			t.Fatalf("4bpp=%t: packing allocated %g objects", fourBPP, allocs)
		}
	}
	// No palette is required by the 8bpp path.
	packTileCanvas(src, dst, &bits, nil, false)
}

func TestTileCanvasRowsDenseAndNilMatchFull(t *testing.T) {
	src := tileCanvasAlignedBytes(FramebufferSize, 0)
	for i := range src {
		src[i] = byte(i*37 + i/240*11)
	}
	var pal [256]byte
	for i := range pal {
		pal[i] = byte(i*7 + 3)
	}
	var allRows [20]uint32
	for i := range allRows {
		allRows[i] = ^uint32(0) // Upper two bits must never address extra tiles.
	}
	for _, fourBPP := range []bool{false, true} {
		for _, rows := range []*[20]uint32{nil, &allRows} {
			dst := tileCanvasAlignedBytes(FramebufferSize, 0)
			var bits [19]uint32
			if changed := packTileCanvasRows(src, dst, &bits, &pal, fourBPP, rows); changed != 600 {
				t.Fatalf("4bpp=%t, nil=%t: changed %d tiles", fourBPP, rows == nil, changed)
			}
			want := referenceTileCanvas(src, &pal, fourBPP)
			if !bytes.Equal(dst[:len(want)], want) {
				t.Fatalf("4bpp=%t, nil=%t: dense pixels differ", fourBPP, rows == nil)
			}
			if bits[18] != (1<<24)-1 {
				t.Fatalf("out-of-range tile flags = %08x", bits[18])
			}
			bits = [19]uint32{}
			if changed := packTileCanvasRows(src, dst, &bits, &pal, fourBPP, rows); changed != 0 || bits != [19]uint32{} {
				t.Fatal("unchanged dense frame became dirty")
			}
		}
	}
}

func TestTileCanvasRowsMovementUnionAndUnselectedStorage(t *testing.T) {
	for _, fourBPP := range []bool{false, true} {
		for _, alignment := range []int{0, 1} {
			src := tileCanvasAlignedBytes(FramebufferSize, alignment)
			dst := tileCanvasAlignedBytes(FramebufferSize, alignment)
			var pal [256]byte
			pal[206] = 1
			var bits [19]uint32
			var oldRows, newRows [20]uint32
			oldRows[0] = 1
			src[0] = 206
			if changed := packTileCanvasRows(src, dst, &bits, &pal, fourBPP, &oldRows); changed != 1 {
				t.Fatalf("initial selected pixel changed %d tiles", changed)
			}
			assertTileCanvasDirty(t, bits, 0)
			bits = [19]uint32{}
			src[0], src[239+159*240] = 0, 206
			newRows[19] = 1 << 29
			var union [20]uint32
			for i := range union {
				union[i] = oldRows[i] | newRows[i]
			}
			originalUnion := union
			if changed := packTileCanvasRows(src, dst, &bits, &pal, fourBPP, &union); changed != 2 {
				t.Fatalf("4bpp=%t, align=%d: movement changed %d tiles", fourBPP, alignment, changed)
			}
			if union != originalUnion {
				t.Fatal("packing modified candidate mask")
			}
			assertTileCanvasDirty(t, bits, 0, 599)
			want := referenceTileCanvas(src, &pal, fourBPP)
			if !bytes.Equal(dst[:len(want)], want) {
				t.Fatal("old|new candidate union failed to erase or draw a moved pixel")
			}
			// An empty mask must not inspect new source differences or overwrite
			// any backing tiles, and pending uploads must survive.
			src[42] = 206
			before, pending := bytes.Clone(dst), bits
			var empty [20]uint32
			empty[0] = 3 << 30 // Out-of-grid flags also select no tiles.
			if changed := packTileCanvasRows(src, dst, &bits, &pal, fourBPP, &empty); changed != 0 || bits != pending || !bytes.Equal(dst, before) {
				t.Fatal("empty candidate mask altered backing or dirty flags")
			}
			// The caller later includes that source tile; the previous pending
			// movement flags are still retained.
			empty[0] = 1 << 5
			if changed := packTileCanvasRows(src, dst, &bits, &pal, fourBPP, &empty); changed != 1 {
				t.Fatalf("newly selected tile changed %d tiles", changed)
			}
			assertTileCanvasDirty(t, bits, 0, 5, 599)
		}
	}
}
