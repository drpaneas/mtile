// Benchmark-only adapter: normal builds and an independent PCM/telemetry capture.
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/drpaneas/mtile/internal/audioimport"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func save(path string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	check(e)
	check(os.WriteFile(path, append(b, '\n'), 0644))
}
func check(e error) {
	if e != nil {
		panic(e)
	}
}

type observation struct {
	Tick    uint32              `json:"tick"`
	Refresh uint32              `json:"refresh"`
	Stats   emulator.DebugStats `json:"stats"`
}

func main() {
	a := os.Args[1:]
	switch a[0] {
	case "import":
		r, e := audioimport.Import(a[1], a[3:], a[2])
		check(e)
		save(a[1]+"/import.json", r)
	case "batch-import":
		trim, e := strconv.ParseBool(a[4])
		check(e)
		b, e := audioimport.PrepareBatch(a[5:], nil, audioimport.PrepareOptions{Quality: audioimport.Quality(a[3]), TrimSilence: trim}, nil)
		check(e)
		defer b.Close()
		var names []string
		for _, item := range b.Items {
			names = append(names, item.Name)
		}
		r, e := b.Import(a[1], names, a[2])
		check(e)
		save(a[1]+"/import.json", r)
	case "build":
		s, e := build.NewService("", "")
		check(e)
		s.Debug = true
		s.KeepWork = true
		if runtimeDir := os.Getenv("PCM_RUNTIME"); runtimeDir != "" {
			s.RuntimeDir = runtimeDir
		}
		s.DisableHeapElision = os.Getenv("PCM_RETAIN_HEAP") == "1"
		s.Renderer, e = transpiler.ParseRenderer(a[2])
		check(e)
		r, e := s.Build(a[1], a[3])
		check(e)
		defer os.RemoveAll(r.WorkDir)
		for _, n := range []string{"main.go", "game.elf", "game.map", "target-gba.json"} {
			b, e := os.ReadFile(filepath.Join(r.WorkDir, n))
			check(e)
			check(os.WriteFile(a[3]+"."+n, b, 0644))
		}
		r.WorkDir = ""
		r.MapPath = a[3] + ".game.map"
		save(a[3]+".build.json", r)
	case "capture":
		warm, e := strconv.Atoi(a[4])
		check(e)
		count, e := strconv.Atoi(a[5])
		check(e)
		s, e := emulator.Start(a[1], a[2])
		check(e)
		defer s.Close()
		var rows []observation
		var sum, leftSum, rightSum float64
		var n, nonzero, clips, peak, overflows int
		var previous uint32
		for frame := 0; frame < (warm+count+100)*8; frame++ {
			f, e := s.Step(0, false)
			check(e)
			if !f.Telemetry.DebugValid || f.Telemetry.Stats.Frames <= uint32(warm) {
				continue
			}
			if f.Overflow {
				overflows++
			}
			for i := 0; i+3 < len(f.PCM); i += 4 {
				l := int(int16(binary.LittleEndian.Uint16(f.PCM[i:])))
				r := int(int16(binary.LittleEndian.Uint16(f.PCM[i+2:])))
				leftSum += float64(l * l)
				rightSum += float64(r * r)
				x := float64(l+r) / 2
				sum += x * x
				n++
				if l != 0 || r != 0 {
					nonzero++
				}
				p := max(abs(l), abs(r))
				peak = max(peak, p)
				if p >= 32767 {
					clips++
				}
			}
			if f.Telemetry.Stats.Frames != previous {
				rows = append(rows, observation{f.Telemetry.Stats.Frames, f.Number, f.Telemetry.Stats})
				previous = f.Telemetry.Stats.Frames
				if len(rows) == count {
					break
				}
			}
		}
		if len(rows) != count {
			panic(fmt.Sprintf("only %d publications", len(rows)))
		}
		save(a[3], map[string]any{"observations": rows, "pcm": map[string]any{"sampleRate": emulator.SampleRate, "stereoFrames": n, "rmsMono": math.Sqrt(sum / float64(n)), "rmsLeft": math.Sqrt(leftSum / float64(n)), "rmsRight": math.Sqrt(rightSum / float64(n)), "peak": peak, "nonzeroFrames": nonzero, "clippedFrames": clips, "overflows": overflows}})
	default:
		panic("import | build | capture")
	}
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
