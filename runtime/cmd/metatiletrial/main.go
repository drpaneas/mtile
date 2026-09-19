//go:build tinygo

// A standalone native experiment: expand 2x2 metatiles into a 32x32 tile map.
package main

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
	"unsafe"
)

type guardedMap struct {
	before uint32
	pixels [1024]uint16
	after  uint32
}

//go:section .ewram
var output guardedMap

//go:noinline
func rawMap(source *[1024]uint16) { copy(output.pixels[:], source[:]) }

//go:noinline
func metatileMap(ids *[256]byte) {
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			p := y*64 + x*2
			tile := &dict[ids[y*16+x]]
			output.pixels[p], output.pixels[p+1] = tile[0], tile[1]
			output.pixels[p+32], output.pixels[p+33] = tile[2], tile[3]
		}
	}
}
func word(index int) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(uintptr(0x0203f000 + index*4)))
}
func check(want *[1024]uint16) uint32 {
	bad := uint32(0)
	for i, pixel := range output.pixels {
		if pixel != want[i] {
			bad++
		}
	}
	if output.before != 0x1badcafe || output.after != 0x2468ace0 {
		bad += 100000
	}
	return bad
}
func main() {
	word(0).Set(0x12340001)
	output.before, output.after = 0x1badcafe, 0x2468ace0
	hw.StartFrameCycleCounter()
	for fixture := 0; fixture < 2; fixture++ {
		src, ids := &repeatedFlat, &repeated
		if fixture == 1 {
			src, ids = &diverseFlat, &diverse
		}
		start := hw.ReadFrameCycleCounter()
		for i := 0; i < 100; i++ {
			rawMap(src)
		}
		word(1 + fixture*3).Set((hw.ReadFrameCycleCounter() - start) / 100)
		mismatch := check(src)
		start = hw.ReadFrameCycleCounter()
		for i := 0; i < 100; i++ {
			metatileMap(ids)
		}
		word(2 + fixture*3).Set((hw.ReadFrameCycleCounter() - start) / 100)
		word(3 + fixture*3).Set(mismatch + check(src))
	}
	word(0).Set(0x12340002)
	for {
	}
}
