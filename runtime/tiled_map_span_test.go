//go:build mtile_mode0 || mtile_tilecanvas

package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func TestMode0MapSpanMatchesFilteredCells(t *testing.T) {
	flags := make([]byte, 1024)
	for i := range flags {
		flags[i] = byte(i*71 + 13)
	}
	for _, shift := range []int{0, 1, 2, 3} {
		for _, width := range []int{0, 1, 7, 31, 32, 33, 64} {
			for _, limit := range []int{0, 1, 255, 512, 768, 1024} {
				for _, mask := range []byte{0, 1, 3, 128, 255} {
					t.Run(fmt.Sprintf("shift%d/w%d/limit%d/mask%d", shift, width, limit, mask), func(t *testing.T) {
						src := make([]uint16, width)
						for i := range src {
							src[i] = uint16(i * 97 % 1103)
						}
						// Ensure the sentinel and limit boundaries are represented.
						if width >= 4 {
							src[0], src[1], src[2], src[3] = 0, uint16(limit), uint16(max(0, limit-1)), 65535
						}
						got := bytes.Repeat([]byte{0xa5}, 8+width*2+8)
						want := append([]byte(nil), got...)
						start := 4 + shift
						for i, id := range src {
							if int(id) >= limit || mask != 0 && flags[id]&mask != mask {
								id = 0
							}
							want[start+i*2], want[start+i*2+1] = byte(id), byte(id>>8)
						}
						mode0CopyMapSpan(got[start:start+width*2], src, flags, limit, mask)
						if !bytes.Equal(got, want) {
							t.Fatal("filtered pixels or neighboring canary bytes differ")
						}
					})
				}
			}
		}
	}
}

func TestMode0MapSpanRejectsShortDestination(t *testing.T) {
	dst := []byte{0x12, 0x34, 0x56}
	mode0CopyMapSpan(dst, []uint16{1, 2}, nil, 3, 0)
	if !bytes.Equal(dst, []byte{0x12, 0x34, 0x56}) {
		t.Fatal("short destination was partially overwritten")
	}
}
