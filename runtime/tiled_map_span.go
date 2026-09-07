//go:build mtile_mode0 || mtile_tilecanvas

package runtime

import "unsafe"

// mode0CopyMapSpan copies one contiguous hardware screen-block row segment.
// Its caller has clipped the logical source rectangle and split at the 32-cell
// screen-block boundary. Validate the slice once, then filter and copy native
// halfwords rather than reloading Context and recomputing screen offsets for
// every cell. The destination is an EWRAM shadow, not visible VRAM.
//
//go:noinline
//go:section .iwram
func mode0CopyMapSpan(dst []byte, src []uint16, flags []byte, limit int, mask byte) {
	if len(src) == 0 || len(src) > len(dst)/2 {
		return
	}
	if limit < 0 {
		limit = 0
	}
	if mask != 0 && limit > len(flags) {
		limit = len(flags)
	}
	if littleEndianNumberWords && uintptr(unsafe.Pointer(&dst[0]))&1 == 0 {
		out := unsafe.Slice((*uint16)(unsafe.Pointer(&dst[0])), len(src))
		if mask == 0 {
			for i, id := range src {
				if int(id) >= limit {
					id = 0
				}
				out[i] = id
			}
		} else {
			for i, id := range src {
				if int(id) >= limit || flags[id]&mask != mask {
					id = 0
				}
				out[i] = id
			}
		}
		return
	}
	for i, id := range src {
		if int(id) >= limit || mask != 0 && flags[id]&mask != mask {
			id = 0
		}
		dst[i*2], dst[i*2+1] = byte(id), byte(id>>8)
	}
}
