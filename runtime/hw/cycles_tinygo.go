//go:build tinygo

package hw

const (
	timerDisable       uint16 = 0
	timerEnable        uint16 = 0x0080
	timerCascade       uint16 = 0x0004
	timerEnableCascade        = timerEnable | timerCascade
)

// StartCycleCounter programs TM0+TM1 as a 32-bit F/1 cycle counter.
func StartCycleCounter() {
	SetReg16(TM0CNT_H, timerDisable)
	SetReg16(TM1CNT_H, timerDisable)
	SetReg16(TM0CNT_L, 0)
	SetReg16(TM1CNT_L, 0)
	SetReg16(TM1CNT_H, timerEnableCascade)
	SetReg16(TM0CNT_H, timerEnable)
}

// StopCycleCounter freezes TM0 then TM1 and returns the 32-bit count.
func StopCycleCounter() uint32 {
	SetReg16(TM0CNT_H, timerDisable)
	SetReg16(TM1CNT_H, timerDisable)
	low := GetReg16(TM0CNT_L)
	high := GetReg16(TM1CNT_L)
	return uint32(high)<<16 | uint32(low)
}

// StartFrameCycleCounter programs TM2+TM3 as a second 32-bit F/1 counter.
// TM0/TM1 remain available for shorter update/draw/blit measurements.
func StartFrameCycleCounter() {
	SetReg16(TM2CNT_H, timerDisable)
	SetReg16(TM3CNT_H, timerDisable)
	SetReg16(TM2CNT_L, 0)
	SetReg16(TM3CNT_L, 0)
	SetReg16(TM3CNT_H, timerEnableCascade)
	SetReg16(TM2CNT_H, timerEnable)
}

// StopFrameCycleCounter freezes TM2 then TM3 and returns the 32-bit count.
func StopFrameCycleCounter() uint32 {
	SetReg16(TM2CNT_H, timerDisable)
	SetReg16(TM3CNT_H, timerDisable)
	low := GetReg16(TM2CNT_L)
	high := GetReg16(TM3CNT_L)
	return uint32(high)<<16 | uint32(low)
}
