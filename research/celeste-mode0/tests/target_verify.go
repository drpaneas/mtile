//go:build ignore

// go run examples/celeste/tests/target_verify.go
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
)

func main() {
	project := "examples/celeste"
	if len(os.Args) > 1 {
		project = os.Args[1]
	}
	out, err := os.MkdirTemp("", "celeste-target-")
	check(err)
	service, err := build.NewService("", "")
	check(err)
	service.Debug = true
	service.KeepWork = true
	service.UseProjectRenderer = true
	result, err := service.Build(project, filepath.Join(out, "celeste-debug.gba"))
	check(err)
	fmt.Println("Debug build:", result.WorkDir)
	session, err := emulator.Start("build/player/mtile-player", filepath.Join(out, "celeste-debug.gba"))
	check(err)
	defer session.Close()
	var last emulator.Frame
	var activeFrames, audioFrames, samples int
	var maxWork uint32
	rows := []emulator.DebugStats{}
	// Spawn, walk into spikes, respawn, repeated jump/dash/wall inputs and idle.
	for i := 0; i < 1800; i++ {
		var keys uint16
		if i >= 100 && i < 160 {
			keys = emulator.KeyRight
		}
		if i >= 320 && i < 1400 {
			switch (i - 320) % 120 {
			case 0, 1:
				keys = emulator.KeyB
			case 10, 11:
				keys = emulator.KeyA | emulator.KeyUp | emulator.KeyRight
			default:
				if (i/120)%2 == 0 {
					keys = emulator.KeyRight
				} else {
					keys = emulator.KeyLeft
				}
			}
		}
		last, err = session.Step(keys, false)
		check(err)
		if last.Overflow {
			panic("native audio overflow")
		}
		for _, v := range last.PCM {
			if v != 0 {
				audioFrames++
				break
			}
		}
		if last.Telemetry.PSG.Status&15 != 0 {
			activeFrames++
		}
		if last.Telemetry.DebugValid {
			st := last.Telemetry.Stats
			if st.HeapError != emulator.HeapOK {
				panic(fmt.Sprintf("native heap error at LCD frame %d: %+v", i, st))
			}
			if st.LastFrame > maxWork {
				maxWork = st.LastFrame
			}
			if len(rows) == 0 || rows[len(rows)-1].Frames != st.Frames {
				rows = append(rows, st)
				samples++
			}
		}
		if i == 98 || i == 119 || i == 450 {
			f, err := os.Create(filepath.Join(out, fmt.Sprintf("frame-%d.png", i)))
			check(err)
			check(png.Encode(f, &image.RGBA{Pix: last.RGBA, Stride: 240 * 4, Rect: image.Rect(0, 0, 240, 160)}))
			check(f.Close())
		}
	}
	if samples < 100 {
		panic("missing native execution evidence")
	}
	if audioFrames == 0 || activeFrames == 0 {
		panic("missing native PSG/audio output")
	}
	b, err := json.MarshalIndent(rows, "", "  ")
	check(err)
	check(os.WriteFile(filepath.Join(out, "performance.json"), b, 0644))
	var cycles []int
	var overBudget int
	for _, row := range rows {
		cycles = append(cycles, int(row.LastFrame))
		if row.LastFrame > 561792 {
			overBudget++
		}
	}
	sort.Ints(cycles)
	fmt.Printf("All observed gameplay/lifecycle ticks: median=%d, p95=%d, max=%d cycles; %d/%d above 561792. This reports performance separately from behavior assertions.\n", cycles[len(cycles)/2], cycles[(len(cycles)-1)*95/100], cycles[len(cycles)-1], overBudget, len(cycles))
	fmt.Printf("Native mGBA: %d logical ticks /1800 LCD frames, max observed work=%d/561792 cycles, final stats=%+v, audibleframes=%d, activePSG=%d\n", samples, maxWork, last.Telemetry.Stats, audioFrames, activeFrames)
	fmt.Println("Target evidence:", out)
}
func check(err error) {
	if err != nil {
		panic(err)
	}
}
