package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/emulator"
)

func TestDesktopStarterCreatesOneEditableProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	bundle := t.TempDir()
	exe := filepath.Join(bundle, "mtile-editor")
	if starter, err := openDesktopStarter(exe); err != nil || starter.projectDir != "" {
		t.Fatalf("source checkout startup: %+v, %v", starter, err)
	}
	rom := filepath.Join(bundle, "coin-quest.gba")
	if err := os.WriteFile(rom, []byte("packaged ROM"), 0o600); err != nil {
		t.Fatal(err)
	}
	type opened struct {
		starter desktopStarter
		err     error
	}
	results := make(chan opened, 4)
	for range cap(results) {
		go func() {
			s, err := openDesktopStarter(exe)
			results <- opened{s, err}
		}()
	}
	dir := filepath.Join(home, "mTile Projects", "Coin Quest")
	fresh := 0
	for range cap(results) {
		r := <-results
		if r.err != nil || r.starter.projectDir != dir {
			t.Fatalf("open starter: %+v", r)
		}
		if r.starter.rom.ROMPath == rom {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("expected exactly one creation, got %d", fresh)
	}
	entry := filepath.Join(dir, "game.lua")
	data, err := os.ReadFile(entry)
	if err != nil || !strings.Contains(string(data), "COIN QUEST") {
		t.Fatalf("missing Coin Quest source: %v", err)
	}
	if err := os.WriteFile(entry, []byte("saved edits"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := openDesktopStarter(exe)
	if err != nil || s.projectDir != dir || s.rom.ROMPath != "" {
		t.Fatalf("edited project must rebuild: %+v, %v", s, err)
	}
	if data, err := os.ReadFile(entry); err != nil || string(data) != "saved edits" {
		t.Fatalf("saved edits were lost: %q, %v", data, err)
	}
}

func TestDesktopStarterMacAppResources(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS app bundle layout")
	}
	t.Setenv("HOME", t.TempDir())
	contents := filepath.Join(t.TempDir(), "Mtile.app", "Contents")
	resources := filepath.Join(contents, "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatal(err)
	}
	rom := filepath.Join(resources, "coin-quest.gba")
	if err := os.WriteFile(rom, []byte("starter"), 0o644); err != nil {
		t.Fatal(err)
	}
	starter, err := openDesktopStarter(filepath.Join(contents, "MacOS", "mtile-editor"))
	if err != nil || starter.rom.ROMPath != rom {
		t.Fatalf("macOS starter = %+v, %v", starter, err)
	}
}

func TestDesktopStarterRunsPackagedROM(t *testing.T) {
	rom := os.Getenv("MTILE_VERIFY_STARTER_ROM")
	if rom == "" {
		t.Skip("set MTILE_VERIFY_STARTER_ROM to the packaged coin-quest.gba")
	}
	info, err := os.Stat(rom)
	if err != nil {
		t.Fatal(err)
	}
	// A source checkout can compile the preview player on first use. Prepare
	// that fixture before timing the launch of the packaged cartridge.
	helper, err := emulator.ResolveHelper()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MTILE_PLAYER", helper)
	t.Setenv("MTILE_TINYGO", filepath.Join(t.TempDir(), "no-compiler"))
	g := &Game{mode: modeCode, runReady: make(chan playLaunch, 1)}
	var s desktopStarter
	s.rom.ROMPath, s.rom.ROMSize = rom, int(info.Size())
	g.playDesktopStarter(s)
	select {
	case launch := <-g.runReady:
		defer launch.discard()
		if launch.err != nil || launch.session == nil || !launch.prebuilt || !g.developGameFocus {
			t.Fatalf("open bundled cartridge: %v", launch.err)
		}
		play := func(keys uint16) []byte {
			t.Helper()
			var frame emulator.Frame
			for range 60 {
				frame, err = launch.session.Step(keys, false)
				if err != nil {
					t.Fatal(err)
				}
			}
			return frame.RGBA
		}
		still := bytes.Clone(play(0))
		if len(still) == 0 || bytes.Equal(still, play(emulator.KeyRight)) {
			t.Fatal("Coin Quest did not respond to movement")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("bundled cartridge did not start")
	}
}
