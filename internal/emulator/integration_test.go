package emulator_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
)

// Keep a real mTile panic path linked: this proves the logging wrapper is
// allocation-free and its diagnostics survive a cartridge that stops drawing.
func TestLiveRuntimeFailureLog(t *testing.T) {
	if os.Getenv("MTILE_LIVE_INTEGRATION") != "1" {
		t.Skip("set MTILE_LIVE_INTEGRATION=1 to build and run a diagnostic cartridge")
	}
	helper := os.Getenv("MTILE_PLAYER")
	if helper == "" {
		t.Fatal("MTILE_PLAYER must point to the native helper")
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	root, err := exec.Command(service.TinyGoBin, "env", "TINYGOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := build.GenerateTarget(strings.TrimSpace(string(root)), service.RuntimeDir, target); err != nil {
		t.Fatal(err)
	}
	module := fmt.Sprintf("module diagnostic\n\ngo 1.27.1\n\nrequire github.com/drpaneas/mtile/runtime v0.0.0\nreplace github.com/drpaneas/mtile/runtime => %q\n", filepath.ToSlash(service.RuntimeDir))
	source := `package main
import (
 rt "github.com/drpaneas/mtile/runtime"
 "github.com/drpaneas/mtile/runtime/hw"
 _ "unsafe"
)
//go:section .ewram
var ctx rt.Context
func main() {
 hw.SetReg16(hw.INTERRUPT_IME, 1)
 rt.DebugLog("long: ` + strings.Repeat("x", 300) + `")
 rt.DebugLog("short")
 if hw.GetReg16(hw.INTERRUPT_IME) != 1 { rt.DebugLog("interrupt restore failed") }
 ctx.Pget(0, 0) // Deliberately rejected by the private tile-canvas renderer.
 for {}
}`
	for name, contents := range map[string]string{"go.mod": module, "main.go": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rom := filepath.Join(dir, "diagnostic.gba")
	cmd := exec.Command(service.TinyGoBin, "build", "-gc=none", "-opt=2", "-tags=mtile_preview_log,mtile_tilecanvas,mtile_heapless", "-target="+target, "-o", rom, ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("diagnostic ROM build: %v\n%s", err, output)
	}
	session, err := emulator.Start(helper, rom)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	for i := 0; i < 4; i++ {
		if _, err := session.Step(0, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	logs := strings.Join(session.DrainLogs(), "\n")
	for _, want := range []string{"long: " + strings.Repeat("x", 249), "short", "mTile runtime error:", "tile canvas composed pixel readback is unsupported; use Mode 4"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("diagnostic %q missing from %q", want, logs)
		}
	}
	if strings.Contains(logs, "interrupt restore failed") || strings.Contains(logs, "shortx") {
		t.Fatalf("logger corrupted state or retained a previous message: %q", logs)
	}
}

// TestLiveDebugTelemetry exercises the complete preview path: TinyGo debug
// cartridge, native mGBA helper, protocol v2, runtime counters, and named Lua
// globals. It is opt-in because it needs the external TinyGo toolchain.
func TestLiveDebugTelemetry(t *testing.T) {
	if os.Getenv("MTILE_LIVE_INTEGRATION") != "1" {
		t.Skip("set MTILE_LIVE_INTEGRATION=1 to build and run a real debug cartridge")
	}
	helper := os.Getenv("MTILE_PLAYER")
	if helper == "" {
		t.Fatal("MTILE_PLAYER must point to the native helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository root")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	service.KeepWork = true
	rom := filepath.Join(t.TempDir(), "mapdemo-debug.gba")
	result, err := service.Build(filepath.Join(root, "examples", "mapdemo"), rom)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("debug build workspace: %s", result.WorkDir)
	session, err := emulator.Start(helper, rom)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	var firstFrames uint32
	var firstPX int32
	var lastPX int32
	seenDebug := false
	seenPX := false
	lastFrames := uint32(0)
	for frameIndex := 0; frameIndex < 20; frameIndex++ {
		keys := uint16(0)
		if frameIndex >= 4 {
			keys = emulator.KeyRight
		}
		frame, err := session.Step(keys, false)
		if err != nil {
			t.Fatal(err)
		}
		if !frame.Telemetry.Valid {
			t.Fatalf("frame %d has invalid hardware telemetry: %+v", frameIndex, frame.Telemetry)
		}
		if !frame.Telemetry.DebugValid {
			continue
		}
		if !seenDebug {
			firstFrames = frame.Telemetry.Stats.Frames
			seenDebug = true
		}
		lastFrames = frame.Telemetry.Stats.Frames
		for _, watch := range frame.Telemetry.Watches {
			if watch.Name != "px" {
				continue
			}
			if !seenPX {
				firstPX = watch.Raw
				seenPX = true
			}
			lastPX = watch.Raw
		}
	}
	if !seenDebug {
		t.Fatal("debug mailbox never published after warm-up")
	}
	if !seenPX {
		t.Fatal("debug mailbox did not expose the top-level px global")
	}
	if lastFrames <= firstFrames {
		t.Fatalf("runtime frame counter did not advance: first=%d last=%d", firstFrames, lastFrames)
	}
	if firstPX == 0 {
		t.Fatal("px watch did not preserve its non-zero fixed-point value")
	}
	if lastPX <= firstPX {
		t.Fatalf("px watch did not react to right input: first=%d last=%d", firstPX, lastPX)
	}
	// Close joins os/exec's stderr pump before checking the final log lines.
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if logs := strings.Join(session.DrainLogs(), "\n"); !strings.Contains(logs, "Cartridge preview started") {
		t.Fatalf("preview startup diagnostic missing: %s", logs)
	}
}

// TestLivePSGTelemetry proves that the inspector's channel state comes from
// mGBA's GBA sound registers while a real PSG-only cartridge is playing.
func TestLivePSGTelemetry(t *testing.T) {
	if os.Getenv("MTILE_LIVE_INTEGRATION") != "1" {
		t.Skip("set MTILE_LIVE_INTEGRATION=1 to build and run a real PSG cartridge")
	}
	helper := os.Getenv("MTILE_PLAYER")
	if helper == "" {
		t.Fatal("MTILE_PLAYER must point to the native helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository root")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	rom := filepath.Join(t.TempDir(), "audio-psg-debug.gba")
	if _, err := service.Build(filepath.Join(root, "examples", "audio-gb-song"), rom); err != nil {
		t.Fatal(err)
	}
	session, err := emulator.Start(helper, rom)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	active := false
	for frameIndex := 0; frameIndex < 24; frameIndex++ {
		frame, err := session.Step(0, false)
		if err != nil {
			t.Fatal(err)
		}
		telemetry := frame.Telemetry
		if !telemetry.Valid {
			t.Fatalf("frame %d has invalid hardware telemetry", frameIndex)
		}
		if telemetry.PSG.Status&0x0f != 0 && telemetry.PSG.MixPSG != 0 {
			active = true
			break
		}
	}
	if !active {
		t.Fatal("PSG cartridge never exposed an active hardware channel and mixer state")
	}
}
