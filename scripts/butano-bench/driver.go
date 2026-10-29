// Benchmark-only build/capture adapter. Does not alter either engine.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

type sample struct {
	Tick                 uint32                `json:"tick"`
	VideoFrame           uint32                `json:"videoFrame"`
	Cycles               uint32                `json:"cycles"`
	Update               uint32                `json:"update"`
	Draw                 uint32                `json:"draw"`
	Commit               uint32                `json:"commit"`
	Missed               uint32                `json:"missed"`
	SHA                  string                `json:"sha256"`
	RawScanSHA           string                `json:"rawScanSHA256"`
	PixelPublicationTick uint32                `json:"pixelPublicationTick"`
	Watches              []emulator.Watch      `json:"watches"`
	GC                   uint32                `json:"gc"`
	HeapAllocated        uint32                `json:"heapAllocated"`
	HeapPeak             uint32                `json:"heapPeak"`
	HeapCapacity         uint32                `json:"heapCapacity"`
	HeapLive             uint32                `json:"heapLive"`
	PSG                  emulator.PSGRegisters `json:"psg"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func run() error {
	a := os.Args[1:]
	if len(a) == 0 {
		return fmt.Errorf("build project renderer output | capture player rom out warmup samples")
	}
	if a[0] == "build" {
		if len(a) != 4 {
			return fmt.Errorf("build requires project renderer output")
		}
		s, e := build.NewService("", "")
		if e != nil {
			return e
		}
		s.Debug = true
		s.KeepWork = true
		s.Renderer, e = transpiler.ParseRenderer(a[2])
		if e != nil {
			return e
		}
		r, e := s.Build(a[1], a[3])
		if e != nil {
			return e
		}
		defer os.RemoveAll(r.WorkDir)
		for _, n := range []string{"main.go", "game.elf", "game.map", "target-gba.json"} {
			b, e := os.ReadFile(filepath.Join(r.WorkDir, n))
			if e == nil {
				if e = os.WriteFile(a[3]+"."+n, b, 0644); e != nil {
					return e
				}
			}
		}
		r.WorkDir = ""
		r.MapPath = a[3] + ".game.map"
		return writeJSON(a[3]+".build.json", r)
	}
	if a[0] != "capture" || len(a) != 8 {
		return fmt.Errorf("invalid capture arguments")
	}
	warm, e := strconv.Atoi(a[5])
	if e != nil || warm < 0 {
		return fmt.Errorf("invalid warmup")
	}
	count, e := strconv.Atoi(a[6])
	if e != nil || count < 2 {
		return fmt.Errorf("invalid samples")
	}
	lag, e := strconv.Atoi(a[7])
	if e != nil || lag < 0 {
		return fmt.Errorf("invalid pixel lag")
	}
	s, e := emulator.Start(a[1], a[2])
	if e != nil {
		return e
	}
	defer s.Close()
	samples := make([]sample, 0, count)
	var pending *sample
	var pixels []byte
	var lastPixels []byte
	var firstDisplay uint32
	var warmupMissed uint32
	for step := 0; step < (warm+count+100)*32; step++ {
		f, e := s.Step(0, false)
		if e != nil {
			return e
		}
		if !f.Telemetry.DebugValid {
			if step >= 240 && firstDisplay == 0 {
				return fmt.Errorf("no debug mailbox after 240 display refreshes")
			}
			continue
		}
		st := f.Telemetry.Stats
		if int(st.Frames) <= warm {
			warmupMissed = st.MissedFrames
		}
		if firstDisplay == 0 {
			firstDisplay = f.Number
		}
		// Keep the FIRST complete scan per mailbox publication. mTile publishes
		// in VBlank; the first completed scan carries the same game tick.
		// Butano publishes after its second refresh. Both use zero lag.
		// The explicit lag is verified with deterministic moving fixtures.
		if pending != nil && pending.Tick != st.Frames {
			if int(pending.Tick) > warm {
				samples = append(samples, *pending)
				lastPixels = append(lastPixels[:0], pixels...)
			}
			pending = nil
			if len(samples) == count+lag {
				break
			}
		}
		if pending != nil {
			continue
		}
		hash := sha256.Sum256(f.RGBA)
		pending = &sample{Tick: st.Frames, VideoFrame: f.Number, Cycles: st.LastFrame, Update: st.LastUpdate, Draw: st.LastDraw, Commit: st.LastBlit, Missed: st.MissedFrames, SHA: hex.EncodeToString(hash[:]), Watches: f.Telemetry.Watches, GC: st.GCCount, HeapCapacity: st.HeapCapacity, HeapLive: st.HeapLive, HeapAllocated: st.HeapAllocated, HeapPeak: st.HeapPeakLive, PSG: f.Telemetry.PSG}
		pixels = append(pixels[:0], f.RGBA...)
	}
	if len(samples) != count+lag {
		return fmt.Errorf("captured %d/%d", len(samples), count)
	}
	for i := 0; i < count; i++ {
		samples[i].RawScanSHA = samples[i].SHA
		samples[i].SHA = samples[i+lag].SHA
		samples[i].PixelPublicationTick = samples[i+lag].Tick
	}
	samples = samples[:count]
	if e = writeJSON(a[3], map[string]any{"firstPublicationVideoFrame": firstDisplay, "warmupMissed": warmupMissed, "samples": samples}); e != nil {
		return e
	}
	out, e := os.Create(a[4])
	if e != nil {
		return e
	}
	defer out.Close()
	return png.Encode(out, &image.RGBA{Pix: lastPixels, Stride: 240 * 4, Rect: image.Rect(0, 0, 240, 160)})
}
