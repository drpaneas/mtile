//go:build mtile_heapless && mtile_mode0

package runtime

import (
	"testing"
	"unsafe"
)

func TestHeaplessContextUsesCompactMetadata(t *testing.T) {
	if got := unsafe.Sizeof(Heap{}); got > 256 {
		t.Fatalf("heapless Heap metadata = %d bytes, want at most 256", got)
	}
	if got := unsafe.Sizeof(Context{}); got > 16*1024 {
		t.Fatalf("heapless Context = %d bytes, want at most 16 KiB", got)
	}
	if maxHandles != 1 || maxGlobals != 1 || maxLocals != 1 || maxTemps != 1 {
		t.Fatalf("heapless backing slots = handles:%d globals:%d locals:%d temps:%d", maxHandles, maxGlobals, maxLocals, maxTemps)
	}
}
