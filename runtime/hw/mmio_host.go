//go:build !tinygo

package hw

import "unsafe"

// The host implementation is deliberately small. It makes the runtime's
// drawing and input semantics testable without pretending to emulate GBA
// scanline timing.
var (
	hostDISPCNT  uint16 = 0x0400
	hostVCOUNT   uint16 = ScreenHeight
	hostKEYINPUT uint16 = 0x03FF
	hostBG2PA    uint16
	hostBG2PB    uint16
	hostBG2PC    uint16
	hostBG2PD    uint16
	hostBG2X     uint32
	hostBG2Y     uint32
	hostIO       [0x400]byte
	hostVRAM     [VRAMBytes]byte
	hostOAM      [OAMBytes]byte
	hostPalette  [512]uint16
	hostWrites   [128]hostRegisterWrite
	hostWriteN   int
)

type hostRegisterWrite struct {
	Address uintptr
	Value   uint16
}

func recordHostRegisterWrite(address uintptr, value uint16) {
	if hostWriteN < len(hostWrites) {
		hostWrites[hostWriteN] = hostRegisterWrite{Address: address, Value: value}
	}
	hostWriteN++
}

func GetReg16(addr uintptr) uint16 {
	switch addr {
	case DISPCNT:
		return hostDISPCNT
	case VCOUNT:
		return hostVCOUNT
	case KEYINPUT:
		return hostKEYINPUT
	case BG2PA:
		return hostBG2PA
	case BG2PB:
		return hostBG2PB
	case BG2PC:
		return hostBG2PC
	case BG2PD:
		return hostBG2PD
	default:
		if off, ok := hostIOOffset(addr, 2); ok {
			return uint16(hostIO[off]) | uint16(hostIO[off+1])<<8
		}
		if off, ok := hostVRAMOffset(addr, 2); ok {
			return uint16(hostVRAM[off]) | uint16(hostVRAM[off+1])<<8
		}
		if off, ok := hostOAMOffset(addr, 2); ok {
			return uint16(hostOAM[off]) | uint16(hostOAM[off+1])<<8
		}
		if off, ok := hostPaletteOffset(addr, 2); ok {
			return hostPalette[off/2]
		}
	}
	return 0
}

func SetReg16(addr uintptr, value uint16) {
	recordHostRegisterWrite(addr, value)
	switch addr {
	case DISPCNT:
		hostDISPCNT = value
	case VCOUNT:
		hostVCOUNT = value
	case KEYINPUT:
		hostKEYINPUT = value
	case BG2PA:
		hostBG2PA = value
	case BG2PB:
		hostBG2PB = value
	case BG2PC:
		hostBG2PC = value
	case BG2PD:
		hostBG2PD = value
	default:
		if off, ok := hostIOOffset(addr, 2); ok {
			hostIO[off] = byte(value)
			hostIO[off+1] = byte(value >> 8)
			return
		}
		if off, ok := hostVRAMOffset(addr, 2); ok {
			hostVRAM[off] = byte(value)
			hostVRAM[off+1] = byte(value >> 8)
			return
		}
		if off, ok := hostOAMOffset(addr, 2); ok {
			hostOAM[off] = byte(value)
			hostOAM[off+1] = byte(value >> 8)
			return
		}
		if off, ok := hostPaletteOffset(addr, 2); ok {
			hostPalette[off/2] = value
		}
	}
}

func GetReg32(addr uintptr) uint32 {
	switch addr {
	case BG2X:
		return hostBG2X
	case BG2Y:
		return hostBG2Y
	default:
		if off, ok := hostIOOffset(addr, 4); ok {
			return uint32(hostIO[off]) |
				uint32(hostIO[off+1])<<8 |
				uint32(hostIO[off+2])<<16 |
				uint32(hostIO[off+3])<<24
		}
		if off, ok := hostVRAMOffset(addr, 4); ok {
			return uint32(hostVRAM[off]) |
				uint32(hostVRAM[off+1])<<8 |
				uint32(hostVRAM[off+2])<<16 |
				uint32(hostVRAM[off+3])<<24
		}
		if off, ok := hostOAMOffset(addr, 4); ok {
			return uint32(hostOAM[off]) |
				uint32(hostOAM[off+1])<<8 |
				uint32(hostOAM[off+2])<<16 |
				uint32(hostOAM[off+3])<<24
		}
	}
	return 0
}

