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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
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
	err               error
}

func main() {
	frames := flag.Int("frames", 1800, "mGBA frames to run per project (60 per second)")
	helper := flag.String("player", "build/player/mtile-player", "headless mtile-player path")
	out := flag.String("out", "", "ROM output directory (default: temporary directory)")
	flag.StringVar(&inputMode, "input", "explore", "input script: explore (title presses, then cycling directions) or right (hold right, press A)")
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

	var reports []report
	for _, project := range projects {
		r := survey(service, *helper, project, filepath.Join(dir, filepath.Base(project)+".gba"), *frames, uint32(*spikes))
		reports = append(reports, r)
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", project, r.err)
		}
	}
	print(reports)
}

func survey(service build.Service, helper, project, rom string, frames int, spikes uint32) report {
	r := report{project: project}
	result, err := service.Build(project, rom)
	if err != nil {
		r.err = err
		return r
	}
	r.renderer = result.Renderer.String()
	r.iwram = result.IWRAMUsed
	session, err := emulator.Start(helper, rom)
	if err != nil {
		r.err = err
		return r
	}
	defer session.Close()

	var lastFrames, missedAtStart uint32
	for i := range frames {
		frame, err := session.Step(inputAt(i), false)
		if err != nil {
			r.err = err
			return r
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
	return r
}

// inputAt presses Start and A to leave title screens, then cycles through
// directions with periodic A/B presses so movement, scrolling, and actions run.
var inputMode string

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

func print(reports []report) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "project\trenderer\tticks\tmissed\tmedian\tp95\tmax\tbudget%\tupd p95\tdraw p95\tgc max\theap peak\tiwram\t")
	for _, r := range reports {
		if r.err != nil {
			fmt.Fprintf(w, "%s\terror\t\t\t\t\t\t\t\t\t\t\t\t\n", filepath.Base(r.project))
			continue
		}
		p95 := percentile(r.frame, 0.95)
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%.0f%%\t%d\t%d\t%d\t%s\t%d\t\n",
			filepath.Base(r.project), r.renderer, r.ticks, r.missed,
			percentile(r.frame, 0.5), p95, r.maxFrame, 100*float64(p95)/tickBudget,
			percentile(r.update, 0.95), percentile(r.draw, 0.95), slices.Max(append(r.gc, 0)),
			fmt.Sprintf("%d/%d", r.heapPeak, r.heapCap), r.iwram)
	}
	w.Flush()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
