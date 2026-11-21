package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/drpaneas/mtile/internal/build"
)

type desktopStarter struct {
	projectDir string
	rom        build.Result // Populated only for a freshly created project.
}

// The release places a compiled Coin Quest cartridge in its resource directory.
// A source-built editor still opens the current
// project when started without arguments.
func openDesktopStarter(executable string) (desktopStarter, error) {
	resources := filepath.Dir(executable)
	if runtime.GOOS == "darwin" && filepath.Base(resources) == "MacOS" && filepath.Base(filepath.Dir(resources)) == "Contents" {
		resources = filepath.Join(filepath.Dir(resources), "Resources")
	}
	romPath := filepath.Join(resources, "coin-quest.gba")
	info, err := os.Stat(romPath)
	if errors.Is(err, os.ErrNotExist) {
		return desktopStarter{}, nil
	}
	if err != nil {
		return desktopStarter{}, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return desktopStarter{}, fmt.Errorf("invalid bundled Coin Quest ROM at %s", romPath)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return desktopStarter{}, err
	}
	dir := filepath.Join(home, "mTile Projects", "Coin Quest")
	starter := desktopStarter{projectDir: dir}
	// CreateProject publishes the complete template by renaming its staging
	// directory. A concurrent launch can open the winner's project immediately.
	err = build.CreateProject(dir, build.NewProjectOptions{Template: "coin-quest"})
	if err != nil {
		if existing, statErr := os.Stat(dir); statErr == nil && existing.IsDir() {
			return starter, nil
		}
		return desktopStarter{}, err
	}
	starter.rom = build.Result{ROMPath: romPath, ROMSize: int(info.Size())}
	return starter, nil
}

func (g *Game) playDesktopStarter(starter desktopStarter) {
	g.developGameFocus = true
	if starter.rom.ROMPath == "" {
		// Returning users may have changed code, sprites or any other input.
		g.startPreviewBuild("", nil, false)
		return
	}
	g.runMu.Lock()
	g.runID++
	id := g.runID
	g.runBusy = true
	g.runStatus = "OPENING COIN QUEST..."
	g.runMu.Unlock()
	go g.launchPreviewROM(playLaunch{id: id, result: starter.rom, prebuilt: true})
}
