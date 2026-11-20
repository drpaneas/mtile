// Command perfsurvey builds projects as debug ROMs with their own renderer
// policy, plays a fixed input script in the headless mGBA helper, and reports
// per-tick cycle costs and missed frames from the runtime debug mailbox.
//
// Run from the repository root:
//
//	go run ./cmd/perfsurvey examples/pong examples/lumen
//
// With no arguments it surveys every directory under examples/ that has a
// project.json.
package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"text/tabwriter"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// tickBudget is one 30 Hz update: two 280,896-cycle GBA frames.
const tickBudget = 2 * 280896

type report struct {
	project, renderer string
	ticks, missed     uint32
	frame, update     []uint32
	draw, gc          []uint32
	maxFrame          uint32
	heapPeak, heapCap uint32
	iwram             uint64
	rom               int
	err               error
}

func main() {
	frames := flag.Int("frames", 1800, "mGBA frames to run per project (60 per second)")
	helper := flag.String("player", "build/player/mtile-player", "headless mtile-player path")
	out := flag.String("out", "", "ROM output directory (default: temporary directory)")
	flag.StringVar(&inputMode, "input", "explore", "input script: explore (title presses, then cycling directions) or right (hold right, press A)")
	flag.StringVar(&hashDir, "hashes", "", "write one SHA-256 per displayed frame to <dir>/<project>.txt for pixel-parity comparisons")
	flag.Func("dump", "write this 0-based frame as PNG into the -hashes directory (repeatable)", func(v string) error {
		n, err := strconv.Atoi(v)
		dumpFrames = append(dumpFrames, n)
		return err
	})
	flag.BoolVar(&tickInput, "tick-input", false, "key the input script to game ticks instead of display frames, so timing changes cannot change game state")
	renderer := flag.String("renderer", "", "override every project's renderer: mode4, mode0, or auto")
	spikes := flag.Uint("spikes", 0, "also print every sampled tick costing at least this many cycles")
	flag.Parse()

	projects := flag.Args()
	if len(projects) == 0 {
		matches, _ := filepath.Glob("examples/*/project.json")
		for _, m := range matches {
			projects = append(projects, filepath.Dir(m))
		}
	}
	dir := *out
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "mtile-perfsurvey-"); err != nil {
			fatal(err)
		}
	}
	service, err := build.NewService("", "")
	if err != nil {
		fatal(err)
	}
	service.Debug = true
	service.UseProjectRenderer = true
	if *renderer != "" {
		if service.Renderer, err = transpiler.ParseRenderer(*renderer); err != nil {
			fatal(err)
		}
		service.UseProjectRenderer = false
	}

	var reports []report
	for _, project := range projects {
		r := survey(service, *helper, project, filepath.Join(dir, filepath.Base(project)+".gba"), *frames, uint32(*spikes))
		reports = append(reports, r)
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", project, r.err)
		}
	}
	if err := print(reports); err != nil {
		fatal(err)
	}
}

