package runtime

import (
	"bytes"
	"testing"
	"unsafe"
)

func TestMapCell8Key0PreservesPixelsAndGuards(t *testing.T) {
	var words [16]uint32
	source := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), 64)
	state := uint32(0x3d28c73a)
	for pattern := 0; pattern < 1408; pattern++ {
		for pixel := range source {
			state = state*1664525 + 1013904223
			switch {
			case pattern < 256:
				source[pixel] = byte(pattern)
			case pattern < 320:
				source[pixel] = 0
				if pixel == pattern-256 {
					source[pixel] = 255
				}
			case pattern < 384:
				source[pixel] = 255
				if pixel == pattern-320 {
					source[pixel] = 0
				}
			default:
				source[pixel] = byte(state >> 24)
				if pattern&1 != 0 && state&2 == 0 {
					source[pixel] = 0
				}
			}
		}
		savedSource := append([]byte(nil), source...)
		for alignment := 0; alignment < 4; alignment++ {
			var backing [8*ScreenWidth/4 + 8]uint32
			destination := unsafe.Slice((*byte)(unsafe.Pointer(&backing[0])), len(backing)*4)
			for i := range destination {
				destination[i] = byte(i*73 + pattern)
			}
			want := append([]byte(nil), destination...)
			start := 16 + alignment
			for pixel, color := range source {
				if color != 0 {
					want[start+(pixel/8)*ScreenWidth+pixel%8] = color
				}
			}
			copyMapCell8Key0(unsafe.Pointer(&source[0]), unsafe.Pointer(&destination[start]))
			if !bytes.Equal(destination, want) {
				t.Fatalf("pattern %d destination alignment %d changed pixels or guards", pattern, alignment)
			}
			if !bytes.Equal(source, savedSource) {
				t.Fatal("source cell was modified")
			}
		}
	}
}
