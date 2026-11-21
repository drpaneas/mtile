// Command editor is Mtile's single-window GBA IDE: sprites, maps, the
// four-channel Music Lab, and a Develop workspace that combines Lua source
// editing with an embedded Play surface backed by the private headless mGBA
// helper. The PCM Editor prepares and imports optional Direct Sound assets.
package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	dir := flag.String("project", ".", "path to the native asset project directory")
	flag.Parse()

	var starter desktopStarter
	if len(os.Args) == 1 {
		executable, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		starter, err = openDesktopStarter(executable)
		if err != nil {
			log.Fatalf("editor: %v", err)
		}
		if starter.projectDir != "" {
			*dir = starter.projectDir
		}
	}
	g, err := newGame(*dir)
	if err != nil {
		log.Fatalf("editor: %v", err)
	}
	if starter.projectDir != "" {
		g.playDesktopStarter(starter)
	}
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowSizeLimits(minimumScreenWidth, minimumScreenHeight, -1, -1)
	ebiten.SetWindowTitle("mtile editor - " + filepath.Base(*dir))
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	// The editor is mostly static. Keeping the offscreen buffer lets Draw
	// return without touching it on idle ticks, which makes Ebitengine skip the
	// Metal command buffer and swap entirely. Dynamic surfaces (transport,
	// recording, MIDI and the emulator) still request every frame from Update.
	ebiten.SetScreenClearedEveryFrame(false)
	runErr := ebiten.RunGame(&editorWindow{Game: g})
	g.pcmDiscardBatch()
	g.closePlayMode()
	g.closeNativePerformance()
	// Persist before reporting a fatal error too: log.Fatalf calls os.Exit
	// immediately, so an unconditional persist-then-check is the only order
	// that does not risk losing an editing session to whatever triggered
	// RunGame's error return.
	if err := errors.Join(runErr, g.persist()); err != nil {
		log.Fatalf("editor: %v", err)
	}
}
