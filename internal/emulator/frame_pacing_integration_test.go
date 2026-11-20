package emulator_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This gate exercises real LCD/IRQ timing. A host VCOUNT stub cannot reproduce
// a mixer interrupt that spans every visible scanline before returning.
func TestFramePacingROMConformance(t *testing.T) {
	if os.Getenv("MTILE_FRAME_PACING_INTEGRATION") != "1" {
		t.Skip("set MTILE_FRAME_PACING_INTEGRATION=1 for native LCD/HALT regression checks")
	}
	helper, err := emulator.ResolveHelper()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name, init, update string
		audible            bool
	}{
		{"silent", "", "", false},
		{"pcm32", "pcm_music(3)", "", true},
		{"pcm32_psg", "pcm_music(3)", "if ticks%60==1 then gba_note(1,69,0,120) end", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var reference map[uint32][32]byte
			for _, mode := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
				t.Run(mode.String(), func(t *testing.T) {
					project := filepath.Join(t.TempDir(), "game")
					if err := os.CopyFS(project, os.DirFS("../../examples/audio-only")); err != nil {
						t.Fatal(err)
					}
					var checker [][]int
					for y := 0; y < 8; y++ {
						row := make([]int, 8)
						for x := range row {
							row[x] = 2 + (x+y)%2
						}
						checker = append(checker, row)
					}
					sheet := map[string]any{"width": 16, "height": 8, "sprites": []any{map[string]any{"id": 1, "x": 8, "y": 0, "width": 8, "height": 8, "pixels": checker, "flags": 0}}}
					imageJSON, err := json.Marshal(sheet)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(project, "assets/spritesheet.json"), imageJSON, 0644); err != nil {
						t.Fatal(err)
					}
					source := fmt.Sprintf(`ticks=0
px=4
function _init() %s end
function _update()
 ticks=ticks+1
 px=4+(ticks*3)%%220
 %s
end
function _draw() cls(1) spr(1,px,60) end
`, fixture.init, fixture.update)
					if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0644); err != nil {
						t.Fatal(err)
					}
					s, err := build.NewService("", "")
					if err != nil {
						t.Fatal(err)
					}
					s.Renderer = mode
					s.Debug = true
					if override := os.Getenv("MTILE_FRAME_PACING_RUNTIME"); override != "" {
						s.RuntimeDir = override
					}
					rom := filepath.Join(t.TempDir(), "pacing.gba")
					if _, err := s.Build(project, rom); err != nil {
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
					pixels := map[uint32][32]byte{}
					var previousTick, previousRefresh uint32
					var intervals, pcmSamples, nonzero int
					var psg bool
					for refresh := 0; refresh < 1600; refresh++ {
						f, err := session.Step(0, false)
						if err != nil {
							t.Fatal(err)
						}
						if f.Overflow {
							t.Fatal("audio capture overflow")
						}
						if !f.Telemetry.DebugValid {
							continue
						}
						st := f.Telemetry.Stats
						if st.Frames <= 60 {
							continue
						}
						if st.HeapCapacity != 0 || st.HeapAllocated != 0 || st.GCCount != 0 || st.HeapError != 0 {
							t.Fatalf("unexpected heap state: %+v", st)
						}
						psg = psg || f.Telemetry.PSG.Status&1 != 0
						for i := 0; i+1 < len(f.PCM); i += 2 {
							pcmSamples++
							if int16(binary.LittleEndian.Uint16(f.PCM[i:])) != 0 {
								nonzero++
							}
						}
						if st.Frames != previousTick {
							if previousTick != 0 {
								if st.Frames != previousTick+1 || f.Number-previousRefresh != 2 {
									t.Fatalf("cadence tick %d -> %d: refresh %d -> %d; stats=%+v", previousTick, st.Frames, previousRefresh, f.Number, st)
								}
								intervals++
							}
							previousTick, previousRefresh = st.Frames, f.Number
							pixels[st.Frames] = sha256.Sum256(f.RGBA)
							if intervals == 599 {
								break
							}
						}
					}
					if intervals != 599 {
						t.Fatalf("only %d update intervals", intervals)
					}
					if fixture.audible && nonzero < pcmSamples/2 {
						t.Fatalf("PCM lost: %d/%d nonzero", nonzero, pcmSamples)
					}
					if !fixture.audible && nonzero != 0 {
						t.Fatal("silent fixture produced audio")
					}
					if fixture.name == "pcm32_psg" && !psg {
						t.Fatal("PSG stopped while PCM and HALT were active")
					}
					unique := map[[32]byte]bool{}
					for _, p := range pixels {
						unique[p] = true
					}
					if len(unique) < 30 {
						t.Fatal("moving pixel fixture did not animate")
					}
					if reference == nil {
						reference = pixels
					} else {
						for tick, want := range reference {
							if got, ok := pixels[tick]; !ok || got != want {
								t.Fatalf("Mode 0/4 pixels differ at update %d", tick)
							}
						}
					}
					t.Logf("599 two-refresh intervals; nonzero audio samples %d/%d; PSG=%t", nonzero, pcmSamples, psg)
				})
			}
		})
	}
}

