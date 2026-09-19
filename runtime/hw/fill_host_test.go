//go:build !tinygo

package hw

import "testing"

func TestFill32WritesPackedWords(t *testing.T) {
	dst := []byte{9, 9, 9, 9, 9, 9, 9, 9, 7, 7, 7, 7}
	Fill32(dst, 0x04030201, 2)
	want := []byte{1, 2, 3, 4, 1, 2, 3, 4, 7, 7, 7, 7}
	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("dst[%d] = %d, want %d", i, dst[i], want[i])
		}
	}
}

func TestFill32RejectsBadCount(t *testing.T) {
	dst := []byte{1, 2, 3, 4}
	Fill32(dst, 0xFFFFFFFF, 0)
	Fill32(dst, 0xFFFFFFFF, 2)
	Fill32(nil, 0xFFFFFFFF, 1)
	if dst[0] != 1 || dst[3] != 4 {
		t.Fatal("Fill32 changed dst on a rejected request")
	}
}

// The native companion in scripts/dma-check also exercises the DMA-sensitive
// target path: unaligned DMA32 would mask the address and overwrite the guards.
func TestFill32UnalignedPreservesGuards(t *testing.T) {
	for offset := 0; offset < 4; offset++ {
		for words := 1; words <= 8; words++ {
			dst := make([]byte, 48)
			for i := range dst {
				dst[i] = 0xa5
			}
			start := 4 + offset
			Fill32(dst[start:start+words*4], 0x12345678, words)
			for i, got := range dst {
				want := byte(0xa5)
				if i >= start && i < start+words*4 {
					want = byte(uint32(0x12345678) >> uint(((i-start)&3)*8))
				}
				if got != want {
					t.Fatalf("offset=%d words=%d byte=%d got=%#x want=%#x", offset, words, i, got, want)
				}
			}
		}
	}
}
