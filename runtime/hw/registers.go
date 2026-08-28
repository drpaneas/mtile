package hw

// GBA memory-mapped registers and Mode 4 layout.
//
// These are uintptr values so the TinyGo implementation can pass them
// directly to runtime/volatile. The desktop implementation treats them as
// symbolic addresses in a small simulator.
const (
	DISPCNT       uintptr = 0x04000000
	VCOUNT        uintptr = 0x04000006
	BG2PA         uintptr = 0x04000020
	BG2PB         uintptr = 0x04000022
	BG2PC         uintptr = 0x04000024
	BG2PD         uintptr = 0x04000026
	BG2X          uintptr = 0x04000028
	BG2Y          uintptr = 0x0400002C
	SOUND1CNT_L   uintptr = 0x04000060
	SOUND1CNT_H   uintptr = 0x04000062
	SOUND1CNT_X   uintptr = 0x04000064
	SOUND2CNT_L   uintptr = 0x04000068
	SOUND2CNT_H   uintptr = 0x0400006C
	SOUND3CNT_L   uintptr = 0x04000070
	SOUND3CNT_H   uintptr = 0x04000072
	SOUND3CNT_X   uintptr = 0x04000074
	SOUND4CNT_L   uintptr = 0x04000078
	SOUND4CNT_H   uintptr = 0x0400007C
	SOUNDCNT_L    uintptr = 0x04000080
	SOUNDCNT_H    uintptr = 0x04000082
	SOUNDCNT_X    uintptr = 0x04000084
	SOUNDBIAS     uintptr = 0x04000088
	WAVE_RAM0     uintptr = 0x04000090
	WAVE_RAM1     uintptr = 0x04000094
	WAVE_RAM2     uintptr = 0x04000098
	WAVE_RAM3     uintptr = 0x0400009C
	FIFO_A        uintptr = 0x040000A0
	FIFO_B        uintptr = 0x040000A4
	DMA1SAD       uintptr = 0x040000BC
	DMA1DAD       uintptr = 0x040000C0
	DMA1CNT       uintptr = 0x040000C4
	DMA2SAD       uintptr = 0x040000C8
	DMA2DAD       uintptr = 0x040000CC
	DMA2CNT       uintptr = 0x040000D0
	KEYINPUT      uintptr = 0x04000130
	DMA3SAD       uintptr = 0x040000D4
	DMA3DAD       uintptr = 0x040000D8
	DMA3CNT       uintptr = 0x040000DC
	TM0CNT_L      uintptr = 0x04000100
	TM0CNT_H      uintptr = 0x04000102
	TM1CNT_L      uintptr = 0x04000104
	TM1CNT_H      uintptr = 0x04000106
	TM2CNT_L      uintptr = 0x04000108
	TM2CNT_H      uintptr = 0x0400010A
	TM3CNT_L      uintptr = 0x0400010C
	TM3CNT_H      uintptr = 0x0400010E
	INTERRUPT_IE  uintptr = 0x04000200
	INTERRUPT_IF  uintptr = 0x04000202
	INTERRUPT_IME uintptr = 0x04000208
	PALRAM        uintptr = 0x05000000
	VRAM          uintptr = 0x06000000
	Page0         uintptr = 0x06000000
	Page1         uintptr = 0x0600A000

	ScreenWidth  = 240
	ScreenHeight = 160
	PageStride   = 0xA000
	VisibleBytes = ScreenWidth * ScreenHeight
	VRAMBytes    = PageStride * 2
)
