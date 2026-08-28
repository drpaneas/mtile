package hw

// SoundRoute is one physical GBA output routing choice.
type SoundRoute uint8

const (
	SoundRouteNone SoundRoute = iota
	SoundRouteRight
	SoundRouteLeft
	SoundRouteBoth
)

// PSGMixRatio is the legacy PSG contribution to the GBA hardware mixer.
type PSGMixRatio uint8

const (
	PSGMix25 PSGMixRatio = iota
	PSGMix50
	PSGMix100
)

// DirectSoundBus identifies one GBA Direct Sound FIFO.
type DirectSoundBus uint8

const (
	DirectSoundA DirectSoundBus = iota
	DirectSoundB
)

// DirectSoundTimer identifies the sample clock selected by a FIFO.
type DirectSoundTimer uint8

const (
	DirectSoundTimer0 DirectSoundTimer = iota
	DirectSoundTimer1
)

// DirectSoundLevel is the hardware level applied after one FIFO.
type DirectSoundLevel uint8

const (
	DirectSound50 DirectSoundLevel = iota
	DirectSound100
)

const (
	psgRightLevelMask uint16 = 0x0007
	psgLeftLevelMask  uint16 = 0x0070
	psgRouteMask      uint16 = 0xFF00

	psgRatioMask   uint16 = 0x0003
	fifoAFieldMask uint16 = soundAFIFOVolume | soundARight | soundALeft |
		1<<10 | soundAFIFOReset
	fifoBFieldMask uint16 = soundBFIFOVolume | soundBRight | soundBLeft |
		1<<14 | soundBFIFOReset
)

const (
	soundUserPSG uint8 = 1 << iota
	soundUserFIFOA
	soundUserFIFOB
)

// soundRegisterOwner is the single production owner of SOUNDCNT_L/H/X and
// SOUNDBIAS. The direct-register truth ROMs deliberately bypass it. All
// managed PSG and Direct Sound code mutates these shadows through the helpers
// below, so one subsystem can never erase another subsystem's fields.
var soundRegisters struct {
	initialized bool
	controlL    uint16
	controlH    uint16
	users       uint8
}

func initializeSoundRegisters() {
	if soundRegisters.initialized {
		return
	}
	soundRegisters.initialized = true
	soundRegisters.controlL = 0
	soundRegisters.controlH = 0
	soundRegisters.users = 0
	SetReg16(SOUNDCNT_X, 0)
	SetReg16(SOUNDCNT_L, 0)
	SetReg16(SOUNDCNT_H, 0)
	SetReg16(SOUNDBIAS, 0x0200)
}

func commitSoundL() { SetReg16(SOUNDCNT_L, soundRegisters.controlL) }
func commitSoundH() { SetReg16(SOUNDCNT_H, soundRegisters.controlH) }

func commitSoundMaster() {
	value := uint16(0)
	if soundRegisters.users != 0 {
		value = soundMasterEnable
	}
	SetReg16(SOUNDCNT_X, value)
}

func setSoundUser(user uint8, enabled bool) {
	if enabled {
		soundRegisters.users |= user
	} else {
		soundRegisters.users &^= user
	}
	commitSoundMaster()
}

// ConfigurePSGOutput sets the two PSG terminal levels and PSG/Direct-Sound
// mix ratio. It preserves every channel route and every FIFO field.
func ConfigurePSGOutput(leftLevel, rightLevel uint8, ratio PSGMixRatio) bool {
	if leftLevel > 7 || rightLevel > 7 || ratio > PSGMix100 {
		return false
	}
	initializeSoundRegisters()
	soundRegisters.controlL &^= psgLeftLevelMask | psgRightLevelMask
	soundRegisters.controlL |= uint16(leftLevel)<<4 | uint16(rightLevel)
	soundRegisters.controlH &^= psgRatioMask
	soundRegisters.controlH |= uint16(ratio)
	// SOUNDCNT_L is inside the PSG register window and is not writable on real
	// GBA hardware while SOUNDCNT_X.7 is clear. Keep the shadow now; the first
	// channel route commits the complete value after enabling the master.
	if soundRegisters.users != 0 {
		commitSoundL()
	}
	commitSoundH()
	return true
}

// RoutePSGChannel routes one fixed PSG channel without touching the other
// channels or Direct Sound. channel is 1 through 4.
func RoutePSGChannel(channel uint8, route SoundRoute) bool {
	if channel < 1 || channel > 4 || route > SoundRouteBoth {
		return false
	}
	initializeSoundRegisters()
	bit := uint16(1) << (channel - 1)
	soundRegisters.controlL &^= bit<<8 | bit<<12
	if route == SoundRouteRight || route == SoundRouteBoth {
		soundRegisters.controlL |= bit << 8
	}
	if route == SoundRouteLeft || route == SoundRouteBoth {
		soundRegisters.controlL |= bit << 12
	}
	commitSoundL()
	return true
}

