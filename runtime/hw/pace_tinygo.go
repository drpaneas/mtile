//go:build tinygo

package hw

// ReadFrameCycleCounter samples the running TM2+TM3 cascade without
// stopping it. The 30 Hz pacer polls this while the counter keeps
// counting from its anchor at the previous page flip.
//
// A cascade read races: TM2 can carry into TM3 between the two 16-bit
// reads. The high-low-high sequence retries when the high half moved,
// which can happen at most once per read.
func ReadFrameCycleCounter() uint32 {
	for {
		high := GetReg16(TM3CNT_L)
		low := GetReg16(TM2CNT_L)
		if GetReg16(TM3CNT_L) == high {
			return uint32(high)<<16 | uint32(low)
		}
	}
}

// PaceWaitUntil preserves duration waits for native callers that explicitly
// mask IRQs. Normal game pacing uses WaitFramePace and BIOS HALT instead.
func PaceWaitUntil(deadline uint32) {
	for ReadFrameCycleCounter() < deadline {
	}
}
