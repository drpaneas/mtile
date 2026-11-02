package assetsync

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Direct Sound is an opt-in companion asset, not a reinterpretation of the
// authored PSG v2 library. Public IDs stay stable across repeated imports.
type DirectAudioItem struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	BankID int    `json:"bankId"`
}
type DirectAudioManifest struct {
	Version int               `json:"version"`
	Format  int               `json:"format"`
	Bank    string            `json:"bank"`
	SHA256  string            `json:"sha256"`
	Music   []DirectAudioItem `json:"music"`
	Samples []DirectAudioItem `json:"samples"`
}
type DirectAudioAssets struct {
	Manifest DirectAudioManifest
	Bank     []byte
}

var directAudioName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func LoadDirectAudio(project string) (*DirectAudioAssets, error) {
	var manifest DirectAudioManifest
	present, err := decodeNativeFile(filepath.Join(project, "assets", "audio", "direct", "manifest.json"), "Direct Sound manifest", &manifest, true)
	if err != nil || !present {
		return nil, err
	}
	if manifest.Bank != "bank-"+manifest.SHA256+".bin" || len(manifest.SHA256) != 64 {
		return nil, errors.New("Direct Sound bank must use its SHA256 filename")
	}
	bank, err := os.ReadFile(filepath.Join(project, "assets", "audio", "direct", manifest.Bank))
	if err != nil {
		return nil, fmt.Errorf("read Direct Sound bank: %w", err)
	}
	if err := ValidateDirectAudio(manifest, bank); err != nil {
		return nil, err
	}
	return &DirectAudioAssets{manifest, bank}, nil
}

// ValidateDirectAudio verifies the immutable converter artifact before any C
// playback code sees it. Both current BlocksDS and devkitPro MAS layouts are
// selected explicitly; they must never be confused by the target player.
func ValidateDirectAudio(m DirectAudioManifest, bank []byte) error {
	if m.Version != 1 || (m.Format != 0x18 && m.Format != 0x19) {
		return errors.New("unsupported Direct Sound manifest or Maxmod format")
	}
	if len(bank) < 12 || len(bank) > 24<<20 || string(bank[4:12]) != "*maxmod*" {
		return errors.New("invalid or oversized GBA Maxmod soundbank (limit 24 MiB)")
	}
	hash := sha256.Sum256(bank)
	if hex.EncodeToString(hash[:]) != m.SHA256 {
		return errors.New("Direct Sound bank checksum mismatch; run mtile import-audio again")
	}
	ns, nm := int(binary.LittleEndian.Uint16(bank)), int(binary.LittleEndian.Uint16(bank[2:]))
	if nm > 256 || len(m.Music) != nm || len(m.Samples) > 256 || 12+4*(ns+nm) > len(bank) {
		return errors.New("invalid Direct Sound item counts")
	}
	for i := 0; i < ns+nm; i++ {
		off := int(binary.LittleEndian.Uint32(bank[12+4*i:]))
		if off%4 != 0 || off < 12+4*(ns+nm) || off > len(bank)-8 {
			return errors.New("invalid Maxmod item offset")
		}
		size := int(binary.LittleEndian.Uint32(bank[off:]))
		kind := byte(1)
		if i >= ns {
			kind = 0
		}
		if size < 8 || size > len(bank)-off || bank[off+4] != kind || int(bank[off+5]) != m.Format {
			return fmt.Errorf("invalid Maxmod item %d or incompatible converter format", i)
		}
	}
	for _, group := range []struct {
		items []DirectAudioItem
		limit int
	}{{m.Music, nm}, {m.Samples, ns}} {
		names := map[string]bool{}
		for _, item := range group.items {
			if !directAudioName.MatchString(item.Name) || names[item.Name] || item.BankID < 0 || item.BankID >= group.limit {
				return errors.New("invalid or duplicate Direct Sound name/ID")
			}
			if filepath.Base(item.Source) != item.Source || item.Source == "." {
				return errors.New("Direct Sound source must be one filename")
			}
			names[item.Name] = true
		}
	}
	return nil
}
