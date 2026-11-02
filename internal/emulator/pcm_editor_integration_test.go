package emulator_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/audioimport"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This uses the exact wizard preparation/import/delete path and then runs its
// resulting Maxmod bank on the GBA, including holes in both public ID maps.
func TestPCMEditorPreparedAudioROMConformance(t *testing.T) {
	if os.Getenv("MTILE_PCM_EDITOR_INTEGRATION") != "1" {
		t.Skip("set MTILE_PCM_EDITOR_INTEGRATION=1 and MMUTIL for GBA playback proof")
	}
	tool := os.Getenv("MMUTIL")
	if tool == "" {
		t.Fatal("MMUTIL required")
	}
	helper, err := emulator.ResolveHelper()
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "game")
	if err := os.CopyFS(project, os.DirFS("../../examples/imported-audio")); err != nil {
		t.Fatal(err)
	}
	// Start from a fresh imported companion, preserving the project's PSG data.
	if err := os.RemoveAll(filepath.Join(project, "assets/audio/direct")); err != nil {
		t.Fatal(err)
	}
	rate, frames := 48000, 24000
	raw := make([]byte, 44+frames*4)
	copy(raw, "RIFF")
	binary.LittleEndian.PutUint32(raw[4:], uint32(len(raw)-8))
	copy(raw[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(raw[16:], 16)
	binary.LittleEndian.PutUint16(raw[20:], 1)
	binary.LittleEndian.PutUint16(raw[22:], 2)
	binary.LittleEndian.PutUint32(raw[24:], uint32(rate))
	binary.LittleEndian.PutUint32(raw[28:], uint32(rate*4))
	binary.LittleEndian.PutUint16(raw[32:], 4)
	binary.LittleEndian.PutUint16(raw[34:], 16)
	copy(raw[36:], "data")
	binary.LittleEndian.PutUint32(raw[40:], uint32(len(raw)-44))
	for i := 0; i < frames; i++ {
		if i < rate/8 || i >= 3*rate/8 {
			continue
		}
		v := uint16(int16(20000 * math.Sin(2*math.Pi*1000*float64(i)/float64(rate))))
		binary.LittleEndian.PutUint16(raw[44+i*4:], v)
		binary.LittleEndian.PutUint16(raw[46+i*4:], v)
	}
	source := filepath.Join(t.TempDir(), "jump.wav")
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	sources := t.TempDir()
	removedWave := filepath.Join(sources, "removed_sample.wav")
	if err := os.WriteFile(removedWave, raw, 0600); err != nil {
		t.Fatal(err)
	}
	songRaw, err := os.ReadFile("../../examples/imported-audio/assets/audio/direct/source/music_theme.mod")
	if err != nil {
		t.Fatal(err)
	}
	removedSong, liveSong := filepath.Join(sources, "removed_theme.mod"), filepath.Join(sources, "theme.mod")
	for _, path := range []string{removedSong, liveSong} {
		if err := os.WriteFile(path, songRaw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := audioimport.PrepareBatch([]string{removedWave, source, removedSong, liveSong}, nil, audioimport.PrepareOptions{Quality: audioimport.QualityBalanced, TrimSilence: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Close()
	p, err := batch.Load(0)
	if err != nil || !p.Trimmed || math.Abs(p.After.Seconds()-.29) > .001 {
		t.Fatalf("trimmed batch fixture: %.4f %v", p.After.Seconds(), err)
	}
	if _, err = batch.Import(project, []string{"removed_sample", "jump", "removed_theme", "theme"}, tool); err != nil {
		t.Fatal(err)
	}
	if _, err = audioimport.Delete(project, false, 0, tool); err != nil {
		t.Fatal(err)
	}
	if _, err = audioimport.Delete(project, true, 0, tool); err != nil {
		t.Fatal(err)
	}
	// A build must embed the published bank without running the importer again.
	t.Setenv("MMUTIL", filepath.Join(t.TempDir(), "unavailable-mmutil"))
	lua := `ticks=0
function _init() local retired=0 sample(retired) pcm_music(retired) end
function _update()
 ticks=ticks+1
 if ticks==10 then sample(1) end
 if ticks==30 then pcm_music(1) end
 if ticks==60 then sample(-1) pcm_music(-1) end
end
function _draw() cls(1) end
`
	if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(lua), 0600); err != nil {
		t.Fatal(err)
	}
	var reference []byte
	for _, mode := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0} {
		t.Run(mode.String(), func(t *testing.T) {
			service, err := build.NewService("", "")
			if err != nil {
				t.Fatal(err)
			}
			service.Renderer = mode
			service.Debug = true
			rom := filepath.Join(t.TempDir(), "pcm-editor.gba")
			if _, err = service.Build(project, rom); err != nil {
				t.Fatal(err)
			}
			session, err := emulator.Start(helper, rom)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			var early, wav, song, tail []float64
			var pixels []byte
			updates := uint32(0)
			for i := 0; i < 180; i++ {
				frame, err := session.Step(0, false)
				if err != nil {
					t.Fatal(err)
				}
				if frame.Overflow {
					t.Fatal("audio overflow")
				}
				var dst *[]float64
				switch {
				case i >= 8 && i < 16:
					dst = &early
				case i >= 24 && i < 32:
					dst = &wav
				case i >= 75 && i < 95:
					dst = &song
				case i >= 155:
					dst = &tail
				}
				if dst != nil {
					for off := 0; off+3 < len(frame.PCM); off += 4 {
						left := int16(binary.LittleEndian.Uint16(frame.PCM[off:]))
						right := int16(binary.LittleEndian.Uint16(frame.PCM[off+2:]))
						*dst = append(*dst, float64(int(left)+int(right))/2)
					}
				}
				if frame.Telemetry.DebugValid {
					updates = uint32(frame.Telemetry.Stats.Frames)
				}
				pixels = append(pixels[:0], frame.RGBA...)
			}
			rms := func(s []float64) float64 {
				sum := 0.0
				for _, v := range s {
					sum += v * v
				}
				if len(s) == 0 {
					return 0
				}
				return math.Sqrt(sum / float64(len(s)))
			}
			if updates < 60 || rms(wav) < 30 || rms(song) < 30 || rms(early) > 5 || rms(tail) > 5 {
				t.Fatalf("updates=%d RMS early=%g wav=%g song=%g tail=%g", updates, rms(early), rms(wav), rms(song), rms(tail))
			}
			// Estimate the actual emitted tone using positive-going zero crossings.
			crossings := 0
			for i := 1; i < len(wav); i++ {
				if wav[i-1] <= 0 && wav[i] > 0 {
					crossings++
				}
			}
			hz := float64(crossings) * emulator.SampleRate / float64(len(wav))
			if math.Abs(hz-1000) > 55 {
				t.Fatalf("prepared WAV pitch changed: %.1f Hz", hz)
			}
			if reference == nil {
				reference = append([]byte(nil), pixels...)
			} else if !bytes.Equal(reference, pixels) {
				t.Fatal("renderer pixels differ")
			}
			t.Logf("prepared WAV: %.1f Hz, RMS %.1f; tracker RMS %.1f; deleted IDs silent; %d updates", hz, rms(wav), rms(song), updates)
		})
	}
	t.Run("all_deleted", func(t *testing.T) {
		if _, err := audioimport.Delete(project, false, 1, tool); err != nil {
			t.Fatal(err)
		}
		if _, err := audioimport.Delete(project, true, 1, tool); err != nil {
			t.Fatal(err)
		}
		source := `function _init() local removed=0 sample(removed) pcm_music(removed) end function _draw() cls(1) end`
		if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		service, err := build.NewService("", "")
		if err != nil {
			t.Fatal(err)
		}
		service.Renderer = transpiler.RendererMode0
		service.Debug = true
		rom := filepath.Join(t.TempDir(), "empty-bank.gba")
		if _, err := service.Build(project, rom); err != nil {
			t.Fatal(err)
		}
		session, err := emulator.Start(helper, rom)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		last := uint32(0)
		for i := 0; i < 60; i++ {
			frame, err := session.Step(0, false)
			if err != nil {
				t.Fatal(err)
			}
			if frame.Telemetry.DebugValid {
				last = uint32(frame.Telemetry.Stats.Frames)
			}
		}
		if last < 20 {
			t.Fatal("empty bank stopped the game", last)
		}
	})
}
