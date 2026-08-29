package build

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

const (
	gbaHeaderSize = 0xc0
	maxGBAROMSize = 32 * 1024 * 1024
)

// ValidateROM checks the invariants that distinguish a linked GBA image from
// an arbitrary non-empty file. The first ARM branch is emitted by gba.s and
// must occupy the cartridge entry point; the Nintendo logo/header follow it.
func ValidateROM(rom []byte) error {
	if len(rom) < gbaHeaderSize {
		return fmt.Errorf("GBA ROM is %d bytes, need at least %d", len(rom), gbaHeaderSize)
	}
	if len(rom) > maxGBAROMSize {
		return fmt.Errorf("GBA ROM is %d bytes, exceeds 32 MiB cartridge space", len(rom))
	}
	if rom[3] != 0xea {
		return fmt.Errorf("GBA entry point is not an ARM branch (byte 3 is 0x%02x)", rom[3])
	}
	if fmt.Sprintf("%x", sha256.Sum256(rom[4:0xa0])) != "08a0153cfd6b0ea54b938f7d209933fa849da0d56f5a34c481060c9ff2fad818" {
		return errors.New("GBA Nintendo logo is missing or corrupt")
	}
	if rom[0xb2] != 0x96 {
		return fmt.Errorf("GBA header fixed value is 0x%02x, want 0x96", rom[0xb2])
	}
	if want := headerComplementCheck(rom); rom[0xbd] != want {
		return fmt.Errorf("GBA header complement check is 0x%02x, want 0x%02x", rom[0xbd], want)
	}
	return nil
}

// headerComplementCheck reproduces the BIOS boot check over the title/game
// code/maker code/fixed value/unit code/device type/reserved/version fields
// (0xa0..0xbc). gba.s hardcodes the resulting byte for its all-zero title
// block; computing it here instead of hardcoding a second copy means any
// future header field (e.g. a real game title) is caught by ValidateROM
// automatically rather than silently producing a ROM real hardware rejects.
func headerComplementCheck(rom []byte) byte {
	var sum byte
	for _, b := range rom[0xa0 : 0xbc+1] {
		sum -= b
	}
	return sum - 0x19
}
