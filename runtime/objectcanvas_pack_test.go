//go:build mtile_tilecanvas && !mtile_mode0

package runtime

import (
	"bytes"
	"testing"
)

// Decode OAM shapes and 1D tile addresses independently of the packer's grid
// loops. Return logical color and coverage separately so opaque black cannot
// accidentally compare equal to a transparent hole.
func objectCanvasDecodedPixel(tiles, oam []byte, zero byte, x, y int) (byte, bool) {
	for obj := 0; obj < objectCanvasObjectLimit; obj++ {
		off := obj * 8
		a0 := uint16(oam[off]) | uint16(oam[off+1])<<8
		if a0&0x0200 != 0 {
			continue
		}
		a1 := uint16(oam[off+2]) | uint16(oam[off+3])<<8
		a2 := uint16(oam[off+4]) | uint16(oam[off+5])<<8
		px, py := x-int(a1&511), y-int(a0&255)
		if px < 0 || px >= 16 || py < 0 || py >= 16 {
			continue
		}
		if a0&0x2000 == 0 || a1&0xc000 != 0x4000 || a2&0xc00 != 0 {
			panic("unexpected object attributes")
		}
		tile := int(a2&1023)*32 + (py/8*2+px/8)*64
		physical := tiles[tile+(py%8)*8+px%8]
		if physical == 0 {
			continue
		}
		if physical == zero {
			return 0, true
		}
		return physical, true
	}
	return 0, false
}

func objectCanvasSetPixel(src []byte, coverage *[objectCanvasCoverageWords]uint32, x, y int, color byte) {
	offset := y*ScreenWidth + x
	src[offset] = color
	coverage[offset>>5] |= 1 << uint(offset&31)
}

func assertObjectCanvasMatches(t *testing.T, src []byte, coverage *[objectCanvasCoverageWords]uint32, tiles, oam []byte, result objectCanvasPackResult) {
	t.Helper()
	if result.status != objectCanvasPackOK {
		t.Fatalf("pack rejected with status %d", result.status)
	}
	for y := 0; y < ScreenHeight; y++ {
		for x := 0; x < ScreenWidth; x++ {
			offset := y*ScreenWidth + x
			covered := src[offset] != 0
			if coverage != nil {
				covered = coverage[offset>>5]&(1<<uint(offset&31)) != 0
			}
			got, visible := objectCanvasDecodedPixel(tiles, oam, result.opaqueZero, x, y)
			if visible != covered || visible && got != src[offset] {
				t.Fatalf("pixel(%d,%d): (%d,%t) want (%d,%t)", x, y, got, visible, src[offset], covered)
			}
		}
	}
}

func TestObjectCanvasPackingExactColorsHolesAndOpaqueZero(t *testing.T) {
	src := make([]byte, FramebufferSize)
	var coverage [objectCanvasCoverageWords]uint32
	// Exercise every byte value without requiring all 256 opaque colors in the
	// same frame. Color 255 is ordinary artwork, never a hardcoded reservation.
	for omitted := 0; omitted < 256; omitted += 17 {
		clear(src)
		clear(coverage[:])
		for color := 0; color < 256; color++ {
			if color == omitted {
				continue
			}
			x, y := color%32+5, color/32+7
			objectCanvasSetPixel(src, &coverage, x, y, byte(color))
		}
		objectCanvasSetPixel(src, &coverage, 239, 159, byte((omitted+1)%256))
		tiles, oam := make([]byte, objectCanvasTileBytes), make([]byte, objectCanvasOAMBytes)
		result := packObjectCanvas(src, &coverage, nil, tiles, oam)
		assertObjectCanvasMatches(t, src, &coverage, tiles, oam, result)
		if omitted == 0 && result.opaqueZero != 0 {
			t.Fatal("reserved a palette slot without opaque zero")
		}
		if omitted != 0 && result.opaqueZero != byte(omitted) {
			t.Fatalf("zero used index %d, only %d was available", result.opaqueZero, omitted)
		}
	}
}

