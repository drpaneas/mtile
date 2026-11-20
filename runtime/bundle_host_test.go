//go:build !tinygo

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeSource(t *testing.T) {
	directory := t.TempDir()
	if err := MaterializeSource(directory); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"go.mod", "context.go", "audio/native_controller.go", "hw/psg.go", "crt0/gba.s", "crt0/gba.ld",
		"third_party/aurora/README.md", "third_party/aurora/COPYING", "third_party/aurora/pfunc_pal_Aurora11.lua",
	} {
		info, err := os.Stat(filepath.Join(directory, path))
		if err != nil || info.IsDir() {
			t.Fatalf("materialized %s: info=%v err=%v", path, info, err)
		}
	}
}

func TestGBAStackPointersEndDescendingStackReservations(t *testing.T) {
	directory := t.TempDir()
	if err := MaterializeSource(directory); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(directory, "crt0", "gba.ld"))
	if err != nil {
		t.Fatal(err)
	}
	linker := string(source)
	irqReserve := strings.Index(linker, ". += __stack_size_irq;")
	irqTop := strings.Index(linker, "_stack_top_irq = .;")
	userReserve := strings.Index(linker, ". += __stack_size_usr;")
	userTop := strings.Index(linker, "_stack_top = .;")
	if irqReserve < 0 || irqTop < 0 || userReserve < 0 || userTop < 0 ||
		(irqReserve >= irqTop || irqTop >= userReserve || userReserve >= userTop) {
		t.Fatalf("descending GBA stack reservations do not end at their stack-top symbols")
	}
}
