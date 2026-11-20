package emulator_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This is a GBA/Maxmod test, not a desktop substitute. Enable it with the
// pinned mtile-player and TinyGo available. Fixtures contain original audio.
func TestImportedAudioROMConformance(t *testing.T) {
	if os.Getenv("MTILE_IMPORTED_AUDIO_INTEGRATION") != "1" {
		t.Skip("set MTILE_IMPORTED_AUDIO_INTEGRATION=1 for real ROM audio and pixel checks")
	}
	helper, err := emulator.ResolveHelper()
	if err != nil {
		t.Fatal(err)
	}
	var reference []byte
	for _, mode := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0} {
		t.Run(mode.String(), func(t *testing.T) {
			project := filepath.Join(t.TempDir(), "game")
			if err := os.CopyFS(project, os.DirFS("../../examples/imported-audio")); err != nil {
				t.Fatal(err)
			}
			source := `ticks=0
stamp=0
function _init() pcm_music(0) end
function _update()
 ticks=ticks+1
 stamp=t()
 if ticks==20 then sample(0) end
 if ticks==35 then gba_note(1,69,0,48) end
 if ticks==50 then gba_stop(1) end
 if ticks==60 then pcm_music(-1) sample(-1) end
end
function _draw() cls(1) end
`
			if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			service, err := build.NewService("", "")
			if err != nil {
				t.Fatal(err)
			}
			service.Renderer = mode
			service.Debug = true
			rom := filepath.Join(t.TempDir(), "audio.gba")
			result, err := service.Build(project, rom)
			if err != nil {
				t.Fatal(err)
			}
			session, err := emulator.Start(helper, rom)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := session.Close(); err != nil {
					t.Error(err)
				}
			}()
			var early, wav, psg, tail []float64
			previousTick, lastChange := -1, -1
			events, psgActive := 0, false
			var lastPixels []byte
			for i := 0; i < 240; i++ {
				frame, err := session.Step(0, false)
				if err != nil {
					t.Fatal(err)
				}
				if frame.Overflow {
					t.Fatal("audio capture overflow")
				}
				mono := func(dst *[]float64) {
					for j := 0; j+3 < len(frame.PCM); j += 4 {
						left := int16(binary.LittleEndian.Uint16(frame.PCM[j:]))
						right := int16(binary.LittleEndian.Uint16(frame.PCM[j+2:]))
						*dst = append(*dst, float64(int(left)+int(right))/2)
					}
				}
				switch {
				case i >= 20 && i < 35:
					mono(&early)
				case i >= 45 && i < 60:
					mono(&wav)
				case i >= 78 && i < 90:
					mono(&psg)
				case i >= 200:
					mono(&tail)
				}
				if !frame.Telemetry.DebugValid {
					continue
				}
				if frame.Telemetry.PSG.Status&1 != 0 {
					psgActive = true
				}
				if i > 20 && frame.Telemetry.PSG.MixDMA&0x4400 != 0x4400 {
					t.Fatal("PCM lost Timer 1 routing while PSG was active")
				}
				tick := int(frame.Telemetry.Stats.Frames)
				if tick != previousTick {
					if i > 30 && lastChange > 25 && i-lastChange != 2 {
						t.Fatalf("30 FPS cadence: refresh gap %d at refresh %d", i-lastChange, i)
					}
					previousTick, lastChange = tick, i
					events++
				}
				if frame.Telemetry.Stats.HeapError != 0 {
					t.Fatal("Lua heap error")
				}
				if i == 230 {
					lastPixels = append([]byte(nil), frame.RGBA...)
				}
			}
			rms := func(a []float64) float64 {
				var sum float64
				for _, x := range a {
					sum += x * x
				}
				return math.Sqrt(sum / float64(max(1, len(a))))
			}
			power := func(a []float64, hz float64) float64 {
				var re, im float64
				for i, x := range a {
					phase := 2 * math.Pi * hz * float64(i) / emulator.SampleRate
					re += x * math.Cos(phase)
					im += x * math.Sin(phase)
				}
				return (re*re + im*im) / float64(max(1, len(a)*len(a)))
			}
			if rms(early) < 100 || rms(tail) > 5 {
				t.Fatalf("music start/stop RMS: before %.2f after %.2f", rms(early), rms(tail))
			}
			if power(wav, 880) < power(early, 880)*10 {
				t.Fatalf("imported WAV bell missing: 880 Hz power %.2f -> %.2f", power(early, 880), power(wav, 880))
			}
			if !psgActive || power(psg, 440) < power(early, 440)*5 {
				t.Fatalf("PSG/PCM coexistence failed: status=%t 440Hz %.2f -> %.2f", psgActive, power(early, 440), power(psg, 440))
			}
			if events < 100 || events > 125 || len(lastPixels) == 0 {
				t.Fatalf("game did not sustain 30 FPS: %d updates", events)
			}
			if reference == nil {
				reference = lastPixels
			} else if !bytes.Equal(reference, lastPixels) {
				t.Fatal("Mode 0/Mode 4 pixels differ")
			}
			t.Logf("ROM=%d EWRAM free=%d IWRAM=%d; %d updates; music RMS %.1f tail %.3f; WAV880 %.1f PSG440 %.1f", result.ROMSize, result.EWRAMFree, result.IWRAMUsed, events, rms(early), rms(tail), power(wav, 880), power(psg, 440))
		})
	}
}
