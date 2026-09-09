//go:build tinygo && mtile_preview_log

package runtime

import (
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"
)

// DebugLog writes bounded, terminated messages to mGBA's debug registers.
// Keep this independent of TinyGo 0.42's mgbadebug runtime: that build tag
// omits zeroSizeAllocPtr and cannot compile with the GBA target. This path
// uses mTile's usual startup and reports mTile errors, not arbitrary Go panics.
func DebugLog(message string) {
	state := interrupt.Disable()
	enable := (*volatile.Register16)(unsafe.Pointer(uintptr(0x04FFF780)))
	flags := (*volatile.Register16)(unsafe.Pointer(uintptr(0x04FFF700)))
	buffer := (*[256]volatile.Register8)(unsafe.Pointer(uintptr(0x04FFF600)))
	enable.Set(0xC0DE)
	if enable.Get() != 0x1DEA {
		interrupt.Restore(state)
		return
	}
	index := 0
	for i := 0; i < len(message); i++ {
		ch := message[i]
		if ch == '\n' || index == 255 {
			buffer[index].Set(0)
			flags.Set(0x102) // warning, send bit set
			index = 0
			if ch == '\n' {
				continue
			}
		}
		buffer[index].Set(ch)
		index++
	}
	if index != 0 {
		buffer[index].Set(0)
		flags.Set(0x102)
	}
	interrupt.Restore(state)
}
