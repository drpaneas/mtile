package audioref

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestReferenceCaseTablesAreTotal(t *testing.T) {
	for kind := CH1; kind <= All; kind++ {
		if CaseCount(kind) == 0 || KindName(kind) == "INVALID" {
			t.Fatalf("kind %d has no truth cases", kind)
		}
		for i := 0; i < CaseCount(kind); i++ {
			if CaseName(kind, i) == "INVALID" {
				t.Fatalf("kind %d case %d has no name", kind, i)
			}
		}
	}
	if Start(Kind(99), 0) || Start(CH1, -1) || Start(CH1, CaseCount(CH1)) {
		t.Fatal("invalid reference case accepted")
	}
}

func TestCH1ReferenceProgramsOnlyCH1AndSharedMix(t *testing.T) {
	hw.ResetForTest()
	if !Start(CH1, 4) {
		t.Fatal("CH1 sweep case rejected")
	}
	if got := hw.GetReg16(hw.SOUNDCNT_X); got != soundMasterEnable {
		t.Fatalf("SOUNDCNT_X = %#04x", got)
	}
	if got := hw.GetReg16(hw.SOUNDCNT_L); got != 0x1177 {
		t.Fatalf("SOUNDCNT_L = %#04x, want CH1 both %#04x", got, 0x1177)
	}
	if got := hw.GetReg16(hw.SOUND1CNT_L); got != 0x22 {
		t.Fatalf("SOUND1CNT_L = %#04x, want sweep 0x22", got)
	}
	if got := hw.GetReg16(hw.SOUND1CNT_X); got&(1<<15) == 0 {
		t.Fatalf("SOUND1CNT_X = %#04x, trigger missing", got)
	}
	if got := hw.GetReg16(hw.DMA1CNT + 2); got != 0 {
		t.Fatalf("DMA1 unexpectedly active: %#04x", got)
	}
}

func TestCH3ReferenceWritesBothBanksAndSelects64SampleMode(t *testing.T) {
	hw.ResetForTest()
	if !Start(CH3, 6) {
		t.Fatal("CH3 bank case rejected")
	}
	if got := hw.GetReg16(hw.SOUND3CNT_L); got != 1<<5|1<<6|1<<7 {
		t.Fatalf("SOUND3CNT_L = %#04x", got)
	}
	if got := hw.GetReg32(hw.WAVE_RAM0); got != waveBank1[0] {
		t.Fatalf("visible Wave RAM = %#08x, want final bank write %#08x", got, waveBank1[0])
	}
	if got := hw.GetReg16(hw.SOUND3CNT_X); got&(1<<15) == 0 {
		t.Fatalf("SOUND3CNT_X = %#04x, trigger missing", got)
	}
}

func TestPSGReferenceExtendedCasesProgramDocumentedHardwareFields(t *testing.T) {
	tests := []struct {
		name    string
		kind    Kind
		index   int
		address uintptr
		mask    uint16
		want    uint16
	}{
		{"CH1 sweep down", CH1, 5, hw.SOUND1CNT_L, 0x007F, 2<<4 | 1<<3 | 2},
		{"CH1 envelope up", CH1, 9, hw.SOUND1CNT_H, 0xFF00, 2<<12 | 1<<11 | 2<<8},
		{"CH2 length enable", CH2, 6, hw.SOUND2CNT_H, 1 << 14, 1 << 14},
		{"CH2 envelope up", CH2, 5, hw.SOUND2CNT_L, 0xFF00, 2<<12 | 1<<11 | 2<<8},
		{"CH3 bank 1", CH3, 1, hw.SOUND3CNT_L, 1 << 6, 1 << 6},
		{"CH3 75 percent", CH3, 4, hw.SOUND3CNT_H, 1 << 15, 1 << 15},
		{"CH3 length value", CH3, 8, hw.SOUND3CNT_H, 0x00FF, 192},
		{"CH3 length enable", CH3, 8, hw.SOUND3CNT_X, 1 << 14, 1 << 14},
		{"CH3 mute", CH3, 9, hw.SOUND3CNT_H, 0xE000, 0},
		{"CH4 divisor endpoints", CH4, 4, hw.SOUND4CNT_H, 0x00F7, 12<<4 | 7},
		{"CH4 envelope up", CH4, 6, hw.SOUND4CNT_L, 0xFF00, 2<<12 | 1<<11 | 2<<8},
		{"CH4 length enable", CH4, 7, hw.SOUND4CNT_H, 1 << 14, 1 << 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hw.ResetForTest()
			if !Start(tt.kind, tt.index) {
				t.Fatal("reference case rejected")
			}
			if got := hw.GetReg16(tt.address) & tt.mask; got != tt.want {
				t.Fatalf("register %#08x & %#04x = %#04x, want %#04x", tt.address, tt.mask, got, tt.want)
			}
		})
	}
}

