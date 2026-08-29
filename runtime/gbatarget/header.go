// Package gbatarget prepares the cartridge startup assembly on the build host.
package gbatarget

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ValidLogo checks all 156 bytes used by the GBA boot ROM.
func ValidLogo(data []byte) bool {
	return len(data) == 156 && fmt.Sprintf("%x", sha256.Sum256(data)) == "08a0153cfd6b0ea54b938f7d209933fa849da0d56f5a34c481060c9ff2fad818"
}

// ToolchainLogo reads the header from the installed TinyGo GBA target.
func ToolchainLogo(root string) ([]byte, error) {
	path := filepath.Join(root, "targets", "gameboy-advance.s")
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	_, after, ok := strings.Cut(string(source), "// ROM header")
	block, _, end := strings.Cut(after, "// Game title")
	if !ok || !end {
		return nil, fmt.Errorf("%s: GBA header not found", path)
	}
	var logo []byte
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		values, ok := strings.CutPrefix(line, ".byte")
		if !ok {
			return nil, fmt.Errorf("%s: expected .byte in GBA header", path)
		}
		for _, value := range strings.Split(values, ",") {
			b, err := strconv.ParseUint(strings.TrimSpace(value), 0, 8)
			if err != nil {
				return nil, fmt.Errorf("%s: invalid GBA header byte: %w", path, err)
			}
			logo = append(logo, byte(b))
		}
	}
	if !ValidLogo(logo) {
		return nil, fmt.Errorf("%s: GBA header logo checksum mismatch", path)
	}
	return logo, nil
}

// Startup fills the placeholder in a runtime startup template. Older runtime
// checkouts and custom targets can already supply a complete startup.
func Startup(root string, template []byte) ([]byte, error) {
	const marker = `.error "generate the GBA target before assembling this startup"`
	if !bytes.Contains(template, []byte(marker)) {
		return template, nil
	}
	logo, err := ToolchainLogo(root)
	if err != nil {
		return nil, err
	}
	var asm strings.Builder
	asm.WriteString(".byte ")
	for i, b := range logo {
		if i > 0 {
			asm.WriteByte(',')
		}
		fmt.Fprintf(&asm, "0x%02x", b)
	}
	return bytes.Replace(template, []byte(marker), []byte(asm.String()), 1), nil
}
