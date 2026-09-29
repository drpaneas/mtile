package assetsync

import (
	"fmt"
	"strconv"
	"strings"
)

// ObjectSheetROMDeclaration aligns immutable tile pixels for the ARM word-copy
// path. A plain byte array can land at an odd halfword after pose metadata,
// forcing every 64-byte tile copy through memcpy's byte loop. The zero-sized
// uint32 field guarantees alignment without padding the pixel slice or copying
// the bank into RAM. Both asset generators use this declaration.
func ObjectSheetROMDeclaration(pixels []byte) string {
	var b strings.Builder
	b.WriteString("//go:section .rodata\n")
	fmt.Fprintf(&b, "var objSheetROM = struct { _ [0]uint32; Pixels [%d]byte }{Pixels: [%d]byte{", len(pixels), len(pixels))
	for _, pixel := range pixels {
		b.WriteString(strconv.Itoa(int(pixel)))
		b.WriteByte(',')
	}
	b.WriteString("}}\n")
	return b.String()
}
