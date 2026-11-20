package assetsync

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

// aurora.hex contains the RGB values from Richard “DawnBringer” Fhager's
// Aurora 1.1 palette. See runtime/third_party/aurora for its GPLv2 source and notice.
//
//go:embed aurora.hex
var auroraHex string

func canonicalPalette() Palette {
	colors, err := parseAuroraHex(auroraHex)
	if err != nil {
		panic(err)
	}
	return Palette{Name: CanonicalPaletteName, Colors: colors}
}

// CanonicalPaletteROM returns the exact BGR555 table built into the runtime.
// Callers use value equality, not the active palette's name, so an authored
// palette that happens to match Aurora can safely share the runtime copy too.
func CanonicalPaletteROM() [256]uint16 {
	colors, err := expandPalette(canonicalPalette())
	if err != nil {
		panic(err)
	}
	return colors
}

func parseAuroraHex(src string) ([]RGB15, error) {
	var colors []RGB15
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) != 6 {
			return nil, fmt.Errorf("aurora.hex line %d: want 6 hex digits, got %q", i+1, line)
		}
		n, err := strconv.ParseUint(line, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("aurora.hex line %d: %w", i+1, err)
		}
		r8 := int(n >> 16)
		g8 := int((n >> 8) & 0xff)
		b8 := int(n & 0xff)
		colors = append(colors, RGB15{R: r8 >> 3, G: g8 >> 3, B: b8 >> 3})
	}
	if len(colors) != 256 {
		return nil, fmt.Errorf("aurora.hex has %d colors (want 256)", len(colors))
	}
	return colors, nil
}
