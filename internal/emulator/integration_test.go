package emulator_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
)

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