func TestObjectCanvasNilCoverageAndTileGeometry(t *testing.T) {
	src := make([]byte, FramebufferSize)
	for y := 0; y < ScreenHeight; y++ {
		for x := 0; x < ScreenWidth; x++ {
			if (x/16+y/16)%3 != 0 {
				continue
			}
			if (x+y*3)%7 != 0 {
				src[y*ScreenWidth+x] = byte(1 + (x*3+y*11)%255)
			}
		}
	}
	tiles, oam := make([]byte, objectCanvasTileBytes), make([]byte, objectCanvasOAMBytes)
	result := packObjectCanvas(src, nil, nil, tiles, oam)
	if result.objects != 50 {
		t.Fatalf("objects=%d want50", result.objects)
	}
	assertObjectCanvasMatches(t, src, nil, tiles, oam, result)
}

func TestObjectCanvasRejectsCapacityBeforeDestinationMutation(t *testing.T) {
	for _, colors := range []bool{false, true} {
		src := make([]byte, FramebufferSize)
		var coverage [objectCanvasCoverageWords]uint32
		if colors {
			for color := 0; color < 256; color++ {
				objectCanvasSetPixel(src, &coverage, color%32, color/32, byte(color))
			}
		} else {
			for obj := 0; obj < objectCanvasObjectLimit+1; obj++ {
				objectCanvasSetPixel(src, &coverage, obj%15*16, obj/15*16, 11)
			}
		}
		tiles, oam := bytes.Repeat([]byte{0xa5}, objectCanvasTileBytes), bytes.Repeat([]byte{0x5a}, objectCanvasOAMBytes)
		beforeTiles, beforeOAM := append([]byte(nil), tiles...), append([]byte(nil), oam...)
		result := packObjectCanvas(src, &coverage, nil, tiles, oam)
		want := objectCanvasTooManyObjects
		if colors {
			want = objectCanvasTooManyColors
		}
		if result.status != want {
			t.Fatalf("status=%d want%d", result.status, want)
		}
		if !bytes.Equal(tiles, beforeTiles) || !bytes.Equal(oam, beforeOAM) {
			t.Fatal("rejected frame modified destinations")
		}
	}
}

func TestObjectCanvasDirtySlotsMovesAndDisappearance(t *testing.T) {
	src := make([]byte, FramebufferSize)
	var coverage [objectCanvasCoverageWords]uint32
	tiles, oam := make([]byte, objectCanvasTileBytes), make([]byte, objectCanvasOAMBytes)
	objectCanvasSetPixel(src, &coverage, 17, 33, 0)
	objectCanvasSetPixel(src, &coverage, 33, 33, 255)
	result := packObjectCanvas(src, &coverage, nil, tiles, oam)
	if result.changed[0] != 3 || result.objects != 2 || result.opaqueZero != 254 {
		t.Fatalf("first pack=%+v", result)
	}
	assertObjectCanvasMatches(t, src, &coverage, tiles, oam, result)
	again := packObjectCanvas(src, &coverage, nil, tiles, oam)
	if again.changed != ([objectCanvasDirtyWords]uint32{}) {
		t.Fatalf("identical frame marked dirty: %+v", again.changed)
	}
	// Moving whole blocks with identical contents requires only new OAM, no tile upload.
	clear(src)
	clear(coverage[:])
	objectCanvasSetPixel(src, &coverage, 65, 65, 0)
	objectCanvasSetPixel(src, &coverage, 81, 65, 255)
	moved := packObjectCanvas(src, &coverage, nil, tiles, oam)
	if moved.changed != ([objectCanvasDirtyWords]uint32{}) {
		t.Fatal("whole-block move uploaded unchanged graphics")
	}
	assertObjectCanvasMatches(t, src, &coverage, tiles, oam, moved)
	clear(src)
	clear(coverage[:])
	empty := packObjectCanvas(src, &coverage, nil, tiles, oam)
	if empty.objects != 0 || empty.changed != ([objectCanvasDirtyWords]uint32{}) {
		t.Fatalf("empty frame=%+v", empty)
	}
	assertObjectCanvasMatches(t, src, &coverage, tiles, oam, empty)
}

