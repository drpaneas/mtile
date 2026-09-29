package build

import (
	"github.com/drpaneas/mtile/runtime/gbatarget"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
	"github.com/drpaneas/mtile/runtime/audio"
)

func TestGenerateTargetRemovesInheritanceAndUsesProjectStartup(t *testing.T) {
	tinygoRoot := t.TempDir()
	runtimeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tinygoRoot, "targets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tinygoRoot, "src", "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runtimeDir, "crt0"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := `{"inherits":"gameboy-advance","extra-files":["stock.s"],"gc":"conservative"}`
	if err := os.WriteFile(filepath.Join(tinygoRoot, "targets", "gameboy-advance.json"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(tinygoRoot, "src", "runtime", "asm_arm.S"),
		filepath.Join(runtimeDir, "crt0", "gba.s"),
		filepath.Join(runtimeDir, "crt0", "gba.ld"),
	} {
		if err := os.WriteFile(path, []byte("input"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(t.TempDir(), "target-gba.json")
	if err := GenerateTarget(tinygoRoot, runtimeDir, output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := targetHasNoInheritedStartup(data); err != nil {
		t.Fatal(err)
	}
	var target map[string]any
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatal(err)
	}
	if target["gc"] != "none" {
		t.Fatalf("gc = %v, want none", target["gc"])
	}
	if target["linkerscript"] != filepath.Join(runtimeDir, "crt0", "gba.ld") {
		t.Fatalf("linkerscript = %v", target["linkerscript"])
	}
	files, ok := target["extra-files"].([]any)
	if !ok || len(files) != 2 {
		t.Fatalf("extra-files = %#v", target["extra-files"])
	}
	wantStartup, _ := filepath.Rel(tinygoRoot, filepath.Join(runtimeDir, "crt0", "gba.s"))
	if files[0] != filepath.ToSlash(wantStartup) || files[1] != "src/runtime/asm_arm.S" {
		t.Fatalf("extra-files = %#v, want [%q %q]", files, filepath.ToSlash(wantStartup), "src/runtime/asm_arm.S")
	}
}

func TestGenerateSmokeTargetUsesSmokeStartup(t *testing.T) {
	tinygoRoot := t.TempDir()
	runtimeDir := t.TempDir()
	for _, dir := range []string{
		filepath.Join(tinygoRoot, "targets"),
		filepath.Join(tinygoRoot, "src", "runtime"),
		filepath.Join(runtimeDir, "crt0"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tinygoRoot, "targets", "gameboy-advance.json"), []byte(`{"inherits":"stock"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(tinygoRoot, "src", "runtime", "asm_arm.S"),
		filepath.Join(runtimeDir, "crt0", "gba-smoke.s"),
		filepath.Join(runtimeDir, "crt0", "gba.ld"),
	} {
		if err := os.WriteFile(path, []byte("input"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(t.TempDir(), "target-gba-smoke.json")
	if err := GenerateSmokeTarget(tinygoRoot, runtimeDir, output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var target map[string]any
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatal(err)
	}
	files := target["extra-files"].([]any)
	want, _ := filepath.Rel(tinygoRoot, filepath.Join(runtimeDir, "crt0", "gba-smoke.s"))
	if files[0] != filepath.ToSlash(want) {
		t.Fatalf("smoke extra-files = %#v, want %q", files, filepath.ToSlash(want))
	}
}

func TestGenerateTargetRejectsMissingBaseTarget(t *testing.T) {
	tinygoRoot := t.TempDir()
	runtimeDir := t.TempDir()
	if err := GenerateTarget(tinygoRoot, runtimeDir, filepath.Join(t.TempDir(), "target.json")); err == nil {
		t.Fatal("expected an error when TinyGo's own gameboy-advance.json target is missing")
	}
}

func TestGenerateTargetRejectsMissingRuntimeInput(t *testing.T) {
	tinygoRoot := t.TempDir()
	runtimeDir := t.TempDir()
	for _, dir := range []string{
		filepath.Join(tinygoRoot, "targets"),
		filepath.Join(tinygoRoot, "src", "runtime"),
		filepath.Join(runtimeDir, "crt0"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tinygoRoot, "targets", "gameboy-advance.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tinygoRoot, "src", "runtime", "asm_arm.S"), []byte("input"), 0o644); err != nil {
		t.Fatal(err)
	}
	// gba.s and gba.ld are intentionally left unwritten in runtimeDir/crt0.
	if err := GenerateTarget(tinygoRoot, runtimeDir, filepath.Join(t.TempDir(), "target.json")); err == nil {
		t.Fatal("expected an error when the runtime crt0 inputs are missing")
	}
}

func TestAtomicWriteRejectsUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}
	parent := t.TempDir()
	locked := filepath.Join(parent, "locked")
	if err := os.MkdirAll(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	if err := atomicWrite(filepath.Join(locked, "out.json"), []byte("data"), 0o644); err == nil {
		t.Fatal("expected atomicWrite to fail creating a temp file in an unwritable directory")
	}
}

func TestGenerateTargetRejectsJSONNull(t *testing.T) {
	tinygoRoot := t.TempDir()
	runtimeDir := t.TempDir()
	for _, dir := range []string{
		filepath.Join(tinygoRoot, "targets"),
		filepath.Join(tinygoRoot, "src", "runtime"),
		filepath.Join(runtimeDir, "crt0"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tinygoRoot, "targets", "gameboy-advance.json"), []byte("null"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(tinygoRoot, "src", "runtime", "asm_arm.S"),
		filepath.Join(runtimeDir, "crt0", "gba.s"),
		filepath.Join(runtimeDir, "crt0", "gba.ld"),
	} {
		if err := os.WriteFile(path, []byte("input"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := GenerateTarget(tinygoRoot, runtimeDir, filepath.Join(t.TempDir(), "target.json")); err == nil {
		t.Fatal("expected a JSON-null target to be rejected")
	}
}

func TestParseLinkerMapAndHeapGate(t *testing.T) {
	mapFile := filepath.Join(t.TempDir(), "game.map")
	content := `Address Size Align Out In Symbol
 2000000 2000000 0 1 _ewram_end = .
 2000000 2000000 0 1 _heap_start = .
 203fe00 203fe00 0 1 _heap_end = ORIGIN(ewram) + LENGTH(ewram)
 3001200 3001200 0 1 _ebss = .
`
	if err := os.WriteFile(mapFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	symbols, err := ParseLinkerMap(mapFile)
	if err != nil {
		t.Fatal(err)
	}
	if symbols.EWRAMEnd != 0x02000000 || symbols.HeapEnd != 0x0203fe00 || symbols.IWRAMEnd != 0x03001200 {
		t.Fatalf("symbols = %#v", symbols)
	}
	if err := ValidateHeapBudget(symbols); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHeapBudget(LinkerSymbols{EWRAMEnd: 0x0203f100, HeapEnd: 0x02040000}); err == nil {
		t.Fatal("expected reserve failure")
	}
}

func TestGBAMemoryCapacitiesMatchReservedLinkerRegions(t *testing.T) {
	if EWRAMCapacity != 256*1024-512 {
		t.Fatalf("EWRAM capacity = %d", EWRAMCapacity)
	}
	if IWRAMCapacity != 32*1024-96 {
		t.Fatalf("IWRAM capacity = %d", IWRAMCapacity)
	}
}

func TestParseLinkerMapAcceptsHexPrefixedAddresses(t *testing.T) {
	mapFile := filepath.Join(t.TempDir(), "game.map")
	content := `Address Size Align Out In Symbol
 0x2000000 2000000 0 1 _ewram_end = .
 0x203fe00 203fe00 0 1 _heap_end = .
`
	if err := os.WriteFile(mapFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	symbols, err := ParseLinkerMap(mapFile)
	if err != nil {
		t.Fatal(err)
	}
	if symbols.EWRAMEnd != 0x02000000 || symbols.HeapEnd != 0x0203fe00 {
		t.Fatalf("symbols = %#v", symbols)
	}
}

func TestParseMapAddressRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"", "0x", "0X", "zz", "-1"} {
		if _, ok := parseMapAddress(value); ok {
			t.Fatalf("parseMapAddress(%q) = ok, want rejected", value)
		}
	}
}

func TestTargetHasNoInheritedStartupRejectsBadJSON(t *testing.T) {
	if err := targetHasNoInheritedStartup([]byte("not json")); err == nil {
		t.Fatal("expected a decode error for malformed JSON")
	}
}

func TestTargetHasNoInheritedStartupRejectsJSONNull(t *testing.T) {
	if err := targetHasNoInheritedStartup([]byte("null")); err == nil {
		t.Fatal("expected JSON null to be rejected as a target object")
	}
}

func TestTargetHasNoInheritedStartupRejectsInherits(t *testing.T) {
	if err := targetHasNoInheritedStartup([]byte(`{"inherits":"gameboy-advance"}`)); err == nil {
		t.Fatal("expected an error when inherits is still present")
	}
}

func TestDiscoverTinyGoRootRejectsFailingBinary(t *testing.T) {
	if _, err := discoverTinyGoRoot(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected an error for a nonexistent tinygo binary")
	}
}

func TestDiscoverTinyGoRootRejectsEmptyOutput(t *testing.T) {
	fakeTinygo := filepath.Join(t.TempDir(), "tinygo")
	if err := os.WriteFile(fakeTinygo, []byte("#!/bin/sh\nprintf ''\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverTinyGoRoot(fakeTinygo); err == nil {
		t.Fatal("expected an error when tinygo env TINYGOROOT prints nothing")
	}
}

func TestMode0BuildTagsKeepObjectBackendDependencies(t *testing.T) {
	tests := []struct {
		name   string
		report transpiler.RendererReport
		want   string
	}{
		{name: "empty", report: transpiler.RendererReport{Mode0Layers: 0}, want: "mtile_mode0,mtile_mode0_layers0"},
		{name: "text", report: transpiler.RendererReport{Mode0Layers: 1, Mode0Text: true}, want: "mtile_mode0,mtile_mode0_obj,mtile_mode0_text"},
		{name: "packing", report: transpiler.RendererReport{Mode0Layers: 1, Mode0ObjectPacking: true}, want: "mtile_mode0,mtile_mode0_obj,mtile_mode0_objpack"},
		{name: "pose streaming", report: transpiler.RendererReport{Mode0Layers: 1, Mode0PoseStreaming: true}, want: "mtile_mode0,mtile_mode0_obj,mtile_mode0_pspr"},
		{name: "resident poses", report: transpiler.RendererReport{Mode0Layers: 1, Mode0PoseResident: true}, want: "mtile_mode0,mtile_mode0_obj,mtile_mode0_pspr"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mode0BuildTags(test.report); got != test.want {
				t.Fatalf("tags = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRuntimeBuildTagsSpecializeMapStorageForBothRenderers(t *testing.T) {
	tests := []struct {
		name   string
		report transpiler.RendererReport
		want   string
	}{
		{name: "mode4-compact", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, HeapLinked: true}, want: "mtile_map_compact,mtile_static_strings_1"},
		{name: "mode4-raster-disabled", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, MapRasterDisabled: true, HeapLinked: true}, want: "mtile_map_compact,mtile_no_map_raster,mtile_static_strings_1"},
		{name: "mode4-streaming-raster-disabled", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, MapStreaming: true, MapRasterDisabled: true, HeapLinked: true}, want: "mtile_no_map_raster,mtile_static_strings_1"},
		{name: "mode0-raster-disabled", report: transpiler.RendererReport{Selected: transpiler.RendererMode0, Mode0Layers: 1, MapRasterDisabled: true, HeapLinked: true}, want: "mtile_mode0,mtile_map_compact,mtile_static_strings_1"},
		{name: "mode4-streaming", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, MapStreaming: true, HeapLinked: true}, want: "mtile_static_strings_1"},
		{name: "mode0-compact", report: transpiler.RendererReport{Selected: transpiler.RendererMode0, Mode0Layers: 1, HeapLinked: true}, want: "mtile_mode0,mtile_map_compact,mtile_static_strings_1"},
		{name: "mode0-streaming", report: transpiler.RendererReport{Selected: transpiler.RendererMode0, Mode0Layers: 1, MapStreaming: true, HeapLinked: true}, want: "mtile_mode0,mtile_map_streaming,mtile_static_strings_1"},
		{name: "mode4-heapless", report: transpiler.RendererReport{Selected: transpiler.RendererMode4}, want: "mtile_map_compact,mtile_heapless,mtile_static_strings_1"},
		{name: "mode0-heapless", report: transpiler.RendererReport{Selected: transpiler.RendererMode0, Mode0Layers: 1}, want: "mtile_mode0,mtile_map_compact,mtile_heapless,mtile_static_strings_1"},
		{name: "mode0-readonly-sheet-upload", report: transpiler.RendererReport{Selected: transpiler.RendererMode0, Mode0Layers: 1, HeapLinked: true, SheetReadOnly: true}, want: "mtile_mode0,mtile_map_compact,mtile_mode0_sheet_immutable,mtile_static_strings_1"},
		{name: "mode4-readonly-sheet", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, HeapLinked: true, SheetReadOnly: true}, want: "mtile_map_compact,mtile_sheet_readonly,mtile_static_strings_1"},
		{name: "static-tier-8", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, HeapLinked: true, StaticStrings: 8}, want: "mtile_map_compact,mtile_static_strings_8"},
		{name: "static-tier-32", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, HeapLinked: true, StaticStrings: 9}, want: "mtile_map_compact,mtile_static_strings_32"},
		{name: "static-tier-128", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, HeapLinked: true, StaticStrings: 33}, want: "mtile_map_compact,mtile_static_strings_128"},
		{name: "static-tier-default", report: transpiler.RendererReport{Selected: transpiler.RendererMode4, HeapLinked: true, StaticStrings: 129}, want: "mtile_map_compact"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeBuildTags(test.report); got != test.want {
				t.Fatalf("tags = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseLinkerMapMissingFile(t *testing.T) {
	if _, err := ParseLinkerMap(filepath.Join(t.TempDir(), "missing.map")); err == nil {
		t.Fatal("expected an error reading a nonexistent linker map")
	}
}

func TestParseLinkerMapReportsMissingSymbols(t *testing.T) {
	mapFile := filepath.Join(t.TempDir(), "game.map")
	if err := os.WriteFile(mapFile, []byte("Address Size Align Out In Symbol\n 1000 1000 0 1 unrelated_symbol\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseLinkerMap(mapFile)
	if err == nil {
		t.Fatal("expected an error for a map missing both boundary symbols")
	}
	if !strings.Contains(err.Error(), "_ewram_end") || !strings.Contains(err.Error(), "_heap_end") {
		t.Fatalf("error = %v, want it to name both missing symbols", err)
	}
}

func TestValidateHeapBudgetRejectsHeapEndBeforeEWRAMEnd(t *testing.T) {
	err := ValidateHeapBudget(LinkerSymbols{EWRAMEnd: 0x02040000, HeapEnd: 0x02000000})
	if err == nil {
		t.Fatal("expected an error when heap end precedes EWRAM end")
	}
}

func TestValidateHeapBudgetRejectsReservedDebugMailbox(t *testing.T) {
	err := ValidateHeapBudget(LinkerSymbols{EWRAMEnd: 0x0203f000, HeapEnd: 0x02040000})
	if err == nil || !strings.Contains(err.Error(), "exceeds application EWRAM") {
		t.Fatalf("reserved mailbox validation = %v", err)
	}
}

func TestAddAssetBytesRejectsUint64Overflow(t *testing.T) {
	if _, err := addAssetBytes(^uint64(0)-1, 2, "test"); err == nil {
		t.Fatal("expected asset payload addition to reject uint64 overflow")
	}
}

func TestSetEnvReplacesExistingEntry(t *testing.T) {
	env := []string{"PATH=/bin", "GOWORK=/wrong", "LANG=C", "GOWORK=/also-wrong"}
	got := setEnv(env, "GOWORK", "off")
	if len(got) != len(env)-1 || got[1] != "GOWORK=off" {
		t.Fatalf("setEnv = %v, want duplicate GOWORK entries collapsed", got)
	}
	got = setEnv(got, "GOFLAGS", "-trimpath")
	if got[len(got)-1] != "GOFLAGS=-trimpath" {
		t.Fatalf("setEnv append = %v", got)
	}
}

func validGBAHeader() []byte {
	rom := make([]byte, 0xc0)
	rom[3] = 0xea
	command := exec.Command("tinygo", "env", "TINYGOROOT")
	output, err := command.Output()
	if err == nil {
		logo, err := gbatarget.ToolchainLogo(strings.TrimSpace(string(output)))
		if err == nil {
			copy(rom[4:0xa0], logo)
		}
	}
	rom[0xb2] = 0x96
	rom[0xbd] = headerComplementCheck(rom)
	return rom
}

func TestValidateROM(t *testing.T) {
	if err := ValidateROM(validGBAHeader()); err != nil {
		t.Fatal(err)
	}
	rom := validGBAHeader()
	rom[3] = 0
	if err := ValidateROM(rom); err == nil {
		t.Fatal("expected invalid entry point")
	}
}

func TestValidateROMRejectsTooShortInput(t *testing.T) {
	if err := ValidateROM(validGBAHeader()[:0xc0-1]); err == nil {
		t.Fatal("expected a ROM shorter than the header to be rejected")
	}
	if err := ValidateROM(nil); err == nil {
		t.Fatal("expected a nil ROM to be rejected")
	}
}

func TestValidateROMRejectsCorruptLogo(t *testing.T) {
	rom := validGBAHeader()
	rom[0x9f] ^= 0xff
	if err := ValidateROM(rom); err == nil {
		t.Fatal("expected a corrupt Nintendo logo to be rejected")
	}
}

func TestValidateROMRejectsWrongFixedHeaderValue(t *testing.T) {
	rom := validGBAHeader()
	rom[0xb2] = 0
	if err := ValidateROM(rom); err == nil {
		t.Fatal("expected the GBA fixed header value to be checked")
	}
}

func TestValidateROMRejectsWrongComplementCheck(t *testing.T) {
	rom := validGBAHeader()
	rom[0xbd] ^= 0xff
	if err := ValidateROM(rom); err == nil {
		t.Fatal("expected the GBA header complement check to be validated")
	}
}

// TestHeaderComplementCheckMatchesHandAssembledConstant guards against
// headerComplementCheck and gba.s's hardcoded 0x51 byte silently drifting
// apart: both must agree for the all-zero title/game-code/maker-code block
// crt0 actually ships, since gba.s never recomputes this byte itself.
func TestHeaderComplementCheckMatchesHandAssembledConstant(t *testing.T) {
	rom := make([]byte, 0xc0)
	rom[0xb2] = 0x96
	if got := headerComplementCheck(rom); got != 0x51 {
		t.Fatalf("headerComplementCheck(all-zero title block) = 0x%02x, want 0x51 (gba.s's hardcoded value)", got)
	}
}

func TestValidateROMRejectsOversizedCartridge(t *testing.T) {
	rom := append(validGBAHeader(), make([]byte, maxGBAROMSize)...)
	if err := ValidateROM(rom); err == nil {
		t.Fatal("expected a ROM larger than 32 MiB to be rejected")
	}
}

// TestCreateProjectRejectsInvalidDimensions covers negative dimensions only:
// zero is a sentinel. Sheet 0 becomes 240x160; map 0 becomes 30x20 when the
// sheet has at least one complete 8x8 sprite. Those zeros therefore never
// reach the "must be positive" check below.
func TestCreateProjectRejectsInvalidDimensions(t *testing.T) {
	cases := []NewProjectOptions{
		{SheetWidth: -1, SheetHeight: 16, MapWidth: 1, MapHeight: 1},
		{SheetWidth: 16, SheetHeight: -1, MapWidth: 1, MapHeight: 1},
		{SheetWidth: 16, SheetHeight: 16, MapWidth: -1, MapHeight: 1},
		{SheetWidth: 16, SheetHeight: 16, MapWidth: 1, MapHeight: -1},
	}
	for i, options := range cases {
		if err := CreateProject(filepath.Join(t.TempDir(), "p"), options); err == nil {
			t.Fatalf("case %d: expected an error for %+v", i, options)
		}
	}
}

func TestCreateProjectRejectsOverflowingDimensions(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	options := NewProjectOptions{SheetWidth: maxInt, SheetHeight: 2, MapWidth: 1, MapHeight: 1}
	if err := CreateProject(filepath.Join(t.TempDir(), "p"), options); err == nil {
		t.Fatal("expected an overflow error for sheet dimensions")
	}
	options = NewProjectOptions{SheetWidth: 16, SheetHeight: 16, MapWidth: maxInt, MapHeight: 2}
	if err := CreateProject(filepath.Join(t.TempDir(), "p"), options); err == nil {
		t.Fatal("expected an overflow error for map dimensions")
	}
}

func TestCreateProjectRejectsOversizedAssetPayloadBeforeAllocation(t *testing.T) {
	options := NewProjectOptions{SheetWidth: 240, SheetHeight: 160, MapWidth: 4096, MapHeight: 4096}
	if err := CreateProject(filepath.Join(t.TempDir(), "too-large"), options); err == nil {
		t.Fatal("expected an oversized GBA asset payload to be rejected before map allocation")
	}
}

func TestCreateProjectRejectsDanglingDestinationSymlink(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "project")
	if err := os.Symlink(filepath.Join(parent, "missing-target"), destination); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := CreateProject(destination, NewProjectOptions{}); err == nil {
		t.Fatal("expected an existing dangling symlink to be rejected")
	}
}

func TestNewServiceRejectsMissingRuntimeDir(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if _, err := NewService("", t.TempDir()); err == nil {
		t.Fatal("expected an error for a runtime directory without crt0/gba.s")
	}
}

func TestNewServiceRejectsRuntimeDirectoryInput(t *testing.T) {
	runtimeDir := t.TempDir()
	crt0 := filepath.Join(runtimeDir, "crt0")
	if err := os.MkdirAll(filepath.Join(crt0, "gba.s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crt0, "gba.ld"), []byte("input"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(filepath.Join(t.TempDir(), "tinygo"), runtimeDir); err == nil {
		t.Fatal("expected a runtime directory used as gba.s to be rejected")
	}
}

func TestNewServiceRejectsMissingTinyGoOnPATH(t *testing.T) {
	t.Setenv("PATH", "")
	if _, err := NewService("", ""); err == nil {
		t.Fatal("expected an error when tinygo is not on PATH")
	}
}

func TestCreateProjectUsesNativeDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hello")
	if err := CreateProject(dir, NewProjectOptions{}); err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.SheetW != 240 || assets.SheetH != 160 || assets.MapW != 30 || assets.MapH != 20 || assets.SpriteN != 600 {
		t.Fatalf("defaults = %dx%d map %dx%d sprites %d", assets.SheetW, assets.SheetH, assets.MapW, assets.MapH, assets.SpriteN)
	}
	if assets.AudioVersion != assetsync.NativeAudioSchemaVersion || assets.NativeAudio == nil {
		t.Fatalf("default audio = version %d native=%#v", assets.AudioVersion, assets.NativeAudio)
	}
	if assets.Project.Renderer != assetsync.NewProjectRendererName {
		t.Fatalf("new-project renderer = %q, want %q", assets.Project.Renderer, assetsync.NewProjectRendererName)
	}
	if assets.NativeAudio.Settings != assetsync.DefaultNativeAudioSettings() ||
		len(assets.NativeAudio.Library.Sounds) != 0 || len(assets.NativeAudio.Library.Clips) != 0 ||
		len(assets.NativeAudio.Library.Parts) != 0 || len(assets.NativeAudio.Library.Songs) != 0 {
		t.Fatalf("default native audio = %#v", assets.NativeAudio)
	}
	for _, name := range []string{"settings.json", "sounds.json", "music.json"} {
		if _, err := os.Stat(filepath.Join(dir, "assets", "audio", name)); err != nil {
			t.Fatalf("new project %s: %v", name, err)
		}
	}
	for _, name := range []string{"sfx.json", filepath.Join("samples", "manifest.json")} {
		if _, err := os.Stat(filepath.Join(dir, "assets", "audio", name)); !os.IsNotExist(err) {
			t.Fatalf("new project unexpectedly contains superseded %s: %v", name, err)
		}
	}
	game, err := os.ReadFile(filepath.Join(dir, GameFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(game), "function _draw()") {
		t.Fatalf("default game is missing _draw: %s", game)
	}
	requested, err := transpiler.ParseRenderer(assets.Project.Renderer)
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := transpiler.TranspileWithReport(GameFileName, string(game), transpiler.Options{
		Renderer: requested,
		SheetW:   assets.SheetW,
		SheetH:   assets.SheetH,
		MapW:     assets.MapW,
		MapH:     assets.MapH,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || report.Mode0Layers != 0 || !report.Mode0Objects || !report.Mode0Text {
		t.Fatalf("new-project renderer report = %+v, want zero-BG Mode 0 text", report)
	}
	if err := CreateProject(dir, NewProjectOptions{}); err == nil {
		t.Fatal("expected existing project refusal")
	}
}

func TestCompiledAssetFootprintReportsNativeAudioROMRecords(t *testing.T) {
	assets := &assetsync.Assets{
		SheetROM: make([]byte, 64), MapROM: make([]uint16, 2), FlagROM: make([]byte, 1),
		NativeAudio: &assetsync.NativeAudioAssets{Library: audio.Library{
			Sounds: make([]audio.Sound, 1), Clips: []audio.Clip{{NoteEvents: make([]audio.NoteEvent, 2)}},
			Parts: make([]audio.Part, 1), Songs: []audio.Song{{Sequence: []uint8{0}}},
		}},
	}
	got, err := validateCompiledAssetFootprint(assets)
	if err != nil {
		t.Fatal(err)
	}
	if want := 100 + 16 + 2*12 + 16 + 16 + 1; got != want {
		t.Fatalf("audio ROM bytes = %d, want %d", got, want)
	}
}

func TestBuildRejectsSupersededV1AudioBeforeToolchain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "invalid-sfx")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "audio", "sfx.json")
	invalid := `{"version":1,"sfx":[{"id":0,"name":"bad","speed":8,"steps":[{"pitch":57,"waveform":"triangle","volume":7,"effect":"bogus"}]}]}`
	if err := os.WriteFile(path, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "must-not-run"), RuntimeDir: t.TempDir()}
	if _, err := service.Build(dir, ""); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("Build error = %v, want pre-toolchain legacy audio rejection", err)
	}
}

func TestBuildForcedMode0RejectsIncompatibleSourceBeforeToolchain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mode0-incompatible")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	source := "function _draw()\n  cls()\n  map()\n  pset(1, 2, 3)\nend\n"
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{
		TinyGoBin: filepath.Join(t.TempDir(), "must-not-run"), RuntimeDir: t.TempDir(),
		Renderer: transpiler.RendererMode0,
	}
	_, err := service.Build(dir, "")
	if err == nil || !strings.Contains(err.Error(), "M0_MODE4_BUILTIN") || !strings.Contains(err.Error(), "game.lua:4") {
		t.Fatalf("Build error = %v, want source-mapped Mode 0 rejection", err)
	}
}

func TestServiceResolvesProjectRendererUnlessExplicitlyOverridden(t *testing.T) {
	project := assetsync.Project{Renderer: "auto"}
	service := Service{Renderer: transpiler.RendererMode0}
	got, err := service.resolveRenderer(project)
	if err != nil || got != transpiler.RendererMode0 {
		t.Fatalf("explicit renderer = %v, %v", got, err)
	}
	service.UseProjectRenderer = true
	got, err = service.resolveRenderer(project)
	if err != nil || got != transpiler.RendererAuto {
		t.Fatalf("project renderer = %v, %v", got, err)
	}
	project.Renderer = "adaptive"
	if _, err := service.resolveRenderer(project); err == nil || !strings.Contains(err.Error(), "project renderer") {
		t.Fatalf("invalid project renderer error = %v", err)
	}
}

func TestBuildRejectsExplicitV1ProjectBeforeToolchain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v1-project")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	settings := `{"version":1,"masterVolume":256}`
	if err := os.WriteFile(filepath.Join(dir, "assets", "audio", "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "must-not-run"), RuntimeDir: t.TempDir()}
	if _, err := service.Build(dir, ""); err == nil || !strings.Contains(err.Error(), "audio version 1 is superseded") {
		t.Fatalf("Build error = %v, want explicit v1 rejection", err)
	}
}

func TestBuildRejectsInvalidMusicBeforeToolchain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "invalid-music")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	sounds := nativeCH1Sounds()
	if err := assetsync.SaveNativeSounds(dir, &sounds); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "audio", "music.json")
	invalid := `{"version":2,"ppqn":96,"clips":[{"id":0,"name":"bad","kind":"ch1","lengthTicks":96,"events":[{"tick":0,"durationTicks":24,"note":60,"soundId":9,"priority":128}]}],"parts":[],"songs":[]}`
	if err := os.WriteFile(path, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "must-not-run"), RuntimeDir: t.TempDir()}
	if _, err := service.Build(dir, ""); err == nil || !strings.Contains(err.Error(), "sound 9 does not match ch1") {
		t.Fatalf("Build error = %v, want pre-toolchain native sound reference rejection", err)
	}
}

func nativeCH1Sounds() assetsync.NativeSoundLibrary {
	return assetsync.NativeSoundLibrary{Version: assetsync.NativeAudioSchemaVersion, Sounds: []assetsync.NativeSoundAsset{{
		ID: 0, Name: "pulse-lead", Kind: "ch1", Route: "both",
		Pulse: &assetsync.NativePulseAsset{
			Duty: 50, Envelope: assetsync.NativeEnvelopeAsset{Initial: 12, Direction: "down", Pace: 2},
			Sweep: &assetsync.NativeSweepAsset{Pace: 0, Direction: "up", Shift: 0},
		},
	}}}
}

func writeNativeCH1Fixture(t *testing.T, dir string) {
	t.Helper()
	sounds := nativeCH1Sounds()
	if err := assetsync.SaveNativeSounds(dir, &sounds); err != nil {
		t.Fatal(err)
	}
	music := assetsync.NativeMusicLibrary{
		Version: assetsync.NativeAudioSchemaVersion, PPQN: 96,
		Clips: []assetsync.NativeClipAsset{{
			ID: 0, Name: "startup", Kind: "ch1", LengthTicks: 96,
			Events: []assetsync.NativeEventAsset{{Tick: 0, DurationTicks: 24, Note: 60, SoundID: 0, Priority: 128}},
		}},
	}
	if err := assetsync.SaveNativeMusic(dir, sounds, &music); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRejectsOversizedMetadataBeforeToolchain(t *testing.T) {
	err := buildRejectsMetadataFixture(t,
		`{"width":65536,"height":65536,"sprites":[]}`,
		`{"width":0,"height":0,"cells":[]}`,
	)
	if err == nil || !strings.Contains(err.Error(), "uint32 asset limit") {
		t.Fatalf("Build error = %v, want metadata preflight uint32 limit error", err)
	}
}

// buildRejectsMetadataFixture writes a hand-crafted project (bypassing
// CreateProject/assetsync's own writers, which would refuse this input
// themselves) so Build's own validateBuildAssetFootprint preflight is what
// actually rejects it, before any TinyGo invocation.
func buildRejectsMetadataFixture(t *testing.T, spritesheetJSON, mapJSON string) error {
	t.Helper()
	projectDir := t.TempDir()
	files := map[string]string{
		"project.json":            `{"activePalette":"aurora"}`,
		"assets/spritesheet.json": spritesheetJSON,
		"assets/map.json":         mapJSON,
		GameFileName:              "function _draw()\nend\n",
	}
	for name, contents := range files {
		path := filepath.Join(projectDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"), RuntimeDir: t.TempDir()}
	_, err := service.Build(projectDir, "")
	if _, statErr := os.Stat(filepath.Join(projectDir, "game.gba")); !os.IsNotExist(statErr) {
		t.Fatalf("Build created an output despite metadata rejection: stat error %v", statErr)
	}
	return err
}

// TestBuildRejectsNegativeSheetDimensionsFromDisk covers checkedAssetProduct's
// negative-dimension branch: LoadSpritesheet/LoadMap deserialize whatever the
// JSON on disk says without validating it, so a hand-edited spritesheet.json
// (not something CreateProject or the editor would ever write) is the only
// way a negative width reaches Build's own footprint preflight.
func TestBuildRejectsNegativeSheetDimensionsFromDisk(t *testing.T) {
	err := buildRejectsMetadataFixture(t,
		`{"width":-8,"height":8,"sprites":[]}`,
		`{"width":0,"height":0,"cells":[]}`,
	)
	if err == nil || !strings.Contains(err.Error(), "nonnegative") {
		t.Fatalf("Build error = %v, want the nonnegative-dimension preflight error", err)
	}
}

// TestBuildRejectsOversizedMapMetadataBeforeToolchain covers
// validateBuildAssetFootprint's third checkedAssetProduct call (map cells):
// the sheet alone is small enough to pass the first two calls, so only an
// oversized map on its own can reach this branch.
func TestBuildRejectsOversizedMapMetadataBeforeToolchain(t *testing.T) {
	err := buildRejectsMetadataFixture(t,
		`{"width":8,"height":8,"sprites":[]}`,
		`{"width":100000,"height":100000,"cells":[]}`,
	)
	if err == nil || !strings.Contains(err.Error(), "uint32 asset limit") {
		t.Fatalf("Build error = %v, want the map uint32 limit preflight error", err)
	}
}

func TestWriteWorkspaceRejectsUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}
	locked := t.TempDir()
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	if err := (Service{}).writeWorkspace(locked, "package main"); err == nil {
		t.Fatal("expected writeWorkspace to fail writing into an unwritable directory")
	}
}

func TestBuildKeepWorkReportsWorkspaceOnFailure(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	if err := CreateProject(projectDir, NewProjectOptions{SheetWidth: 16, SheetHeight: 16, MapWidth: 2, MapHeight: 2}); err != nil {
		t.Fatal(err)
	}
	service := Service{
		TinyGoBin:  filepath.Join(t.TempDir(), "missing-tinygo"),
		RuntimeDir: t.TempDir(),
		KeepWork:   true,
	}
	_, err := service.Build(projectDir, "")
	if err == nil {
		t.Fatal("expected a missing TinyGo failure")
	}
	const marker = "(temporary build workspace kept at "
	start := strings.Index(err.Error(), marker)
	if start < 0 {
		t.Fatalf("error = %v, want the kept workspace path", err)
	}
	workspace := strings.TrimSuffix(err.Error()[start+len(marker):], ")")
	t.Cleanup(func() { _ = os.RemoveAll(workspace) })
	if info, statErr := os.Stat(workspace); statErr != nil || !info.IsDir() {
		t.Fatalf("kept workspace %q is not available: %v", workspace, statErr)
	}
}

// TestBuildEndToEndWithRealTinyGo is the only test in this package that
// exercises Service.Build against a real TinyGo instead of fixture inputs to
// GenerateTarget/ParseLinkerMap/ValidateROM in isolation. Those unit tests
// guard each stage's contract, but none of them proves the stages actually
// compose into a ROM tinygo will link and this package will accept. Skips when
// tinygo is not on PATH so the rest of the suite stays hermetic.
func TestBuildEndToEndWithRealTinyGo(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "e2e")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 16, SheetHeight: 16, MapWidth: 2, MapHeight: 2}); err != nil {
		t.Fatal(err)
	}
	// Keep both numeric lowering edge cases in this real-link smoke test while
	// preserving the generated starter game's coverage.
	gamePath := filepath.Join(dir, GameFileName)
	source, err := os.ReadFile(gamePath)
	if err != nil {
		t.Fatal(err)
	}
	source = append([]byte("large = 2147483648\nminimum = -32768.0\n"), source...)
	if err := os.WriteFile(gamePath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	romPath := filepath.Join(t.TempDir(), "game.gba")
	result, err := service.Build(dir, romPath)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.ROMPath != romPath {
		t.Fatalf("ROMPath = %q, want %q", result.ROMPath, romPath)
	}
	rom, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rom) != result.ROMSize {
		t.Fatalf("ROM on disk is %d bytes, Result.ROMSize says %d", len(rom), result.ROMSize)
	}
	if err := ValidateROM(rom); err != nil {
		t.Fatalf("Build produced a ROM that fails its own validation: %v", err)
	}
	if result.EWRAMFree < EWRAMReserve {
		t.Fatalf("EWRAMFree = %d, want at least the %d reserve Build already enforced", result.EWRAMFree, EWRAMReserve)
	}
	if _, err := os.Stat(service.RuntimeDir); err != nil {
		t.Fatalf("resolved runtime dir %q is unusable: %v", service.RuntimeDir, err)
	}
}

func TestBuildEndToEndWithRealTinyGoSFX(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "audio-sfx")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 16, SheetHeight: 16, MapWidth: 2, MapHeight: 2}); err != nil {
		t.Fatal(err)
	}
	writeNativeCH1Fixture(t, dir)
	game := `function _init()
  sfx(0)
end
function _draw()
  cls(0)
  print("sfx", 8, 8, 7)
end
`
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(game), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	romPath := filepath.Join(t.TempDir(), "audio-sfx.gba")
	result, err := service.Build(dir, romPath)
	if err != nil {
		t.Fatalf("audio SFX Build: %v", err)
	}
	rom, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.ROMSize != len(rom) || result.EWRAMFree < EWRAMReserve ||
		result.AudioVoices != 4 ||
		result.AudioProfile != assetsync.NativeAudioProfile || result.AudioPSGRatio != 100 || result.AudioROMBytes == 0 {
		t.Fatalf("audio build result = %+v file bytes=%d", result, len(rom))
	}
	if err := ValidateROM(rom); err != nil {
		t.Fatalf("audio SFX ROM failed validation: %v", err)
	}
}

func TestBuildEndToEndElidesAuthoredButUnusedNativeAudio(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "unused-native-audio")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 16, SheetHeight: 16, MapWidth: 2, MapHeight: 2}); err != nil {
		t.Fatal(err)
	}
	writeNativeCH1Fixture(t, dir)
	game := "function _draw()\n  cls(0)\nend\n"
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(game), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.KeepWork = true
	result, err := service.Build(dir, filepath.Join(t.TempDir(), "silent.gba"))
	if err != nil {
		t.Fatalf("silent native-audio Build: %v", err)
	}
	defer func() { _ = os.RemoveAll(result.WorkDir) }()
	if result.AudioROMBytes != 0 || result.AudioVoices != 0 || result.AudioProfile != "" || result.AudioPSGRatio != 0 {
		t.Fatalf("unused audio was reported as linked: %+v", result)
	}
	for _, forbidden := range []string{"nativeAudioController", "nativeSoundROM", "handleNativeTimer0"} {
		if strings.Contains(result.GeneratedGo, forbidden) {
			t.Fatalf("generated silent cartridge contains %q", forbidden)
		}
	}
	linkMap, err := os.ReadFile(result.MapPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(linkMap), "handleNativeTimer0") {
		t.Fatal("unused native Timer0 IRQ survived the target link")
	}
}

func TestBuildRejectsMissingLiteralSFXIDBeforeTinyGo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "invalid-static-sfx")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 16, SheetHeight: 16, MapWidth: 2, MapHeight: 2}); err != nil {
		t.Fatal(err)
	}
	writeNativeCH1Fixture(t, dir)
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte("function _init()\n  sfx(1)\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{TinyGoBin: filepath.Join(dir, "tinygo-must-not-run"), RuntimeDir: filepath.Join(dir, "runtime-must-not-be-read")}
	_, err := service.Build(dir, filepath.Join(dir, "invalid.gba"))
	if err == nil || !strings.Contains(err.Error(), "transpile game.lua") ||
		!strings.Contains(err.Error(), "game.lua:2:3: sfx ID 1 is not present in assets/audio/music.json") {
		t.Fatalf("Build error = %v, want static SFX ID diagnostic before TinyGo", err)
	}
}

func TestBuildEndToEndWithRealTinyGoStreamingMap(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "large-map")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 16, SheetHeight: 8, MapWidth: 192, MapHeight: 96}); err != nil {
		t.Fatal(err)
	}
	game := `cell=0
function _init()
  mset(0,191,95,1)
end
function _update()
  cell=mget(0,191,95)
end
function _draw()
  cls(0)
  map(0,0,0,0,0,30,20,0)
end
`
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(game), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	service.KeepWork = true
	romPath := filepath.Join(t.TempDir(), "large-map.gba")
	result, err := service.Build(dir, romPath)
	if err != nil {
		t.Fatalf("Build large streaming map: %v", err)
	}
	defer func() { _ = os.RemoveAll(result.WorkDir) }()
	rom, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateROM(rom); err != nil {
		t.Fatalf("streaming-map ROM failed validation: %v", err)
	}
	if result.EWRAMFree < EWRAMReserve {
		t.Fatalf("streaming-map EWRAMFree = %d, want at least %d", result.EWRAMFree, EWRAMReserve)
	}
	linkMap, err := os.ReadFile(result.MapPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"loadMapLayerChunk", "main.mapChunksRAM", "main.mapEditsRAM"} {
		if !strings.Contains(string(linkMap), required) {
			t.Fatalf("streaming build did not retain required chunk-cache symbol %q", required)
		}
	}
}

func TestBuildEndToEndCompactMapElidesStreamingRuntime(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "compact-map")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 16, SheetHeight: 8, MapWidth: 30, MapHeight: 20}); err != nil {
		t.Fatal(err)
	}
	game := `cell=0
function _init()
  mset(0,29,19,1)
end
function _update()
  cell=mget(0,29,19)
end
function _draw()
  cls(0)
  map(0,0,0,0,0,30,20,0)
end
`
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(game), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.KeepWork = true
	result, err := service.Build(dir, filepath.Join(t.TempDir(), "compact-map.gba"))
	if err != nil {
		t.Fatalf("Build compact map: %v", err)
	}
	defer func() { _ = os.RemoveAll(result.WorkDir) }()
	linkMap, err := os.ReadFile(result.MapPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"mapStreamingGetLayer", "mapStreamingSetLayer", "loadMapLayerChunk", "mapEditLookup"} {
		if strings.Contains(string(linkMap), forbidden) {
			t.Fatalf("compact build retained streaming symbol %q", forbidden)
		}
	}
}

func TestBuildEndToEndHeaplessElidesLuaHeapRuntime(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "heapless")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 16, SheetHeight: 8, MapWidth: 2, MapHeight: 2}); err != nil {
		t.Fatal(err)
	}
	game := `frame=1
player={x=8,y=8}
function _update()
  if btn(0) then player.x=player.x-1 end
end
function _draw()
  cls(0)
  spr(frame,player.x,player.y)
  print("A",0,0,7)
end
`
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(game), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.KeepWork = true
	service.Renderer = transpiler.RendererMode0
	result, err := service.Build(dir, filepath.Join(t.TempDir(), "heapless.gba"))
	if err != nil {
		t.Fatalf("Build heapless cartridge: %v", err)
	}
	defer func() { _ = os.RemoveAll(result.WorkDir) }()
	if result.HeapLinked {
		t.Fatal("heapless build reported the Lua heap as linked")
	}
	if result.EWRAMUsed >= 16*1024 {
		t.Fatalf("heapless Mode 0 build uses %d EWRAM bytes, want compact Context below 16 KiB", result.EWRAMUsed)
	}
	linkMap, err := os.ReadFile(result.MapPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"heapSpaceA", "heapSpaceB", "collectCore", "ensureHeap", "RegisterGlobal", "PushLocal", "PushTemp", "uploadMode0DirtyTiles"} {
		if strings.Contains(string(linkMap), forbidden) {
			t.Fatalf("specialized build retained unreachable runtime symbol %q", forbidden)
		}
	}
}

func TestBuildEndToEndAllocatingProgramRetainsLuaHeapRuntime(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "heap-program")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	game := `items={1}
function _update()
  add(items,2)
  deli(items,1)
end
function _draw()
  cls(0)
end
`
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(game), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.KeepWork = true
	result, err := service.Build(dir, filepath.Join(t.TempDir(), "heap-program.gba"))
	if err != nil {
		t.Fatalf("Build allocating cartridge: %v", err)
	}
	defer func() { _ = os.RemoveAll(result.WorkDir) }()
	if !result.HeapLinked {
		t.Fatal("allocating build reported the Lua heap as elided")
	}
	linkMap, err := os.ReadFile(result.MapPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"heapSpaceA", "heapSpaceB", "collectCore"} {
		if !strings.Contains(string(linkMap), required) {
			t.Fatalf("allocating build omitted required Lua heap symbol %q", required)
		}
	}
}

func TestBuildEndToEndWithRealTinyGoZeroSpriteAssets(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "zero-sprite")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 7, SheetHeight: 8}); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	romPath := filepath.Join(t.TempDir(), "zero-sprite.gba")
	result, err := service.Build(dir, romPath)
	if err != nil {
		t.Fatalf("Build zero-sprite project: %v", err)
	}
	rom, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.ROMSize != len(rom) {
		t.Fatalf("ROMSize = %d, file size = %d", result.ROMSize, len(rom))
	}
	if err := ValidateROM(rom); err != nil {
		t.Fatalf("zero-sprite ROM failed validation: %v", err)
	}
}

// TestBuildExamplesHelloEndToEnd builds the actual examples/hello/ project
// shipped in the repository (the same one examples/hello/README.md points
// users at) rather than a synthetic fixture.
// Every other real-TinyGo test in this file exercises CreateProject output;
// none of them would notice if a transpiler, runtime, or asset-loading change
// broke the one example a new user is told to run first.
//
// The example is copied first so a leftover examples/hello/game.gba from a
// manual `mtile build examples/hello/` cannot make this test fail, and so
// we can plant a sentinel ROM to prove an explicit -o path does not clobber
// a pre-existing game.gba in the project directory.
func TestBuildExamplesHelloEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	projectDir := filepath.Join(t.TempDir(), "hello")
	copyHelloExample(t, projectDir)
	sentinel := []byte("not a gba rom")
	if err := os.WriteFile(filepath.Join(projectDir, "game.gba"), sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	romPath := filepath.Join(t.TempDir(), "game.gba")
	result, err := service.Build(projectDir, romPath)
	if err != nil {
		t.Fatalf("Build examples/hello: %v", err)
	}
	leftover, err := os.ReadFile(filepath.Join(projectDir, "game.gba"))
	if err != nil {
		t.Fatal(err)
	}
	if string(leftover) != string(sentinel) {
		t.Fatal("Build with an explicit output path clobbered a pre-existing game.gba in the project directory")
	}
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") || e.Name() == "go.mod" || e.Name() == "target-gba.json" {
			t.Fatalf("Build wrote toolchain file %q into the example project", e.Name())
		}
	}
	rom, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.ROMSize != len(rom) {
		t.Fatalf("ROMSize = %d, file size = %d", result.ROMSize, len(rom))
	}
	if err := ValidateROM(rom); err != nil {
		t.Fatalf("examples/hello ROM failed validation: %v", err)
	}
	if result.EWRAMFree < EWRAMReserve {
		t.Fatalf("EWRAMFree = %d, want at least the %d reserve Build already enforced", result.EWRAMFree, EWRAMReserve)
	}
}

// TestAudioExamplesUseEditableProjectAssets guards the editor-facing example
// contract. Sprite 0 is the map eraser, so a one-sprite sheet leaves no tile
// that can be authored or placed even though the audio-only ROM still builds.
func TestAudioExamplesUseEditableProjectAssets(t *testing.T) {
	for _, name := range []string{
		"audio-gb-melody", "audio-gb-song", "audio-gb-editor",
	} {
		t.Run(name, func(t *testing.T) {
			assets, err := assetsync.Load(examplesProjectDir(t, name))
			if err != nil {
				t.Fatalf("load example: %v", err)
			}
			if assets.SheetW < 8 || assets.SheetH < 8 || assets.SheetW%8 != 0 || assets.SheetH%8 != 0 {
				t.Fatalf("audio example has a non-editable sheet: %dx%d", assets.SheetW, assets.SheetH)
			}
			if assets.SpriteN <= 1 {
				t.Fatalf("SpriteN = %d, want at least one editable ID after reserved sprite 0", assets.SpriteN)
			}
			if assets.MapW < 1 || assets.MapH < 1 {
				t.Fatalf("map = %dx%d, want positive dimensions", assets.MapW, assets.MapH)
			}
		})
	}
}

// Background-layers is a teaching project users are expected to open in the
// visual editors, not only build as a ROM. Keep its logical layout aligned to
// the native 30x20 sheet, exact screen guides, and four authored BG planes.
func TestBackgroundLayersExampleUsesEditorFriendlyAssets(t *testing.T) {
	assets, err := assetsync.Load(examplesProjectDir(t, "background-layers"))
	if err != nil {
		t.Fatalf("load example: %v", err)
	}
	if assets.SheetW != 240 || assets.SheetH != 160 || assets.SpriteCols != 30 || assets.SpriteRows != 20 || assets.SpriteN != 600 {
		t.Fatalf("sheet = %dx%d pixels (%dx%d sprites, SpriteN=%d), want native 240x160 (30x20, SpriteN=600)",
			assets.SheetW, assets.SheetH, assets.SpriteCols, assets.SpriteRows, assets.SpriteN)
	}
	if assets.MapW != 60 || assets.MapH != 20 || assets.MapLayers != 4 {
		t.Fatalf("map = %dx%dx%d, want four 60x20 planes", assets.MapW, assets.MapH, assets.MapLayers)
	}
	planeCells := assets.MapW * assets.MapH
	for layer := 0; layer < assets.MapLayers; layer++ {
		for y := 0; y < assets.MapH; y++ {
			for x := 0; x < 30; x++ {
				base := layer*planeCells + y*assets.MapW
				left := assets.MapROM[base+x]
				right := assets.MapROM[base+x+30]
				if left != right {
					t.Fatalf("BG%d repeat differs at (%d,%d): left=%d right=%d", layer, x, y, left, right)
				}
			}
		}
	}
	if colors := len(assets.Palette.Colors); colors != 256 {
		t.Fatalf("palette colors = %d, want 256 paintable entries", colors)
	}
}

func TestBackgroundLayersExampleSelectsFourParallaxBackgrounds(t *testing.T) {
	gamePath := filepath.Join(examplesProjectDir(t, "background-layers"), GameFileName)
	source, err := os.ReadFile(gamePath)
	if err != nil {
		t.Fatal(err)
	}
	for bg := 0; bg < 4; bg++ {
		if !strings.Contains(string(source), fmt.Sprintf("map(%d,", bg)) {
			t.Fatalf("game.lua does not draw explicit BG%d", bg)
		}
	}
	if strings.Count(string(source), "camera(") != 5 || !strings.Contains(string(source), "spr(10,") || !strings.Contains(string(source), "print(") {
		t.Fatal("game.lua must retain four parallax cameras, the player object, a camera reset, and its overlay text")
	}
	assets, err := assetsync.Load(examplesProjectDir(t, "background-layers"))
	if err != nil {
		t.Fatal(err)
	}
	generated, report, err := transpiler.TranspileWithReport("game.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererMode0,
		SheetW:   assets.SheetW, SheetH: assets.SheetH,
		MapW: assets.MapW, MapH: assets.MapH, MapLayers: assets.MapLayers,
		Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Layers != 4 || report.MapDrawMask != 0x0f {
		t.Fatalf("background-layers report = %+v, want all four logical and hardware BGs", report)
	}
	if !report.Mode0Objects || !report.Mode0Text {
		t.Fatalf("background-layers report = %+v, want the player object and overlay text", report)
	}
	for slot := 0; slot < 4; slot++ {
		if !strings.Contains(generated, fmt.Sprintf("c.MapBGAt(%d,", slot)) {
			t.Fatalf("BG%d did not compile to dense hardware slot %d", slot, slot)
		}
	}
}

// Coin-block is the deliberately small current-API proof for an animated map
// cell which becomes a stable used state. Keep it AUTO-eligible so the example
// remains evidence for the existing mutation path rather than quietly relying
// on a forced renderer or a future tile/entity system.
func TestCoinBlockExampleUsesOneCurrentAPIMutationAndSelectsMode0(t *testing.T) {
	dir := examplesProjectDir(t, "coin-block")
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatalf("load example: %v", err)
	}
	if assets.SheetW != 240 || assets.SheetH != 160 || assets.SpriteN != 600 {
		t.Fatalf("sheet = %dx%d SpriteN=%d, want native 240x160 SpriteN=600", assets.SheetW, assets.SheetH, assets.SpriteN)
	}
	if assets.MapW != 30 || assets.MapH != 20 {
		t.Fatalf("map = %dx%d, want one 30x20 plane", assets.MapW, assets.MapH)
	}
	if got := assets.MapROM[14*assets.MapW+14]; got != 4 {
		t.Fatalf("initial block tile = %d, want active tile 4", got)
	}
	if assets.Project.Renderer != "auto" {
		t.Fatalf("renderer = %q, want auto", assets.Project.Renderer)
	}

	game, err := os.ReadFile(filepath.Join(dir, GameFileName))
	if err != nil {
		t.Fatal(err)
	}
	source := string(game)
	for _, snippet := range []string{
		"btnp(5) or frame==90",
		"if jump_tick==4 and not block_used then",
		"coins=1",
		"mset(block_x,block_y,tile)",
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("game.lua is missing coin-block contract %q", snippet)
		}
	}
	if strings.Count(source, "\n  mset(") != 1 {
		t.Fatalf("game.lua has %d direct mset statements, want one", strings.Count(source, "\n  mset("))
	}

	requested, err := transpiler.ParseRenderer(assets.Project.Renderer)
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := transpiler.TranspileWithReport(GameFileName, source, transpiler.Options{
		Renderer: requested,
		SheetW:   assets.SheetW,
		SheetH:   assets.SheetH,
		MapW:     assets.MapW,
		MapH:     assets.MapH,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || report.Mode0Layers != 1 || !report.Mode0Objects || !report.Mode0Text {
		t.Fatalf("renderer report = %+v, want AUTO-selected Mode 0 with one BG, objects, and text", report)
	}
}

func TestNativeAudioExamplesUseOnlyFourChannelV2Assets(t *testing.T) {
	for _, name := range []string{"audio-gb-melody", "audio-gb-song", "audio-gb-editor"} {
		t.Run(name, func(t *testing.T) {
			dir := examplesProjectDir(t, name)
			assets, err := assetsync.Load(dir)
			if err != nil {
				t.Fatalf("load native example: %v", err)
			}
			if assets.AudioVersion != assetsync.NativeAudioSchemaVersion || assets.NativeAudio == nil {
				t.Fatalf("audio version/native assets = %d/%v", assets.AudioVersion, assets.NativeAudio)
			}
			if assets.NativeAudio.Settings.Profile != assetsync.NativeAudioProfile {
				t.Fatalf("example audio profile = %q", assets.NativeAudio.Settings.Profile)
			}
			for _, relative := range []string{"sfx.json", "samples/manifest.json"} {
				if _, err := os.Stat(filepath.Join(dir, "assets", "audio", relative)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("superseded asset %s exists: %v", relative, err)
				}
			}
		})
	}
}

func examplesProjectDir(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate this test file")
	}
	dir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "examples", name))
	if info, err := os.Stat(filepath.Join(dir, GameFileName)); err != nil || info.IsDir() {
		t.Fatalf("examples/%s fixture is missing or invalid: %v", name, err)
	}
	return dir
}

func examplesHelloDir(t *testing.T) string {
	return examplesProjectDir(t, "hello")
}

func copyHelloExample(t *testing.T, dest string) {
	t.Helper()
	src := examplesHelloDir(t)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if strings.HasSuffix(d.Name(), ".gba") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateProjectLeftoverHeightTranspileFlagsMatchSpriteN(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tall-edge")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 240, SheetHeight: 9}); err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.SpriteN != 30 || len(assets.FlagROM) != 30 {
		t.Fatalf("assetsync SpriteN=%d flags=%d, want 30/30", assets.SpriteN, len(assets.FlagROM))
	}
	source, err := os.ReadFile(filepath.Join(dir, GameFileName))
	if err != nil {
		t.Fatal(err)
	}
	got, err := transpiler.Transpile(GameFileName, string(source), transpiler.Options{
		PackageName: "main",
		SheetW:      assets.SheetW, SheetH: assets.SheetH,
		MapW: assets.MapW, MapH: assets.MapH,
		Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM,
		Palette: assets.PaletteROM[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "var flagROM = [ 30 ]byte") && !strings.Contains(got, "var flagROM = [30]byte") {
		t.Fatalf("Build path emitted flags that do not match SpriteN=30:\n%s", got)
	}
	if strings.Contains(got, "var flagROM = [ 33 ]byte") || strings.Contains(got, "var flagROM = [33]byte") {
		t.Fatal("Build path used left-associative W/8*H/8 (33) instead of SpriteN (30)")
	}
}

func TestCreateProjectLeftoverEdgeUsesEmptyMap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "edge")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 7, SheetHeight: 8}); err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.SpriteN != 0 || assets.MapW != 0 || assets.MapH != 0 || len(assets.FlagROM) != 0 || len(assets.MapROM) != 0 {
		t.Fatalf("leftover-edge project = sprites %d map %dx%d flags %d map cells %d", assets.SpriteN, assets.MapW, assets.MapH, len(assets.FlagROM), len(assets.MapROM))
	}
}

// TestCreateProjectRejectsSheetExceedingMaxSpriteN covers a sheet that
// validateNewProjectFootprint's byte-budget check alone would accept (its
// pixel payload is a few MB, nowhere near the 32 MiB cartridge cap) but
// which assetsync.SaveSpritesheet still rejects for exceeding MaxSpriteN.
// It guards the layering between the two checks: CreateProject relies on
// SaveSpritesheet, not its own footprint check, to enforce the sprite-count
// ceiling.
func TestCreateProjectRejectsSheetExceedingMaxSpriteN(t *testing.T) {
	options := NewProjectOptions{SheetWidth: 4096, SheetHeight: 1032}
	if spriteN := spriteCount(options.SheetWidth, options.SheetHeight); spriteN <= assetsync.MaxSpriteN {
		t.Fatalf("test fixture spriteN = %d, want it above MaxSpriteN=%d", spriteN, assetsync.MaxSpriteN)
	}
	err := CreateProject(filepath.Join(t.TempDir(), "too-many-sprites"), options)
	if err == nil || !strings.Contains(err.Error(), "complete sprites") {
		t.Fatalf("CreateProject error = %v, want it to name the MaxSpriteN rejection", err)
	}
}

func TestBuildRejectsMissingProjectDirectory(t *testing.T) {
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"), RuntimeDir: t.TempDir()}
	_, err := service.Build(filepath.Join(t.TempDir(), "does-not-exist"), "")
	if err == nil || !strings.Contains(err.Error(), "project directory") {
		t.Fatalf("Build error = %v, want a missing project directory error", err)
	}
}

func TestBuildRejectsProjectDirectoryThatIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"), RuntimeDir: t.TempDir()}
	_, err := service.Build(file, "")
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("Build error = %v, want a not-a-directory error", err)
	}
}

func TestBuildRejectsMissingGameFile(t *testing.T) {
	dir := t.TempDir()
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"), RuntimeDir: t.TempDir()}
	_, err := service.Build(dir, "")
	if err == nil || !strings.Contains(err.Error(), GameFileName) {
		t.Fatalf("Build error = %v, want it to name %s", err, GameFileName)
	}
}

func TestCreateProjectRejectsMapWhenSpriteNIsZero(t *testing.T) {
	err := CreateProject(filepath.Join(t.TempDir(), "edge-map"), NewProjectOptions{
		SheetWidth: 7, SheetHeight: 8, MapWidth: 1, MapHeight: 1,
	})
	if err == nil {
		t.Fatal("expected leftover-edge sheet with a tile map to be rejected")
	}
	if !strings.Contains(err.Error(), "tile map") && !strings.Contains(err.Error(), "8x8") {
		t.Fatalf("error = %v, want it to name the leftover-edge/map conflict", err)
	}
}