// SetPSGActive controls the PSG claim on the global sound master. Disabling
// PSG cannot stop either Direct Sound FIFO.
func SetPSGActive(active bool) {
	initializeSoundRegisters()
	setSoundUser(soundUserPSG, active)
}

func fifoFields(bus DirectSoundBus) (mask, level, right, left, timer, reset uint16, user uint8, ok bool) {
	switch bus {
	case DirectSoundA:
		return fifoAFieldMask, soundAFIFOVolume, soundARight, soundALeft, 1 << 10, soundAFIFOReset, soundUserFIFOA, true
	case DirectSoundB:
		return fifoBFieldMask, soundBFIFOVolume, soundBRight, soundBLeft, 1 << 14, soundBFIFOReset, soundUserFIFOB, true
	default:
		return 0, 0, 0, 0, 0, 0, 0, false
	}
}

// ConfigureDirectSound configures one FIFO's level, timer, and terminal
// routing. The other FIFO and all PSG settings are preserved.
func ConfigureDirectSound(bus DirectSoundBus, route SoundRoute, level DirectSoundLevel, timer DirectSoundTimer) bool {
	mask, levelBit, rightBit, leftBit, timerBit, resetBit, user, ok := fifoFields(bus)
	if !ok || route > SoundRouteBoth || level > DirectSound100 || timer > DirectSoundTimer1 {
		return false
	}
	initializeSoundRegisters()
	value := uint16(0)
	if level == DirectSound100 {
		value |= levelBit
	}
	if route == SoundRouteRight || route == SoundRouteBoth {
		value |= rightBit
	}
	if route == SoundRouteLeft || route == SoundRouteBoth {
		value |= leftBit
	}
	if timer == DirectSoundTimer1 {
		value |= timerBit
	}
	soundRegisters.controlH = soundRegisters.controlH&^mask | value
	// FIFO reset is a write action, not retained state. Commit it once, then
	// restore the stable shadow so host tests match hardware readback.
	SetReg16(SOUNDCNT_H, soundRegisters.controlH|resetBit)
	commitSoundH()
	setSoundUser(user, route != SoundRouteNone)
	return true
}

// DisableDirectSound resets and releases exactly one FIFO.
func DisableDirectSound(bus DirectSoundBus) bool {
	mask, _, _, _, _, resetBit, user, ok := fifoFields(bus)
	if !ok {
		return false
	}
	initializeSoundRegisters()
	soundRegisters.controlH &^= mask
	SetReg16(SOUNDCNT_H, soundRegisters.controlH|resetBit)
	commitSoundH()
	setSoundUser(user, false)
	return true
}

// StopManagedSound returns every shared sound register to a safe baseline.
// It is the only production operation allowed to shut down all sound users.
func StopManagedSound() {
	initializeSoundRegisters()
	soundRegisters.controlL = 0
	soundRegisters.controlH = 0
	soundRegisters.users = 0
	SetReg16(SOUNDCNT_L, 0)
	SetReg16(SOUNDCNT_H, soundAFIFOReset|soundBFIFOReset)
	SetReg16(SOUNDCNT_H, 0)
	SetReg16(SOUNDCNT_X, 0)
	SetReg16(SOUNDBIAS, 0x0200)
}

// SoundControlSnapshot exposes register-equivalent managed state for the
// editor's GBA view and host conformance tests.
type SoundControlSnapshot struct {
	ControlL uint16
	ControlH uint16
	Master   bool
	PSG      bool
	FIFOA    bool
	FIFOB    bool
}

func ManagedSoundControlSnapshot() SoundControlSnapshot {
	initializeSoundRegisters()
	return SoundControlSnapshot{
		ControlL: soundRegisters.controlL,
		ControlH: soundRegisters.controlH,
		Master:   soundRegisters.users != 0,
		PSG:      soundRegisters.users&soundUserPSG != 0,
		FIFOA:    soundRegisters.users&soundUserFIFOA != 0,
		FIFOB:    soundRegisters.users&soundUserFIFOB != 0,
	}
}

func resetManagedSoundForTest() {
	soundRegisters.initialized = false
	soundRegisters.controlL = 0
	soundRegisters.controlH = 0
	soundRegisters.users = 0
}
