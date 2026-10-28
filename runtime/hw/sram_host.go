//go:build !tinygo

package hw

// Host storage is a deterministic test double, not persistent desktop saving.
// It survives Context replacement to model cartridge memory across cold boots.
var hostSRAM [32768]byte
var hostSRAMWriteLimit = -1

func PrepareSRAM() {}

func ReadSRAMByte(offset int) byte {
	if uint(offset) >= uint(len(hostSRAM)) {
		return 0xff
	}
	return hostSRAM[offset]
}

func WriteSRAMByte(offset int, value byte) {
	if uint(offset) >= uint(len(hostSRAM)) || hostSRAMWriteLimit == 0 {
		return
	}
	if hostSRAMWriteLimit > 0 {
		hostSRAMWriteLimit--
	}
	hostSRAM[offset] = value
}

func ResetSRAMForTest(fill byte) {
	for i := range hostSRAM {
		hostSRAM[i] = fill
	}
	hostSRAMWriteLimit = -1
}

// SetSRAMWriteLimitForTest drops all writes after n successful byte stores.
// Zero models read-only/absent storage; -1 restores ordinary writes.
func SetSRAMWriteLimitForTest(n int) { hostSRAMWriteLimit = n }
