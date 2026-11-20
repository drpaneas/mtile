package gbatarget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolchainLogoRejectsSameLengthSubstitute(t *testing.T) {
	root := t.TempDir()
	targets := filepath.Join(root, "targets")
	if err := os.Mkdir(targets, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "// ROM header\n.byte " + strings.Repeat("0,", 155) + "0\n// Game title\n"
	if err := os.WriteFile(filepath.Join(targets, "gameboy-advance.s"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ToolchainLogo(root); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("substituted header: %v", err)
	}
}