// A masked native caller cannot advance the LCD event counter through IRQ
// handlers. It needs the polling fallback, then normal event pacing on resume.
func TestFramePacingPreservesMaskedIRQs(t *testing.T) {
	if os.Getenv("MTILE_FRAME_PACING_INTEGRATION") != "1" {
		t.Skip("set MTILE_FRAME_PACING_INTEGRATION=1 for native IRQ-state checks")
	}
	helper, err := emulator.ResolveHelper()
	if err != nil {
		t.Fatal(err)
	}
	s, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	root, err := exec.Command(s.TinyGoBin, "env", "TINYGOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := build.GenerateTarget(strings.TrimSpace(string(root)), s.RuntimeDir, target); err != nil {
		t.Fatal(err)
	}
	module := fmt.Sprintf("module pacing-diagnostic\n\ngo 1.27.1\nrequire github.com/drpaneas/mtile/runtime v0.0.0\nreplace github.com/drpaneas/mtile/runtime => %q\n", filepath.ToSlash(s.RuntimeDir))
	source := `package main
import (
 rt "github.com/drpaneas/mtile/runtime"
 "github.com/drpaneas/mtile/runtime/hw"
 "runtime/interrupt"
)
func main() {
 hw.BeginFramePace()
 state:=interrupt.Disable()
 hw.WaitFramePace()
 hw.WaitVDraw()
 hw.WaitForPresentWindow()
 if hw.GetReg16(hw.INTERRUPT_IME)!=0 { rt.DebugLog("masked IRQ state changed");for{} }
 interrupt.Restore(state)
 for i:=0;i<8;i++ {
  hw.BeginFramePace();hw.WaitFramePace();hw.WaitForPresentWindow();hw.WaitFrameStart()
 }
 if hw.GetReg16(hw.INTERRUPT_IME)!=1 { rt.DebugLog("enabled IRQ state changed");for{} }
 rt.DebugLog("pacing IRQ restore OK")
 for { hw.WaitVDraw();hw.WaitForPresentWindow() }
}`
	for name, contents := range map[string]string{"go.mod": module, "main.go": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	rom := filepath.Join(dir, "pacing.gba")
	cmd := exec.Command(s.TinyGoBin, "build", "-gc=none", "-opt=2", "-tags=mtile_preview_log,mtile_heapless", "-target="+target, "-o", rom, ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("masked IRQ ROM: %v\n%s", err, output)
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
	var messages []string
	for i := 0; i < 64; i++ {
		if _, err := session.Step(0, false); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, session.DrainLogs()...)
		if strings.Contains(strings.Join(messages, "\n"), "pacing IRQ restore OK") {
			break
		}
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	logs := strings.Join(append(messages, session.DrainLogs()...), "\n")
	if !strings.Contains(logs, "pacing IRQ restore OK") || strings.Contains(logs, "IRQ state changed") {
		t.Fatalf("IRQ-state regression: %q", logs)
	}
}
