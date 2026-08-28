// Command editor is Mtile's single-window GBA IDE: sprites, maps, the
// four-channel Music Lab, and a Develop workspace that combines Lua source
// editing with an embedded Play surface backed by the private headless mGBA
// helper. Direct Sound/PCM is intentionally absent from the authoring model.
package main

import (
	"flag"
	"log"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	dir := flag.String("project", ".", "path to the native asset project directory")
	flag.Parse()

	g, err := newGame(*dir)
	if err != nil {
		log.Fatalf("editor: %v", err)
	}
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowSizeLimits(minimumScreenWidth, minimumScreenHeight, -1, -1)
	ebiten.SetWindowTitle("mtile editor - " + filepath.Base(*dir))
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	runErr := ebiten.RunGame(g)
	g.closePlayMode()
	g.closeNativePerformance()
	// Persist before reporting a fatal error too: log.Fatalf calls os.Exit
	// immediately, so an unconditional persist-then-check is the only order
	// that does not risk losing an editing session to whatever triggered
	// RunGame's error return.
	g.persist()
	if runErr != nil {
		log.Fatalf("editor: %v", runErr)
	}
}