func survey(service build.Service, helper, project, rom string, frames int, spikes uint32) (r report) {
	r.project = project
	result, err := service.Build(project, rom)
	if err != nil {
		r.err = err
		return r
	}
	r.renderer = result.Renderer.String()
	r.iwram = result.IWRAMUsed
	r.rom = result.ROMSize
	session, err := emulator.Start(helper, rom)
	if err != nil {
		r.err = err
		return r
	}
	defer func() { r.err = errors.Join(r.err, session.Close()) }()
	var hashes *os.File
	if hashDir != "" {
		if hashes, err = os.Create(filepath.Join(hashDir, filepath.Base(project)+".txt")); err != nil {
			r.err = err
			return r
		}
		defer func() { r.err = errors.Join(r.err, hashes.Close()) }()
	}

	var lastFrames, missedAtStart uint32
	for i := range frames {
		at := i
		if tickInput {
			at = 2 * int(lastFrames)
		}
		frame, err := session.Step(inputAt(at), false)
		if err != nil {
			r.err = err
			return r
		}
		if hashes != nil {
			if _, err := fmt.Fprintf(hashes, "%x\n", sha256.Sum256(frame.RGBA)); err != nil {
				r.err = err
				return r
			}
			if slices.Contains(dumpFrames, i) {
				img := &image.RGBA{Pix: frame.RGBA, Stride: 4 * 240, Rect: image.Rect(0, 0, 240, 160)}
				f, err := os.Create(filepath.Join(hashDir, fmt.Sprintf("%s-%d.png", filepath.Base(project), i)))
				if err != nil {
					r.err = err
					return r
				}
				if err := errors.Join(png.Encode(f, img), f.Close()); err != nil {
					r.err = fmt.Errorf("write frame %d: %w", i, err)
					return r
				}
			}
		}
		t := frame.Telemetry
		if !t.DebugValid {
			continue
		}
		s := t.Stats
		if i == 120 {
			missedAtStart = s.MissedFrames
		}
		r.missed = s.MissedFrames - missedAtStart
		r.heapPeak, r.heapCap = s.HeapPeakLive, s.HeapCapacity
		// Skip the first two seconds: cold uploads and title fades.
		if s.Frames != lastFrames && i >= 120 {
			r.ticks++
			r.frame = append(r.frame, s.LastFrame)
			r.update = append(r.update, s.LastUpdate)
			r.draw = append(r.draw, s.LastDraw)
			r.gc = append(r.gc, s.LastGC)
			r.maxFrame = max(r.maxFrame, s.LastFrame)
			if spikes > 0 && s.LastFrame >= spikes {
				fmt.Printf("%s frame %d: total %d update %d draw %d gc %d\n",
					filepath.Base(project), i, s.LastFrame, s.LastUpdate, s.LastDraw, s.LastGC)
			}
		}
		lastFrames = s.Frames
	}
	for _, line := range session.DrainLogs() {
		fmt.Fprintf(os.Stderr, "%s: %s\n", filepath.Base(project), line)
	}
	return r
}

// inputAt presses Start and A to leave title screens, then cycles through
// directions with periodic A/B presses so movement, scrolling, and actions run.
var (
	inputMode, hashDir string
	tickInput          bool
	dumpFrames         []int
)

func inputAt(frame int) uint16 {
	const (
		a, b, start           = emulator.KeyA, emulator.KeyB, emulator.KeyStart
		right, left, up, down = emulator.KeyRight, emulator.KeyLeft, emulator.KeyUp, emulator.KeyDown
	)
	switch {
	case frame < 60:
		return 0
	case frame < 240:
		if frame%40 < 4 {
			if frame%80 < 4 {
				return start
			}
			return a
		}
		return 0
	}
	if inputMode == "right" {
		keys := uint16(right)
		if frame%45 < 6 {
			keys |= a
		}
		return keys
	}
	directions := []uint16{right, right, down, left, left, up}
	keys := directions[(frame/90)%len(directions)]
	if frame%45 < 6 {
		keys |= a
	}
	if frame%150 < 4 {
		keys |= b
	}
	return keys
}

func percentile(values []uint32, p float64) uint32 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sorted[int(float64(len(sorted)-1)*p)]
}

func print(reports []report) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	if _, err := fmt.Fprintln(w, "project\trenderer\tticks\tmissed\tmedian\tp95\tmax\tbudget%\tupd p95\tdraw p95\tgc max\theap peak\tiwram\trom\t"); err != nil {
		return err
	}
	for _, r := range reports {
		if r.err != nil {
			if _, err := fmt.Fprintf(w, "%s\terror\t\t\t\t\t\t\t\t\t\t\t\t\t\n", filepath.Base(r.project)); err != nil {
				return err
			}
			continue
		}
		p95 := percentile(r.frame, 0.95)
		if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%.0f%%\t%d\t%d\t%d\t%s\t%d\t%d\t\n",
			filepath.Base(r.project), r.renderer, r.ticks, r.missed,
			percentile(r.frame, 0.5), p95, r.maxFrame, 100*float64(p95)/tickBudget,
			percentile(r.update, 0.95), percentile(r.draw, 0.95), slices.Max(append(r.gc, 0)),
			fmt.Sprintf("%d/%d", r.heapPeak, r.heapCap), r.iwram, r.rom); err != nil {
			return err
		}
	}
	return w.Flush()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
