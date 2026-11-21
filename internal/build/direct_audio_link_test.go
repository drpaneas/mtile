package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMaxmodBSSRelocationsIgnoreInputSectionOrder(t *testing.T) {
	tinygo, err := exec.LookPath("tinygo")
	if err != nil {
		t.Skip("requires TinyGo")
	}
	for _, tool := range []string{"clang", "ld.lld"} {
		if err := exec.Command(tinygo, tool, "--version").Run(); err != nil {
			t.Skipf("TinyGo has no integrated %s: %v", tool, err)
		}
	}

	// ThinLTO emits these Maxmod globals in different orders in a single
	// object. Keep the cartridge's references fixed while reversing only the
	// order of its input sections, so the regression fails deterministically.
	globals := []string{"mm_achannels", "mmLayerMain", "mm_pchannels", "mpp_channels", "mpp_vars"}
	references := ".section .init,\"ax\"\n.global _start\n_start: .word " + strings.Join(globals, ", ") + "\n"
	link := func() []byte {
		t.Helper()
		var source strings.Builder
		source.WriteString(references)
		for _, name := range globals {
			fmt.Fprintf(&source, ".section .bss.%s,\"aw\",%%nobits\n.balign 4\n%s: .space 4\n", name, name)
		}
		dir := t.TempDir()
		asm, obj, rom := filepath.Join(dir, "test.s"), filepath.Join(dir, "test.o"), filepath.Join(dir, "test.gba")
		if err := os.WriteFile(asm, []byte(source.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{
			{"clang", "--target=armv4t-none-eabi", "-c", asm, "-o", obj},
			{"ld.lld", "--gc-sections", "--oformat=binary", "-T", "../../runtime/crt0/gba.ld", "-o", rom, obj},
		} {
			if out, err := exec.Command(tinygo, args...).CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", args[0], err, out)
			}
		}
		data, err := os.ReadFile(rom)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 4*len(globals) {
			t.Fatalf("unexpected size for %d relocated addresses: %d bytes", len(globals), len(data))
		}
		return data
	}
	forward := link()
	slices.Reverse(globals)
	if reverse := link(); !bytes.Equal(forward, reverse) {
		t.Fatalf("Maxmod addresses depend on input section order: %x vs %x", forward, reverse)
	}
}
