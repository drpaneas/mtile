package assetsync

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedAssetsInitSucceedsAgainstRealRuntime compiles GenerateGo's
// output together with the actual runtime package (not a fake) and calls the
// generated InitAssets against a real rt.Context. Package-shape tests (see
// TestGenerateGo) only check that the emitted text contains expected
// declarations; they cannot catch a mismatch between the ROM/RAM array
// lengths GenerateGo emits and what runtime.assetLensExact independently
// requires, since that check only runs once real Go code executes it. This
// is the equivalent of the transpiler's compileGenerated/runGeneratedHarness
// checks, applied to the asset pipeline.
func TestGeneratedAssetsInitSucceedsAgainstRealRuntime(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	settings, sounds, music := nativeAudioFixture()
	if err := SaveNativeAudioSettings(dir, &settings); err != nil {
		t.Fatal(err)
	}
	if err := SaveNativeSounds(dir, &sounds); err != nil {
		t.Fatal(err)
	}
	if err := SaveNativeMusic(dir, sounds, &music); err != nil {
		t.Fatal(err)
	}

	source, err := GenerateGo(dir, "main")
	if err != nil {
		t.Fatal(err)
	}

	genDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(genDir, "assets.go"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeDir, err := filepath.Abs(filepath.Join("..", "..", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	mod := fmt.Sprintf("module generated\n\ngo 1.27.1\n\nrequire github.com/drpaneas/mtile/runtime v0.0.0\n\nreplace github.com/drpaneas/mtile/runtime => %s\n", runtimeDir)
	if err := os.WriteFile(filepath.Join(genDir, "go.mod"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
	"testing"
)

func TestInitAssetsWiresRealCartridge(t *testing.T) {
	var c rt.Context
	if !InitAssets(&c) {
		t.Fatal("InitAssets returned false against real runtime.Context")
	}
	if !c.CartReady() {
		t.Fatal("CartReady() is false after a successful InitAssets")
	}
	if got := c.Sget(0, 0); got != 15 {
		t.Fatalf("Sget(0,0) = %d, want 15 (from the ROM-seeded sprite)", got)
	}
	if got := c.FgetByte(0); got != 0xa5 {
		t.Fatalf("FgetByte(0) = %#x, want 0xa5", got)
	}
	if got := c.Mget(0, 0); got != 0 {
		t.Fatalf("Mget(0,0) = %d, want 0", got)
	}
	if got, want := c.PaletteEntry(1), uint16(0x0842); got != want {
		t.Fatalf("PaletteEntry(1) = %#04x, want %#04x (canonical aurora gray 0x111111, quantized)", got, want)
	}
	var controller audio.Controller
	if !InitNativeAudio(&controller) {
		t.Fatal("InitNativeAudio rejected generated target records")
	}
	c.AttachNativeAudio(&controller)
	c.MusicV(rt.NumVal(rt.FromInt(0)), rt.Value{})
	controller.Service()
	if got := c.AudioStats().Native; got.CommandsRejected != 0 || got.InvalidAssets != 0 || !got.Channels[0].Active {
		t.Fatalf("generated PSG state = %+v", got)
	}
}
`
	if err := os.WriteFile(filepath.Join(genDir, "harness_test.go"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-run", "TestInitAssetsWiresRealCartridge", "-v", "./...")
	cmd.Dir = genDir
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(os.TempDir(), "mtile-generated-gocache"))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("generated assets harness failed: %v\n%s", err, out.String())
	}
}

func TestGeneratedLargeMapUsesStreamingCacheAgainstRealRuntime(t *testing.T) {
	dir := t.TempDir()
	sheet := Spritesheet{Width: 16, Height: 8}
	mapData := Map{Width: 192, Height: 96, Cells: make([]int, 192*96)}
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)

	source, err := GenerateGo(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "MapStreaming = true") ||
		!strings.Contains(string(source), "mapRAM [rt.MapCacheCells]uint16") ||
		!strings.Contains(string(source), "mapChunksRAM [rt.MapCacheSlots]rt.MapChunk") ||
		!strings.Contains(string(source), "mapEditsRAM [rt.MapEditCapacity]rt.MapEdit") {
		t.Fatal("large-map codegen did not select the streaming EWRAM layout")
	}

	genDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(genDir, "assets.go"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeDir, err := filepath.Abs(filepath.Join("..", "..", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	mod := fmt.Sprintf("module generated\n\ngo 1.27.1\n\nrequire github.com/drpaneas/mtile/runtime v0.0.0\n\nreplace github.com/drpaneas/mtile/runtime => %s\n", runtimeDir)
	if err := os.WriteFile(filepath.Join(genDir, "go.mod"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestLargeMapStreamingSemantics(t *testing.T) {
	var c rt.Context
	if !InitAssets(&c) {
		t.Fatal("InitAssets returned false")
	}
	if got := c.Mget(191, 95); got != 0 {
		t.Fatalf("Mget at far edge = %d, want 0", got)
	}
	c.Mset(31, 31, 1)
	for _, p := range [][2]int{{0, 0}, {32, 0}, {64, 0}, {0, 32}, {32, 32}, {64, 32}} {
		_ = c.Mget(p[0], p[1])
	}
	if got := c.Mget(31, 31); got != 1 {
		t.Fatalf("Mset edit was lost after chunk eviction: %d", got)
	}
}
`
	if err := os.WriteFile(filepath.Join(genDir, "harness_test.go"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-run", "TestLargeMapStreamingSemantics", "-v", "./...")
	cmd.Dir = genDir
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(os.TempDir(), "mtile-generated-gocache"))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("large-map generated harness failed: %v\n%s", err, out.String())
	}
}

// TestRuntimeDefaultPaletteMatchesCanonicalAurora compiles against the nested
// runtime module (the parent module cannot import it) and checks that a
// zero-valued Context's defaultPalette matches aurora.hex quantization. This
// is the drift detector between internal/assetsync/aurora.hex and
// runtime/palette_aurora.go.
func TestRuntimeDefaultPaletteMatchesCanonicalAurora(t *testing.T) {
	p := canonicalPalette()
	var want strings.Builder
	for i, col := range p.Colors {
		if i > 0 {
			want.WriteString(", ")
		}
		fmt.Fprintf(&want, "%#04x", col.Packed())
	}

	runtimeDir, err := filepath.Abs(filepath.Join("..", "..", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	genDir := t.TempDir()
	mod := fmt.Sprintf("module generated\n\ngo 1.27.1\n\nrequire github.com/drpaneas/mtile/runtime v0.0.0\n\nreplace github.com/drpaneas/mtile/runtime => %s\n", runtimeDir)
	if err := os.WriteFile(filepath.Join(genDir, "go.mod"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	harness := fmt.Sprintf(`package main

import (
	"testing"

	rt "github.com/drpaneas/mtile/runtime"
)

func TestDefaultPaletteMatchesAssetsyncAurora(t *testing.T) {
	want := [256]uint16{%s}
	var c rt.Context
	for i, w := range want {
		if got := c.PaletteEntry(i); got != w {
			t.Fatalf("defaultPalette[%%d] = %%#04x, want %%#04x", i, got, w)
		}
	}
}
`, want.String())
	if err := os.WriteFile(filepath.Join(genDir, "harness_test.go"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-run", "TestDefaultPaletteMatchesAssetsyncAurora", "-v", "./...")
	cmd.Dir = genDir
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(os.TempDir(), "mtile-generated-gocache"))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("runtime default palette harness failed: %v\n%s", err, out.String())
	}
}
