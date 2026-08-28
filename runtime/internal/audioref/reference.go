// Package audioref implements the deliberately small direct-register audio
// truth ROMs. It does not use the production audio controller or sequencer:
// the reference set is the third implementation used to verify both of them.
package audioref

import (
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

// Kind selects one independent hardware reference cartridge.
type Kind uint8

const (
	CH1 Kind = iota
	CH2
	CH3
	CH4
	FIFOA
	FIFOB
	All
)

// Route is the GBA output terminal selection used by the truth cases.
type Route uint8

const (
	RouteNone Route = iota
	RouteRight
	RouteLeft
	RouteBoth
)

const (
	soundMasterEnable uint16 = 1 << 7
	psgRatioFull      uint16 = 2
	fifoAFull         uint16 = 1 << 2
	fifoBFull         uint16 = 1 << 3
	fifoARight        uint16 = 1 << 8
	fifoALeft         uint16 = 1 << 9
	fifoATimer1       uint16 = 1 << 10
	fifoAReset        uint16 = 1 << 11
	fifoBRight        uint16 = 1 << 12
	fifoBLeft         uint16 = 1 << 13
	fifoBTimer1       uint16 = 1 << 14
	fifoBReset        uint16 = 1 << 15

	timerEnable uint16 = 1 << 7

	dmaDestinationFixed uint16 = 2 << 5
	dmaSourceFixed      uint16 = 2 << 7
	dmaRepeat           uint16 = 1 << 9
	dmaTransfer32       uint16 = 1 << 10
	dmaStartFIFO        uint16 = 3 << 12
	dmaEnable           uint16 = 1 << 15
	dmaFIFOControl             = dmaDestinationFixed | dmaSourceFixed | dmaRepeat |
		dmaTransfer32 | dmaStartFIFO | dmaEnable
)

// DMA1/2 cannot read Game Pak ROM. These repeated four-sample signals live in
// IWRAM and intentionally require no refill scheduler.
//
//go:section .iwram
var fifoATone uint32 = 0xC0C04040

//go:section .iwram
var fifoBTone uint32 = 0xD030D030

var waveBank0 = [4]uint32{
	0x76543210, 0xFEDCBA98, 0x89ABCDEF, 0x01234567,
}

var waveBank1 = [4]uint32{
	0xFFFF0000, 0xFFFF0000, 0x0000FFFF, 0x0000FFFF,
}

// Little-endian words whose bytes are 01,23,45,67...; because CH3 consumes
// each byte's high nibble first, this audibly verifies sample order 0..15.
var waveNibbleRamp = [4]uint32{
	0x67452301, 0xEFCDAB89, 0x67452301, 0xEFCDAB89,
}

// CaseCount returns the fixed matrix size for one truth ROM.
func CaseCount(kind Kind) int {
	switch kind {
	case CH1:
		return 12
	case CH2, CH3, CH4:
		return 10
	case FIFOA, FIFOB:
		return 6
	case All:
		return 2
	default:
		return 0
	}
}

// KindName and CaseName are ROM UI labels and capture identifiers.
func KindName(kind Kind) string {
	switch kind {
	case CH1:
		return "CH1 PULSE + SWEEP"
	case CH2:
		return "CH2 PULSE"
	case CH3:
		return "CH3 WAVE"
	case CH4:
		return "CH4 NOISE"
	case FIFOA:
		return "DIRECT SOUND FIFO A"
	case FIFOB:
		return "DIRECT SOUND FIFO B"
	case All:
		return "ALL GBA AUDIO"
	default:
		return "INVALID"
	}
}

func CaseName(kind Kind, index int) string {
	if index < 0 || index >= CaseCount(kind) {
		return "INVALID"
	}
	switch kind {
	case CH1:
		return [...]string{
			"12.5% DUTY A4", "25% DUTY C5", "50% DUTY E5", "75% DUTY A4",
			"SWEEP UP", "SWEEP DOWN", "SWEEP SLOW SHIFT 1", "SWEEP FAST SHIFT 3",
			"ENVELOPE DOWN", "ENVELOPE UP", "LEFT ONLY", "RIGHT ONLY",
		}[index]
	case CH2:
		return [...]string{
			"12.5% DUTY A3", "25% DUTY C4", "50% DUTY E4", "75% DUTY A3",
			"ENVELOPE DOWN", "ENVELOPE UP", "LENGTH 125MS", "LEFT ONLY",
			"RIGHT ONLY", "BOTH ROUTE",
		}[index]
	case CH3:
		return [...]string{
			"32 BANK 0 100%", "32 BANK 1 100%", "32-SAMPLE 50%", "32-SAMPLE 25%",
			"32-SAMPLE 75%", "64-SAMPLE BANK 0", "64-SAMPLE BANK 1", "NIBBLE RAMP",
			"LENGTH 250MS", "MUTE LEVEL",
		}[index]
	case CH4:
		return [...]string{
			"15-BIT COARSE", "15-BIT FINE", "7-BIT TONAL", "DIV 0 SHIFT 0",
			"DIV 7 SHIFT 12", "ENVELOPE DOWN", "ENVELOPE UP", "LENGTH 125MS",
			"LEFT ONLY", "RIGHT ONLY",
		}[index]
	case FIFOA, FIFOB:
		return [...]string{"TIMER0 16384 BOTH", "TIMER0 32768 LEFT", "TIMER1 22050 RIGHT", "TIMER1 18157 BOTH", "50% LEVEL", "STOP / RESET"}[index]
	case All:
		return [...]string{"SHARED 16384 CLOCK", "INDEPENDENT 16K/22K"}[index]
	default:
		return "INVALID"
	}
}

// Start programs one truth-table case directly. It returns false without
// touching hardware for an invalid kind or case.
func Start(kind Kind, index int) bool {
	if index < 0 || index >= CaseCount(kind) {
		return false
	}
	Stop()
	startGlobal()
	switch kind {
	case CH1:
		startCH1(index)
	case CH2:
		startCH2(index)
	case CH3:
		startCH3(index)
	case CH4:
		startCH4(index)
	case FIFOA:
		startFIFO(true, index)
	case FIFOB:
		startFIFO(false, index)
	case All:
		startAll(index)
	}
	return true
}

func startGlobal() {
	hw.SetReg16(hw.SOUNDBIAS, 0x0200)
	hw.SetReg16(hw.SOUNDCNT_X, soundMasterEnable)
	hw.SetReg16(hw.SOUNDCNT_L, 0x0077)
	hw.SetReg16(hw.SOUNDCNT_H, psgRatioFull)
}

func psgRoute(channel int, route Route) {
	value := hw.GetReg16(hw.SOUNDCNT_L)
	value &^= 1 << (8 + channel)
	value &^= 1 << (12 + channel)
	if route == RouteRight || route == RouteBoth {
		value |= 1 << (8 + channel)
	}
	if route == RouteLeft || route == RouteBoth {
		value |= 1 << (12 + channel)
	}
	hw.SetReg16(hw.SOUNDCNT_L, value)
}

func pulseDutyEnvelope(duty, initial, pace int, increase bool) uint16 {
	value := uint16(duty&3)<<6 | uint16(pace&7)<<8 | uint16(initial&15)<<12
	if increase {
		value |= 1 << 11
	}
	return value
}

func startCH1(index int) {
	duty := index
	if duty > 3 {
		duty = 2
	}
	period := uint16(1750)
	switch index {
	case 1:
		period = 1824
	case 2:
		period = 1872
	}
	sweep := uint16(0)
	envelopePace := 0
	initial := 15
	increase := false
	route := RouteBoth
	switch index {
	case 4:
		sweep = 2<<4 | 2 // pace 2, increasing period/pitch, shift 2
	case 5:
		sweep = 2<<4 | 1<<3 | 2 // pace 2, decreasing period/pitch, shift 2
	case 6:
		sweep = 7<<4 | 1 // slow, shift 1
	case 7:
		sweep = 1<<4 | 3 // fast, shift 3
	case 8:
		envelopePace = 2
	case 9:
		envelopePace, initial, increase = 2, 2, true
	case 10:
		route = RouteLeft
	case 11:
		route = RouteRight
	}
	psgRoute(0, route)
	hw.SetReg16(hw.SOUND1CNT_L, sweep)
	hw.SetReg16(hw.SOUND1CNT_H, pulseDutyEnvelope(duty, initial, envelopePace, increase))
	hw.SetReg16(hw.SOUND1CNT_X, period|1<<15)
}

func startCH2(index int) {
	duty := index
	if duty > 3 {
		duty = 2
	}
	period := uint16(1610)
	switch index {
	case 1:
		period = 1750
	case 2:
		period = 1824
	}
	route := RouteBoth
	pace := 0
	initial := 13
	increase := false
	length := uint16(0)
	useLength := uint16(0)
	switch index {
	case 4:
		pace = 2
	case 5:
		pace, initial, increase = 2, 2, true
	case 6:
		length, useLength = 32, 1<<14 // 125 ms at the 256 Hz length clock
	case 7:
		route = RouteLeft
	case 8:
		route = RouteRight
	}
	psgRoute(1, route)
	hw.SetReg16(hw.SOUND2CNT_L, length|pulseDutyEnvelope(duty, initial, pace, increase))
	hw.SetReg16(hw.SOUND2CNT_H, period|useLength|1<<15)
}

func writeWaveBank(selectedPlaybackBank uint16, words [4]uint32) {
	// The selected bank is played; the opposite bank is CPU-visible. Select
	// bank 1 to write bank 0 and bank 0 to write bank 1.
	hw.SetReg16(hw.SOUND3CNT_L, selectedPlaybackBank<<6)
	hw.SetReg32(hw.WAVE_RAM0, words[0])
	hw.SetReg32(hw.WAVE_RAM1, words[1])
	hw.SetReg32(hw.WAVE_RAM2, words[2])
	hw.SetReg32(hw.WAVE_RAM3, words[3])
}

func startCH3(index int) {
	writeWaveBank(1, waveBank0)
	writeWaveBank(0, waveBank1)
	level := uint16(1 << 13) // 100%
	dimension := uint16(0)
	bank := uint16(0)
	length := uint16(0)
	useLength := uint16(0)
	switch index {
	case 1:
		bank = 1 << 6
	case 2:
		level = 2 << 13
	case 3:
		level = 3 << 13
	case 4:
		level = 1 << 15
	case 5:
		dimension = 1 << 5
	case 6:
		dimension = 1 << 5
		bank = 1 << 6
	case 7:
		writeWaveBank(1, waveNibbleRamp)
	case 8:
		length, useLength = 192, 1<<14 // (256-192)/256 second
	case 9:
		level = 0
	}
	psgRoute(2, RouteBoth)
	hw.SetReg16(hw.SOUND3CNT_L, dimension|bank|1<<7)
	hw.SetReg16(hw.SOUND3CNT_H, length|level)
	hw.SetReg16(hw.SOUND3CNT_X, 1899|useLength|1<<15)
}

func startCH4(index int) {
	noise := uint16(3 | 5<<4)
	pace := 0
	initial := 15
	increase := false
	length := uint16(0)
	useLength := uint16(0)
	route := RouteBoth
	switch index {
	case 1:
		noise = 1 | 2<<4
	case 2:
		noise = 3 | 1<<3 | 4<<4
	case 3:
		noise = 0
	case 4:
		noise = 7 | 12<<4
	case 5:
		pace = 2
	case 6:
		pace, initial, increase = 2, 2, true
	case 7:
		length, useLength = 32, 1<<14 // 125 ms at the 256 Hz length clock
	case 8:
		route = RouteLeft
	case 9:
		route = RouteRight
	}
	psgRoute(3, route)
	hw.SetReg16(hw.SOUND4CNT_L, length|pulseDutyEnvelope(0, initial, pace, increase))
	hw.SetReg16(hw.SOUND4CNT_H, noise|useLength|1<<15)
}

func routeBits(a bool, route Route) uint16 {
	if a {
		switch route {
		case RouteRight:
			return fifoARight
		case RouteLeft:
			return fifoALeft
		case RouteBoth:
			return fifoARight | fifoALeft
		}
	} else {
		switch route {
		case RouteRight:
			return fifoBRight
		case RouteLeft:
			return fifoBLeft
		case RouteBoth:
			return fifoBRight | fifoBLeft
		}
	}
	return 0
}

func startFIFO(a bool, index int) {
	if index == 5 {
		Stop()
		return
	}
	rate := hw.DirectSoundRate16384
	timer1 := false
	route := RouteBoth
	full := true
	switch index {
	case 1:
		rate, route = hw.DirectSoundRate32768, RouteLeft
	case 2:
		rate, timer1, route = hw.DirectSoundRate22050, true, RouteRight
	case 3:
		rate, timer1 = hw.DirectSoundRate18157, true
	case 4:
		full = false
	}
	clock, _ := hw.DirectSoundClockForRate(rate)
	startFixedFIFO(a, timer1, route, full, clock)
}

func setFIFOControl(a bool, mask, reset uint16) {
	fields := uint16(fifoBFull | fifoBRight | fifoBLeft | fifoBTimer1 | fifoBReset)
	if a {
		fields = fifoAFull | fifoARight | fifoALeft | fifoATimer1 | fifoAReset
	}
	value := hw.GetReg16(hw.SOUNDCNT_H)&^fields | mask
	hw.SetReg16(hw.SOUNDCNT_H, value|reset)
	hw.SetReg16(hw.SOUNDCNT_H, value)
}

func startFixedFIFO(a, timer1 bool, route Route, full bool, clock hw.DirectSoundClock) {
	var source *uint32
	var fifo, sad, dad, cnt uintptr
	var level, timer, reset uint16
	if a {
		source = &fifoATone
		fifo, sad, dad, cnt = hw.FIFO_A, hw.DMA1SAD, hw.DMA1DAD, hw.DMA1CNT
		level, reset = fifoAFull, fifoAReset
		if timer1 {
			timer = fifoATimer1
		}
	} else {
		source = &fifoBTone
		fifo, sad, dad, cnt = hw.FIFO_B, hw.DMA2SAD, hw.DMA2DAD, hw.DMA2CNT
		level, reset = fifoBFull, fifoBReset
		if timer1 {
			timer = fifoBTimer1
		}
	}
	if !full {
		level = 0
	}
	setFIFOControl(a, level|timer|routeBits(a, route), reset)
	hw.SetReg32(fifo, *source)
	hw.SetReg32(sad, uint32(uintptr(unsafe.Pointer(source))))
	hw.SetReg32(dad, uint32(fifo))
	hw.SetReg16(cnt, 4)
	hw.SetReg16(cnt+2, dmaFIFOControl)
	if timer1 {
		hw.SetReg16(hw.TM1CNT_L, clock.TimerReload)
		hw.SetReg16(hw.TM1CNT_H, timerEnable)
	} else {
		hw.SetReg16(hw.TM0CNT_L, clock.TimerReload)
		hw.SetReg16(hw.TM0CNT_H, timerEnable)
	}
}

func startAll(index int) {
	startCH1(4)
	startCH2(2)
	startCH3(4)
	startCH4(2)
	clockA, _ := hw.DirectSoundClockForRate(hw.DirectSoundRate16384)
	clockB := clockA
	timerB := false
	if index == 1 {
		clockB, _ = hw.DirectSoundClockForRate(hw.DirectSoundRate22050)
		timerB = true
	}
	startFixedFIFO(true, false, RouteLeft, true, clockA)
	startFixedFIFO(false, timerB, RouteRight, true, clockB)
}

// Stop leaves the complete audio block in a deterministic silent state.
func Stop() {
	hw.SetReg16(hw.TM0CNT_H, 0)
	hw.SetReg16(hw.TM1CNT_H, 0)
	hw.SetReg16(hw.DMA1CNT+2, 0)
	hw.SetReg16(hw.DMA2CNT+2, 0)
	hw.SetReg16(hw.SOUND1CNT_X, 0)
	hw.SetReg16(hw.SOUND2CNT_H, 0)
	hw.SetReg16(hw.SOUND3CNT_L, 0)
	hw.SetReg16(hw.SOUND3CNT_X, 0)
	hw.SetReg16(hw.SOUND4CNT_H, 0)
	hw.SetReg16(hw.SOUNDCNT_L, 0)
	hw.SetReg16(hw.SOUNDCNT_H, fifoAReset|fifoBReset)
	hw.SetReg16(hw.SOUNDCNT_H, 0)
	hw.SetReg32(hw.FIFO_A, 0)
	hw.SetReg32(hw.FIFO_B, 0)
	hw.SetReg16(hw.TM0CNT_L, 0)
	hw.SetReg16(hw.TM1CNT_L, 0)
	hw.SetReg16(hw.SOUNDBIAS, 0x0200)
	hw.SetReg16(hw.SOUNDCNT_X, 0)
}
