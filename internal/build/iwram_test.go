package build

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestBuildRejectsUnknownIWRAMFunctionBeforeToolchain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "placement")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	project, err := assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	project.IWRAMFunctions = []string{"missing"}
	if err := assetsync.SaveProject(dir, project); err != nil {
		t.Fatal(err)
	}
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "must-not-run"), RuntimeDir: t.TempDir()}
	if _, err := service.Build(dir, ""); err == nil || !strings.Contains(err.Error(), `iwramFunctions: unknown top-level function "missing"`) {
		t.Fatalf("build error = %v", err)
	}
}

func TestBuildEndToEndIWRAMFunctionPlacement(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "placement")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	project, err := assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	project.IWRAMFunctions = []string{"hot"}
	if err := assetsync.SaveProject(dir, project); err != nil {
		t.Fatal(err)
	}
	source := "counter=0\nfunction hot(v) return (v+1)%100 end\nfunction _update() counter=hot(counter) end\nfunction _draw() cls(counter) end\n"
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.KeepWork = true
	result, err := service.Build(dir, filepath.Join(t.TempDir(), "placement.gba"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(result.WorkDir) }()
	if !strings.Contains(result.GeneratedGo, "//go:noinline\n//go:section .iwram\nfunc (g *Game) hot(") {
		t.Fatal("build dropped project placement")
	}
	binary, err := elf.Open(filepath.Join(result.WorkDir, "game.elf"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = binary.Close() }()
	symbols, err := binary.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range symbols {
		if strings.HasSuffix(symbol.Name, ".hot") && strings.Contains(symbol.Name, "Game") {
			if symbol.Value < gbaIWRAMOrigin || symbol.Value >= gbaIWRAMEnd {
				t.Fatalf("selected helper linked at %#x", symbol.Value)
			}
			return
		}
	}
	t.Fatal("selected helper missing from linked ELF symbols")
}

func TestBuildPlacesSoftwareDivideInIWRAM(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "divide")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	source := "acc=1\nfunction _update()\n local d=3+(acc%5)\n acc=acc+(acc+7)/d\n acc=(acc+1)%7\nend\nfunction _draw() cls(acc) end\n"
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.KeepWork = true
	result, err := service.Build(dir, filepath.Join(t.TempDir(), "divide.gba"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(result.WorkDir) }()
	t.Logf("IWRAM %d/%d bytes", result.IWRAMUsed, IWRAMCapacity)
	binary, err := elf.Open(filepath.Join(result.WorkDir, "game.elf"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = binary.Close() }()
	symbols, err := binary.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]uint64{}
	for _, symbol := range symbols {
		switch symbol.Name {
		case "__aeabi_uidivmod", "__aeabi_uidiv", "__aeabi_idivmod", "__aeabi_idiv", "__aeabi_ldivmod", "__aeabi_uldivmod":
			found[symbol.Name] = symbol.Value
		}
	}
	t.Logf("divide symbols %v", found)
	address, ok := found["__aeabi_uidivmod"]
	if !ok {
		t.Fatal("__aeabi_uidivmod missing from linked ELF symbols")
	}
	if address < gbaIWRAMOrigin || address >= gbaIWRAMEnd {
		t.Fatalf("__aeabi_uidivmod linked at %#x, want IWRAM", address)
	}
	for _, name := range []string{"__aeabi_ldivmod", "__aeabi_uldivmod", "__aeabi_idivmod"} {
		address, ok := found[name]
		if !ok {
			continue
		}
		if address < gbaIWRAMOrigin || address >= gbaIWRAMEnd {
			t.Fatalf("%s linked at %#x, want IWRAM", name, address)
		}
	}
}