func TestObjectCanvasCandidatesPaddingAndFinalSlot(t *testing.T) {
	src := make([]byte, FramebufferSize)
	var coverage [objectCanvasCoverageWords]uint32
	var candidates [objectCanvasRows]uint16
	for obj := 150 - objectCanvasObjectLimit; obj < 150; obj++ {
		x, y := obj%15*16, obj/15*16
		objectCanvasSetPixel(src, &coverage, x+15, y+15, byte(1+obj))
		candidates[y/16] |= 1 << uint(x/16)
	}
	tiles, oam := bytes.Repeat([]byte{0xab}, objectCanvasTileBytes+32), bytes.Repeat([]byte{0xcd}, objectCanvasOAMBytes+16)
	result := packObjectCanvas(src, &coverage, &candidates, tiles, oam)
	if result.objects != objectCanvasObjectLimit || result.changed != ([objectCanvasDirtyWords]uint32{^uint32(0), ^uint32(0), ^uint32(0)}) {
		t.Fatalf("96 slots=%+v", result)
	}
	assertObjectCanvasMatches(t, src, &coverage, tiles, oam, result)
	if !bytes.Equal(tiles[objectCanvasTileBytes:], bytes.Repeat([]byte{0xab}, 32)) || !bytes.Equal(oam[objectCanvasOAMBytes:], bytes.Repeat([]byte{0xcd}, 16)) {
		t.Fatal("packer overwrote buffer padding")
	}
}

func TestObjectCanvasPackAllocatesNothing(t *testing.T) {
	src := make([]byte, FramebufferSize)
	var coverage [objectCanvasCoverageWords]uint32
	tiles, oam := make([]byte, objectCanvasTileBytes), make([]byte, objectCanvasOAMBytes)
	objectCanvasSetPixel(src, &coverage, 13, 7, 0)
	if n := testing.AllocsPerRun(10, func() { packObjectCanvas(src, &coverage, nil, tiles, oam) }); n != 0 {
		t.Fatalf("allocated%g", n)
	}
}

func TestObjectCanvasDMARunsAndUnalignedBuffers(t *testing.T) {
	var dirty [objectCanvasDirtyWords]uint32
	var runs [objectCanvasRunLimit]uint16
	for slot := 0; slot < objectCanvasObjectLimit; slot += 2 {
		dirty[slot>>5] |= 1 << uint(slot&31)
	}
	count, cost, size := prepareObjectCanvasRuns(&dirty, &runs)
	if count != objectCanvasRunLimit || size != uint32(objectCanvasObjectLimit/2*256) || cost != 2*size+uint32(count*192) {
		t.Fatalf("fragmented runs count=%d cost=%d bytes=%d", count, cost, size)
	}
	for i, run := range runs[:count] {
		if int(run&127) != i*2 || run>>7 != 1 {
			t.Fatalf("run%d=%04x", i, run)
		}
	}
	for i := range dirty {
		dirty[i] = ^uint32(0)
	}
	count, cost, size = prepareObjectCanvasRuns(&dirty, &runs)
	if count != 1 || runs[0]>>7 != objectCanvasObjectLimit || size != objectCanvasTileBytes || cost != 2*size+192 {
		t.Fatalf("full runs count=%d cost=%d bytes=%d", count, cost, size)
	}
	srcBacking, tilesBacking := make([]byte, FramebufferSize+1), make([]byte, objectCanvasTileBytes+1)
	src, tiles := srcBacking[1:], tilesBacking[1:]
	oam := make([]byte, objectCanvasOAMBytes)
	var coverage [objectCanvasCoverageWords]uint32
	for y := 3; y < 37; y++ {
		for x := 1; x < 43; x++ {
			if (x+y)%3 != 0 {
				objectCanvasSetPixel(src, &coverage, x, y, byte((x+y)%9))
			}
		}
	}
	result := packObjectCanvas(src, &coverage, nil, tiles, oam)
	assertObjectCanvasMatches(t, src, &coverage, tiles, oam, result)
}
