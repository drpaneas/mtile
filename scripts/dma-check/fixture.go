//go:build tinygo

package main

import (
	"device/gba"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"
)

//go:section .ewram
var source [9600]uint32

//go:section .ewram
var guard [16]uint32
var irqCount volatile.Register32
var irqDelay volatile.Register32
var timerIRQ = interrupt.New(gba.IRQ_TIMER0, onTimer)

func onTimer(interrupt.Interrupt) {
	irqCount.Set(irqCount.Get() + 1)
	start := hw.GetReg16(hw.TM0CNT_L)
	for uint16(hw.GetReg16(hw.TM0CNT_L)-start) < uint16(irqDelay.Get()) {
	}
}
func output(i int, v uint32) { hw.SetReg32(0x02030000+uintptr(i*4), v) }

//export trial_fastset
func fastset(src unsafe.Pointer, dest unsafe.Pointer, words uint32)

//go:noinline
func copyKind(kind int, src []byte) {
	switch kind {
	case 0:
		hw.CopyToVRAM32DMA(src, hw.VRAM, 9600)
	case 1:
		for start := 0; start < 9600; start += 4096 {
			n := 9600 - start
			if n > 4096 {
				n = 4096
			}
			hw.CopyToVRAM32DMA(src[start*4:], hw.VRAM+uintptr(start*4), n)
		}
	case 2:
		fastset(unsafe.Pointer(&src[0]), unsafe.Pointer(hw.VRAM), 9600)
	}
}
func main() {
	hw.SetReg16(hw.DISPCNT, 0x0080)
	src := unsafe.Slice((*byte)(unsafe.Pointer(&source[0])), 38400)
	for i := range source {
		source[i] = uint32(i)*0x01020304 + 0x789abcde
	}
	idx := 1
	for offset := 0; offset < 4; offset++ {
		dst := unsafe.Slice((*byte)(unsafe.Pointer(&guard[0])), 64)
		for i := range dst {
			dst[i] = 0xa5
		}
		hw.StartFrameCycleCounter()
		hw.Fill32(dst[8+offset:40+offset], 0x12345678, 8)
		cycles := hw.StopFrameCycleCounter()
		errors := uint32(0)
		for i, b := range dst {
			want := byte(0xa5)
			if i >= 8+offset && i < 40+offset {
				want = byte(uint32(0x12345678) >> uint(((i-8-offset)&3)*8))
			}
			if b != want {
				errors++
			}
		}
		output(idx, errors)
		output(idx+1, cycles)
		idx += 2
	}
	for _, n := range [4]int{-1, 0, 65536, 17} {
		dst := unsafe.Slice((*byte)(unsafe.Pointer(&guard[0])), 64)
		for i := range dst {
			dst[i] = 0xa5
		}
		hw.Fill32(dst, 0x12345678, n)
		errors := uint32(0)
		for _, b := range dst {
			if b != 0xa5 {
				errors++
			}
		}
		output(idx, errors)
		idx++
	}
	for kind := 0; kind < 3; kind++ {
		hw.StartFrameCycleCounter()
		copyKind(kind, src)
		cycles := hw.StopFrameCycleCounter()
		errors := uint32(0)
		for i := range source {
			if hw.ReadVRAM32(hw.VRAM+uintptr(i*4)) != source[i] {
				errors++
			}
		}
		output(idx, cycles)
		output(idx+1, errors)
		idx += 2
	}
	for busy := 0; busy < 2; busy++ {
		irqDelay.Set(uint32(busy * 16384))
		for kind := 0; kind < 3; kind++ {
			for phase := 0; phase < 8; phase++ {
				irqCount.Set(0)
				hw.SetReg16(hw.TM0CNT_H, 0)
				hw.SetReg16(hw.TM1CNT_H, 0)
				hw.SetReg16(hw.INTERRUPT_IF, 8)
				hw.SetReg16(hw.TM0CNT_L, 0)
				hw.SetReg16(hw.TM1CNT_L, 0)
				hw.SetReg16(hw.TM1CNT_H, 0x84)
				timerIRQ.Enable()
				hw.SetReg16(hw.TM0CNT_H, 0xc0)
				for hw.GetReg16(hw.TM0CNT_L) < uint16(phase*8192) {
				}
				copyKind(kind, src)
				hw.StartFrameCycleCounter()
				for hw.ReadFrameCycleCounter() < 2000 {
				}
				hw.SetReg16(hw.TM0CNT_H, 0)
				hw.SetReg16(hw.TM1CNT_H, 0)
				output(idx, uint32(hw.GetReg16(hw.TM1CNT_L)))
				output(idx+1, irqCount.Get())
				idx += 2
				hw.SetReg16(hw.INTERRUPT_IE, 0)
			}
		}
	}

	for kind := 0; kind < 2; kind++ {
		for _, n := range [13]int{-1, 0, 65536, 9601, 1, 2, 7, 8, 31, 32, 4096, 4097, 9600} {
			for i := 0; i < 9601; i++ {
				hw.SetVRAM32(hw.VRAM+uintptr(i*4), 0xa5a5a5a5)
			}
			hw.StartFrameCycleCounter()
			if kind == 0 {
				hw.CopyToVRAM32DMA(src, hw.VRAM, n)
			} else {
				hw.CopyWordsToVRAM32DMA(source[:], hw.VRAM, n)
			}
			cycles := hw.StopFrameCycleCounter()
			errors := uint32(0)
			for i := 0; i < 9601; i++ {
				want := uint32(0xa5a5a5a5)
				if n > 0 && n <= 9600 && i < n {
					want = source[i]
				}
				if hw.ReadVRAM32(hw.VRAM+uintptr(i*4)) != want {
					errors++
				}
			}
			output(idx, errors)
			output(idx+1, cycles)
			idx += 2
		}
	}

	for sourceOffset := 0; sourceOffset < 4; sourceOffset++ {
		for destOffset := 0; destOffset < 4; destOffset++ {
			for i := 0; i < 12; i++ {
				hw.SetVRAM32(hw.VRAM+uintptr(i*4), 0xa5a5a5a5)
			}
			hw.StartFrameCycleCounter()
			hw.CopyToVRAM32DMA(src[sourceOffset:], hw.VRAM+8+uintptr(destOffset), 8)
			cycles := hw.StopFrameCycleCounter()
			errors := uint32(0)
			for i := 0; i < 12; i++ {
				want := uint32(0xa5a5a5a5)
				if destOffset == 0 && i >= 2 && i < 10 {
					p := sourceOffset + (i-2)*4
					want = uint32(src[p]) | uint32(src[p+1])<<8 | uint32(src[p+2])<<16 | uint32(src[p+3])<<24
				}
				if hw.ReadVRAM32(hw.VRAM+uintptr(i*4)) != want {
					errors++
				}
			}
			output(idx, errors)
			output(idx+1, cycles)
			idx += 2
		}
	}
	for destOffset := 0; destOffset < 4; destOffset++ {
		for i := 0; i < 12; i++ {
			hw.SetVRAM32(hw.VRAM+uintptr(i*4), 0xa5a5a5a5)
		}
		hw.StartFrameCycleCounter()
		hw.CopyWordsToVRAM32DMA(source[:], hw.VRAM+8+uintptr(destOffset), 8)
		cycles := hw.StopFrameCycleCounter()
		errors := uint32(0)
		for i := 0; i < 12; i++ {
			want := uint32(0xa5a5a5a5)
			if destOffset == 0 && i >= 2 && i < 10 {
				want = source[i-2]
			}
			if hw.ReadVRAM32(hw.VRAM+uintptr(i*4)) != want {
				errors++
			}
		}
		output(idx, errors)
		output(idx+1, cycles)
		idx += 2
	}
	output(512, uint32(idx))
	output(0, 0xD4A70001)
	for {
	}
}
