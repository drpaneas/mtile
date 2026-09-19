//go:build tinygo

package main

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"unsafe"
)

//go:section .ewram
var storage [4100]uint32

type fixture struct {
	raw, packed []byte
	frames      [64][]byte
}

func bytesOf(s []uint32) []byte { return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*4) }
func output(i int, v uint32)    { hw.SetReg32(0x02030000+uintptr(i*4), v) }

//export trial_lz77
func biosLZ77(src, dst unsafe.Pointer)

//go:noinline
func validate(src []byte, capacity int) int {
	if len(src) < 4 || src[0] != 0x10 {
		return 0
	}
	n := int(src[1]) | int(src[2])<<8 | int(src[3])<<16
	if n <= 0 || n > capacity {
		return 0
	}
	pos, produced := 4, 0
	for produced < n {
		if pos >= len(src) {
			return 0
		}
		flags := src[pos]
		pos++
		for bit := uint(0); bit < 8 && produced < n; bit++ {
			if flags&(0x80>>bit) == 0 {
				if pos >= len(src) {
					return 0
				}
				pos++
				produced++
				continue
			}
			if len(src)-pos < 2 {
				return 0
			}
			count := int(src[pos]>>4) + 3
			distance := (int(src[pos]&15)<<8 | int(src[pos+1])) + 1
			pos += 2
			if distance > produced || count > n-produced {
				return 0
			}
			produced += count
		}
	}
	return n
}

//go:noinline
func decode(src, dst []byte) bool {
	if validate(src, len(dst)) == 0 {
		return false
	}
	if uintptr(unsafe.Pointer(&src[0]))&3 != 0 || uintptr(unsafe.Pointer(&dst[0]))&3 != 0 {
		return false
	}
	biosLZ77(unsafe.Pointer(&src[0]), unsafe.Pointer(&dst[0]))
	return true
}

//go:noinline
func rawCopy(src, dst []byte) { copy(dst, src) }
func reset() {
	for i := range storage {
		storage[i] = 0xa5a5a5a5
	}
}
func mismatches(raw []byte) uint32 {
	buf := bytesOf(storage[:])
	bad := uint32(0)
	for i, got := range buf {
		want := byte(0xa5)
		if i >= 8 && i < 8+len(raw) {
			want = raw[i-8]
		}
		if got != want {
			bad++
		}
	}
	return bad
}
func main() {
	hw.SetReg16(hw.DISPCNT, 0x80)
	buf := bytesOf(storage[:])
	idx := 1
	for fixtureIndex := range fixtureData {
		f := &fixtureData[fixtureIndex]
		dst := buf[8 : 8+len(f.raw)]
		reset()
		hw.StartFrameCycleCounter()
		rawCopy(f.raw, dst)
		elapsed := hw.StopFrameCycleCounter()
		output(idx, elapsed)
		output(idx+1, mismatches(f.raw))
		idx += 2
		reset()
		hw.StartFrameCycleCounter()
		n := validate(f.packed, len(dst))
		elapsed = hw.StopFrameCycleCounter()
		output(idx, elapsed)
		output(idx+1, uint32(n))
		idx += 2
		reset()
		hw.StartFrameCycleCounter()
		biosLZ77(unsafe.Pointer(&f.packed[0]), unsafe.Pointer(&dst[0]))
		elapsed = hw.StopFrameCycleCounter()
		output(idx, elapsed)
		output(idx+1, mismatches(f.raw))
		idx += 2
		reset()
		hw.StartFrameCycleCounter()
		ok := decode(f.packed, dst)
		elapsed = hw.StopFrameCycleCounter()
		output(idx, elapsed)
		output(idx+1, mismatches(f.raw))
		if !ok {
			output(idx+1, 0xffffffff)
		}
		idx += 2
		min, max, sum, errors := ^uint32(0), uint32(0), uint32(0), uint32(0)
		rawSum := uint32(0)
		for frame, packed := range f.frames {
			reset()
			hw.StartFrameCycleCounter()
			ok = decode(packed, dst[:256])
			elapsed = hw.StopFrameCycleCounter()
			if elapsed < min {
				min = elapsed
			}
			if elapsed > max {
				max = elapsed
			}
			sum += elapsed
			expected := f.raw[frame*256 : (frame+1)*256]
			errors += mismatches(expected)
			if !ok {
				errors++
			}
			reset()
			hw.StartFrameCycleCounter()
			rawCopy(expected, dst[:256])
			rawSum += hw.StopFrameCycleCounter()
			errors += mismatches(expected)
		}
		output(idx, min)
		output(idx+1, max)
		output(idx+2, sum)
		output(idx+3, errors)
		output(idx+4, rawSum)
		idx += 5
		reset()
		ok = decode(f.packed, dst[:len(dst)-1])
		errors = mismatches(nil)
		if ok {
			errors++
		}
		output(idx, errors)
		idx++
		reset()
		limit := len(f.packed)
		if limit > len(dst) {
			limit = len(dst) - 1
		}
		ok = decode(f.packed, dst[:limit])
		errors = mismatches(nil)
		if ok {
			errors++
		}
		output(idx, errors)
		idx++
	}
	for _, packed := range malformedData {
		reset()
		ok := decode(packed, buf[8:8+16384])
		errors := mismatches(nil)
		if ok {
			errors++
		}
		output(idx, errors)
		idx++
	}
	output(512, uint32(idx))
	output(0, 0xD4A70001)
	for {
	}
}
