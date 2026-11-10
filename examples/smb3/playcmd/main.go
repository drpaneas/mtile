// Command playcmd runs the local SMB3 compatibility cartridge through mTile's GBA player.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
)

const (
	hostTPS           = 60
	gbaClock          = 16777216
	gbaCyclesPerFrame = 280896
	frameUnits        = gbaCyclesPerFrame * hostTPS
	windowScale       = 3
)

var bindings = [...]struct {
	key  ebiten.Key
	mask uint16
}{
	{ebiten.KeyArrowRight, emulator.KeyRight},
	{ebiten.KeyArrowLeft, emulator.KeyLeft},
	{ebiten.KeyArrowUp, emulator.KeyUp},
	{ebiten.KeyArrowDown, emulator.KeyDown},
	{ebiten.KeyZ, emulator.KeyA},
	{ebiten.KeyX, emulator.KeyB},
	{ebiten.KeyEnter, emulator.KeyStart},
	{ebiten.KeyBackspace, emulator.KeySelect},
	{ebiten.KeyA, emulator.KeyL},
	{ebiten.KeyS, emulator.KeyR},
}

type game struct {
	session *emulator.Session
	pcm     *emulator.PCMStream
	picture *ebiten.Image
	clock   int
	keys    uint16
}

func (g *game) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	for _, binding := range bindings {
		if ebiten.IsKeyPressed(binding.key) {
			g.keys |= binding.mask
		}
	}
	// Carry the GBA clock remainder across host ticks to avoid 60 Hz drift.
	g.clock += gbaClock
	if g.clock < frameUnits {
		return nil
	}
	g.clock -= frameUnits
	frame, err := g.session.Step(g.keys, false)
	if err != nil {
		return fmt.Errorf("step cartridge: %w", err)
	}
	g.keys = 0
	g.picture.WritePixels(frame.RGBA)
	_, err = g.pcm.Write(frame.PCM)
	return err
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.DrawImage(g.picture, nil)
}

func (g *game) Layout(int, int) (int, int) {
	return emulator.Width, emulator.Height
}

func run(rom string) error {
	helper, err := emulator.ResolveHelper()
	if err != nil {
		return err
	}
	session, err := emulator.Start(helper, rom)
	if err != nil {
		return err
	}
	defer session.Close()

	pcm := emulator.NewPCMStream(100)
	defer pcm.Close()
	player, err := audio.NewContext(emulator.SampleRate).NewPlayer(pcm)
	if err != nil {
		return fmt.Errorf("start audio: %w", err)
	}
	defer player.Close()
	player.SetBufferSize(35 * time.Millisecond)
	player.Play()

	ebiten.SetTPS(hostTPS)
	ebiten.SetWindowTitle("Super Mario Bros. 3 | mTile")
	ebiten.SetWindowSize(windowScale*emulator.Width, windowScale*emulator.Height)
	ebiten.SetScreenFilterEnabled(false)
	return ebiten.RunGame(&game{
		session: session,
		pcm:     pcm,
		picture: ebiten.NewImage(emulator.Width, emulator.Height),
		clock:   frameUnits - gbaClock,
	})
}

func main() {
	log.SetFlags(0)
	if len(os.Args) != 2 {
		log.Fatal("usage: go run ./examples/smb3/playcmd path/to/game.gba")
	}
	if err := run(os.Args[1]); err != nil {
		log.Fatal(err)
	}
}