func TestPSGReferenceRouteCasesAreHardPanned(t *testing.T) {
	tests := []struct {
		name       string
		kind       Kind
		index      int
		wantRoutes uint16
	}{
		{"CH1 left", CH1, 10, 1 << 12},
		{"CH1 right", CH1, 11, 1 << 8},
		{"CH2 left", CH2, 7, 1 << 13},
		{"CH2 right", CH2, 8, 1 << 9},
		{"CH4 left", CH4, 8, 1 << 15},
		{"CH4 right", CH4, 9, 1 << 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hw.ResetForTest()
			if !Start(tt.kind, tt.index) {
				t.Fatal("reference case rejected")
			}
			if got := hw.GetReg16(hw.SOUNDCNT_L) & 0xFF00; got != tt.wantRoutes {
				t.Fatalf("SOUNDCNT_L routes = %#04x, want %#04x", got, tt.wantRoutes)
			}
		})
	}
}

func TestFIFOReferencesProgramCorrectDMAAndTimer(t *testing.T) {
	hw.ResetForTest()
	if !Start(FIFOB, 2) {
		t.Fatal("FIFO B Timer1 case rejected")
	}
	if got := hw.GetReg16(hw.SOUNDCNT_H); got&fifoBTimer1 == 0 || got&fifoBRight == 0 || got&fifoBLeft != 0 {
		t.Fatalf("SOUNDCNT_H = %#04x, want FIFO B Timer1 right", got)
	}
	if got := hw.GetReg16(hw.DMA2CNT + 2); got != dmaFIFOControl {
		t.Fatalf("DMA2CNT_H = %#04x, want %#04x", got, dmaFIFOControl)
	}
	if got := hw.GetReg16(hw.TM1CNT_H); got != timerEnable {
		t.Fatalf("TM1CNT_H = %#04x", got)
	}
	if got := hw.GetReg16(hw.TM0CNT_H); got != 0 {
		t.Fatalf("TM0 unexpectedly active: %#04x", got)
	}
}

func TestFIFOStopCaseLeavesMasterAndBusDisabled(t *testing.T) {
	hw.ResetForTest()
	Start(FIFOA, 0)
	if !Start(FIFOA, 5) {
		t.Fatal("FIFO A stop case rejected")
	}
	if hw.GetReg16(hw.SOUNDCNT_X) != 0 || hw.GetReg16(hw.SOUNDCNT_H) != 0 ||
		hw.GetReg16(hw.DMA1CNT+2) != 0 || hw.GetReg16(hw.TM0CNT_H) != 0 {
		t.Fatal("FIFO stop/reset case left hardware active")
	}
}

func TestFIFOLevelCaseClearsFullVolumeBit(t *testing.T) {
	hw.ResetForTest()
	if !Start(FIFOA, 4) {
		t.Fatal("FIFO A 50% case rejected")
	}
	if got := hw.GetReg16(hw.SOUNDCNT_H); got&fifoAFull != 0 {
		t.Fatalf("SOUNDCNT_H = %#04x, FIFO A full-volume bit must be clear", got)
	}
	if got := hw.GetReg16(hw.SOUNDCNT_H); got&fifoBFull != 0 {
		t.Fatalf("SOUNDCNT_H = %#04x, inactive FIFO B level leaked", got)
	}
}

func TestCombinedReferencePreservesAllRoutesAndIndependentTimers(t *testing.T) {
	hw.ResetForTest()
	if !Start(All, 1) {
		t.Fatal("combined independent case rejected")
	}
	if got := hw.GetReg16(hw.SOUNDCNT_L); got != 0xFF77 {
		t.Fatalf("SOUNDCNT_L = %#04x, want every PSG route", got)
	}
	wantH := uint16(psgRatioFull | fifoAFull | fifoBFull | fifoALeft | fifoBRight | fifoBTimer1)
	if got := hw.GetReg16(hw.SOUNDCNT_H); got != wantH {
		t.Fatalf("SOUNDCNT_H = %#04x, want %#04x", got, wantH)
	}
	if hw.GetReg16(hw.TM0CNT_H) != timerEnable || hw.GetReg16(hw.TM1CNT_H) != timerEnable {
		t.Fatal("independent Timer0/Timer1 clocks not active")
	}
	if hw.GetReg16(hw.DMA1CNT+2) != dmaFIFOControl || hw.GetReg16(hw.DMA2CNT+2) != dmaFIFOControl {
		t.Fatal("both FIFO DMA channels must be active")
	}
}

func TestReferenceStopIsDeterministicAndRestartSafe(t *testing.T) {
	hw.ResetForTest()
	Start(All, 1)
	Stop()
	for _, addr := range []uintptr{
		hw.SOUNDCNT_L, hw.SOUNDCNT_H, hw.SOUNDCNT_X,
		hw.TM0CNT_L, hw.TM0CNT_H, hw.TM1CNT_L, hw.TM1CNT_H,
		hw.DMA1CNT + 2, hw.DMA2CNT + 2,
	} {
		if got := hw.GetReg16(addr); got != 0 {
			t.Fatalf("register %#08x = %#04x after stop", addr, got)
		}
	}
	if !Start(CH2, 0) || hw.GetReg16(hw.SOUND2CNT_H)&(1<<15) == 0 {
		t.Fatal("restart after complete stop failed")
	}
}
