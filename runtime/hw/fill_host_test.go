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