func SetReg32(addr uintptr, value uint32) {
	switch addr {
	case BG2X:
		hostBG2X = value
	case BG2Y:
		hostBG2Y = value
	default:
		if off, ok := hostIOOffset(addr, 4); ok {
			hostIO[off] = byte(value)
			hostIO[off+1] = byte(value >> 8)
			hostIO[off+2] = byte(value >> 16)
			hostIO[off+3] = byte(value >> 24)
			return
		}
		if off, ok := hostVRAMOffset(addr, 4); ok {
			hostVRAM[off] = byte(value)
			hostVRAM[off+1] = byte(value >> 8)
			hostVRAM[off+2] = byte(value >> 16)
			hostVRAM[off+3] = byte(value >> 24)
			return
		}
		if off, ok := hostOAMOffset(addr, 4); ok {
			hostOAM[off] = byte(value)
			hostOAM[off+1] = byte(value >> 8)
			hostOAM[off+2] = byte(value >> 16)
			hostOAM[off+3] = byte(value >> 24)
		}
	}
}

func SetVRAM32(addr uintptr, value uint32) {
	SetReg32(addr, value)
}

func ReadVRAM32(addr uintptr) uint32 {
	return GetReg32(addr)
}

func SetPaletteRAM16(index int, value uint16) {
	if index >= 0 && index < len(hostPalette) {
		hostPalette[index] = value
	}
}

func SetPaletteRAM32(index int, lo, hi uint16) {
	if index < 0 || index&1 != 0 || index+1 >= len(hostPalette) {
		return
	}
	hostPalette[index] = lo
	hostPalette[index+1] = hi
}

func ReadPaletteRAM16(index int) uint16 {
	if index < 0 || index >= len(hostPalette) {
		return 0
	}
	return hostPalette[index]
}

func CopyToVRAM32DMA(src []byte, dest uintptr, words int) {
	if words <= 0 || words > 0xFFFF || len(src) < words*4 || dest&3 != 0 {
		return
	}
	if uintptr(unsafe.Pointer(&src[0]))&3 != 0 {
		copyBytesToVRAM32(src, dest, words)
		return
	}
	CopyToVRAM32CPU(src, dest, words)
}

func CopyWordsToVRAM32DMA(src []uint32, dest uintptr, words int) {
	if words <= 0 || words > len(src) || words > 0xFFFF || dest&3 != 0 {
		return
	}
	for index := 0; index < words; index++ {
		SetVRAM32(dest+uintptr(index*4), src[index])
	}
}

func hostIOOffset(addr uintptr, width int) (int, bool) {
	const ioBase uintptr = 0x04000000
	if addr < ioBase || width <= 0 {
		return 0, false
	}
	off := addr - ioBase
	if off >= uintptr(len(hostIO)) || off+uintptr(width) > uintptr(len(hostIO)) {
		return 0, false
	}
	return int(off), true
}

func hostVRAMOffset(addr uintptr, width int) (int, bool) {
	if addr < VRAM {
		return 0, false
	}
	off := addr - VRAM
	if off >= uintptr(len(hostVRAM)) || width < 0 || off+uintptr(width) > uintptr(len(hostVRAM)) {
		return 0, false
	}
	return int(off), true
}

func hostOAMOffset(addr uintptr, width int) (int, bool) {
	if addr < OAM {
		return 0, false
	}
	off := addr - OAM
	if off >= uintptr(len(hostOAM)) || width < 0 || off+uintptr(width) > uintptr(len(hostOAM)) {
		return 0, false
	}
	return int(off), true
}

func hostPaletteOffset(addr uintptr, width int) (int, bool) {
	if addr < PALRAM {
		return 0, false
	}
	off := addr - PALRAM
	if off >= uintptr(len(hostPalette)*2) || width < 0 || off+uintptr(width) > uintptr(len(hostPalette)*2) || off&1 != 0 {
		return 0, false
	}
	return int(off), true
}

// The following helpers are test controls, not runtime API. KEYINPUT remains
// active-low: callers pass the raw register value.
func ResetForTest() {
	resetManagedSoundForTest()
	resetManagedPSGForTest()
	hostDISPCNT = 0x0400
	hostVCOUNT = ScreenHeight
	hostKEYINPUT = 0x03FF
	hostBG2PA, hostBG2PB, hostBG2PC, hostBG2PD = 0, 0, 0, 0
	hostBG2X, hostBG2Y = 0, 0
	hostWriteN = 0
	for i := range hostWrites {
		hostWrites[i] = hostRegisterWrite{}
	}
	for i := range hostIO {
		hostIO[i] = 0
	}
	for i := range hostVRAM {
		hostVRAM[i] = 0
	}
	for i := range hostOAM {
		hostOAM[i] = 0
	}
	for i := range hostPalette {
		hostPalette[i] = 0
	}
	hostCycleCount = 0
	hostFrameCycleCount = 0
}

func SetVCountForTest(value uint16) {
	hostVCOUNT = value
}

func SetKeyInputForTest(value uint16) {
	hostKEYINPUT = value
}
