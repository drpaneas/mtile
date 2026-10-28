//go:build !tinygo

package hw

import "testing"

func TestVideoDMABackupBoundsAndWords(t *testing.T) {
	ResetForTest()
	for _, source := range []uintptr{VRAM + VRAMBytes - 8, OAM + OAMBytes - 8} {
		SetReg32(source, 0x12345678)
		SetReg32(source+4, 0xabcdef01)
		var got [2]uint32
		ReadVideoWords32DMA(got[:], source)
		if got != [2]uint32{0x12345678, 0xabcdef01} {
			t.Fatal(got)
		}
		for _, bad := range []uintptr{source + 1, source + 4, 0, DISPCNT} {
			got = [2]uint32{7, 9}
			ReadVideoWords32DMA(got[:], bad)
			if got != [2]uint32{7, 9} {
				t.Fatalf("invalid source %#x changed backup", bad)
			}
		}
	}
	ReadVideoWords32DMA(nil, VRAM)
}
