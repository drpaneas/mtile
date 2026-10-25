//go:build tinygo && !mtile_mode0 && !mtile_tilecanvas

package runtime

import (
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

// The pages are the hardware Mode 4 frames; no EWRAM framebuffer is linked.
type mode4PageStorage struct{}

func (c *Context) mode4Page(index int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(hw.Page0+uintptr(index)*hw.PageStride)), FramebufferSize)
}

func copyMode4Page(dst, src []byte) {
	hw.CopyToVRAM32DMA(src, uintptr(unsafe.Pointer(&dst[0])), FramebufferSize/4)
}

func fillMode4Page(page []byte, value byte) {
	word := uint32(value) * 0x01010101
	hw.Fill32(page, word, FramebufferSize/4)
}
