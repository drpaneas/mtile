//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This opt-in capacity experiment deliberately postpones a private runtime's
// commit AFTER admission. It measures how late each workload can safely start;
// it does not weaken the production guard or claim that samples prove a bound.
func TestMode0PresentWindowNativeDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_MODE0_PRESENT_NATIVE") != "1" {
		t.Skip("set MTILE_MODE0_PRESENT_NATIVE=1 for native late-VBlank characterization")
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("MTILE_MODE0_PRESENT_OUT")
	if out == "" {
		out, err = os.MkdirTemp("", "mtile-mode0-present-")
	} else {
		err = os.MkdirAll(out, 0755)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Log("evidence:", out)
	helper := filepath.Join(repo, "build/player/mtile-player")
	runtimeSnapshot := filepath.Join(out, "runtime-snapshot")
	runtimeSource := os.Getenv("MTILE_MODE0_PRESENT_RUNTIME")
	if runtimeSource == "" {
		runtimeSource = filepath.Join(repo, "runtime")
	}
	if err := tileCanvasCopyTree(runtimeSource, runtimeSnapshot, false); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"clean", "clean-1", "clean-32", "clean-64", "clean-96", "clean-127", "dirty-maps", "palettes", "mutable16", "packed", "all-mutable", "all-packed", "audio", "audio-dense", "audio-dense-library", "all-mutable-audio-dense", "all-packed-audio-dense"} {
		if filter := os.Getenv("MTILE_MODE0_PRESENT_CASES"); filter != "" && !strings.Contains(","+filter+",", ","+kind+",") {
			continue
		}
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(out, kind)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			project := filepath.Join(dir, "project")
			if err := tileCanvasCopyTree(filepath.Join(repo, "examples/background-layers"), project, true); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(kind, "audio") {
				mode0PresentAudioAssets(t, repo, project, kind)
			}
			source := mode0PresentFixtureSource(kind)
			if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
			runtimeDir := filepath.Join(dir, "runtime")
			if err := tileCanvasCopyTree(runtimeSnapshot, runtimeDir, false); err != nil {
				t.Fatal(err)
			}
			mode0File := filepath.Join(runtimeDir, "mode0.go")
			data, err := os.ReadFile(mode0File)
			if err != nil {
				t.Fatal(err)
			}
			before := "\tc.lastBlitStart = hw.GetReg16(hw.VCOUNT)"
			if strings.Count(string(data), before) != 1 {
				t.Fatal("missing unique private admission hook")
			}
			hook := os.Getenv("MTILE_MODE0_PRESENT_HOOK")
			if hook == "" {
				hook = "after"
			}
			patched := string(data)
			switch hook {
			case "after":
				patched = strings.Replace(patched, before, "\tdiagnosticLateCommit()\n"+before, 1)
			case "before":
				guard := "\tif !mode0PresentWindowFits("
				if strings.Count(patched, guard) != 1 {
					t.Fatal("missing unique admission guard")
				}
				patched = strings.Replace(patched, guard, "\tdiagnosticLateCommit()\n"+guard, 1)
			case "none":
			default:
				t.Fatal("unknown MTILE_MODE0_PRESENT_HOOK", hook)
			}
			delaySource := mode0PresentDelaySource
			if line := os.Getenv("MTILE_MODE0_PRESENT_FIXED_LINE"); line != "" {
				n, err := strconv.Atoi(line)
				if err != nil || n < 160 || n > 227 {
					t.Fatal("invalid fixed line", line)
				}
				delaySource = strings.Replace(delaySource, "line:=lines[(index/3)%uint32(len(lines))]", "line:=uint16("+line+");_ = lines", 1)
			}
			data = []byte(strings.Replace(patched, "c.lastBlitCycles = coldCycles + commitCycles", "c.lastBlitCycles = coldCycles + commitCycles\n DiagnosticCommitCycles = commitCycles", 1) + delaySource)
			mode0PresentWriteJSON(t, filepath.Join(dir, "configuration.json"), map[string]string{"hook": hook, "runtimeSource": runtimeSource, "case": kind})
			if err := os.WriteFile(mode0File, data, 0644); err != nil {
				t.Fatal(err)
			}
			service, err := build.NewService("", runtimeDir)
			if err != nil {
				t.Fatal(err)
			}
			service.Debug = true
			service.KeepWork = true
			var oracle []presentWindowRow
			for _, renderer := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0} {
				svc := service
				svc.Renderer = renderer
				svc.TinyGoBin = mode0PresentTinyGoWrapper(t, dir, service.TinyGoBin, renderer)
				result, err := svc.Build(project, filepath.Join(dir, renderer.String()+".gba"))
				if err != nil {
					t.Fatal(err)
				}
				mode0PresentWriteJSON(t, filepath.Join(dir, renderer.String()+"-build.json"), result)
				count := 59
				if value := os.Getenv("MTILE_MODE0_PRESENT_COUNT"); value != "" {
					count, _ = strconv.Atoi(value)
					if count < 3 || count > 1000 {
						t.Fatal("invalid count", value)
					}
				}
				rows, err := capturePresentWindow(helper, result.ROMPath, count)
				if err != nil {
					t.Fatal(err)
				}
				if renderer == transpiler.RendererMode4 {
					oracle = rows
				} else {
					for i := range rows {
						if rows[i].Tick != oracle[i].Tick {
							t.Fatal("different logical ticks")
						}
						rows[i].FirstDifferentPixels = countDifferingPixels(rows[i].firstRGBA, oracle[i].lastRGBA)
						rows[i].LastDifferentPixels = countDifferingPixels(rows[i].lastRGBA, oracle[i].lastRGBA)
						for j := range rows[i].Scans {
							rows[i].Scans[j].DifferentPixels = countDifferingPixels(rows[i].Scans[j].rgba, oracle[i].lastRGBA)
						}
						if rows[i].FirstDifferentPixels != 0 {
							if err := writePNG(filepath.Join(dir, fmt.Sprintf("first-difference-tick%d.png", rows[i].Tick)), rows[i].firstRGBA); err != nil {
								t.Fatal(err)
							}
						}
						if (hook != "after" && (rows[i].Start < 160 || rows[i].End < rows[i].Start || rows[i].End >= 228 || rows[i].FirstDifferentPixels != 0)) || (rows[i].Tick > 2 && hook == "after" && rows[i].Start == 160 && rows[i].End >= 160 && rows[i].FirstDifferentPixels != 0) {
							t.Errorf("unsafe admitted presentation at tick%d: %d first-scan pixels", rows[i].Tick, rows[i].FirstDifferentPixels)
						}
					}
					mode0PresentWriteJSON(t, filepath.Join(dir, "comparison.json"), rows)
					var maximum uint32
					safe, unsafe := 0, 0
					for _, r := range rows[2:] {
						if r.CommitCycles > maximum {
							maximum = r.CommitCycles
						}
						if r.FirstDifferentPixels == 0 && r.End >= r.Start && r.End < 228 {
							safe++
						} else {
							unsafe++
						}
					}
					t.Logf("%s: warm maximum commit=%d cycles; safe/unsafe tested starts=%d/%d; OAM proof=%d; exact per-scan comparisons in %s", kind, maximum, safe, unsafe, result.RendererOAMEntries, dir)
				}
				if strings.Contains(kind, "audio") {
					audible := false
					var peak uint32
					for _, r := range rows {
						audible = audible || r.Audible
						if r.AudioActive > peak {
							peak = r.AudioActive
						}
						if r.AudioErrors != 0 {
							t.Errorf("native audio errors%d", r.AudioErrors)
						}
					}
					if !audible || peak != 4 {
						t.Errorf("audio coverage audible=%v peak=%d", audible, peak)
					}
				}
				mode0PresentWriteJSON(t, filepath.Join(dir, renderer.String()+"-traces.json"), rows)
			}
		})
	}
}

func mode0PresentFixtureSource(kind string) string {
	nativeAudio := strings.Contains(kind, "audio")
	if strings.HasPrefix(kind, "all-") {
		kind = strings.TrimSuffix(kind, "-audio-dense")
	}
	var b strings.Builder
	initial := ""
	if nativeAudio {
		initial = "music(0)"
	}
	b.WriteString("frame=0\nfunction _init() " + initial + " end\nfunction _update()\n frame=frame+1\n camera(frame%8,0)\n")
	if kind == "dirty-maps" || strings.HasPrefix(kind, "all-") {
		for layer := 0; layer < 4; layer++ {
			fmt.Fprintf(&b, " mset(%d,4,4,1+frame%%2)\n", layer)
		}
	}
	if kind == "palettes" || strings.HasPrefix(kind, "all-") {
		b.WriteString(" pal(5,5+frame%2,0)\n pal(6,8+frame%2,1)\n")
	}
	if kind == "mutable16" || kind == "all-mutable" {
		for id := 1; id <= 16; id++ {
			fmt.Fprintf(&b, " sset(%d,0,5+frame%%2)\n", id*8)
		}
	}
	b.WriteString("end\nfunction _draw()\n cls(1)\n")
	for layer := 0; layer < 4; layer++ {
		fmt.Fprintf(&b, " map(%d,0,0,0,0,60,20,0)\n", layer)
	}
	objects := 128
	if strings.HasPrefix(kind, "clean-") {
		objects, _ = strconv.Atoi(strings.TrimPrefix(kind, "clean-"))
	}
	for i := 0; i < objects; i++ {
		fmt.Fprintf(&b, " spr(%d,%d,%d,1,1,frame%%2==1,false)\n", 1+i%10, 8+(i%16)*14, 4+(i/16)*19)
	}
	if kind == "packed" || kind == "all-packed" {
		// Two cached 32x32 images fill the 32-cell packed cache. Keep the total
		// public object count at128 by replacing the final two one-cell draws.
		s := b.String()
		pos := strings.LastIndex(s, " spr(")
		s = s[:pos]
		pos = strings.LastIndex(s, " spr(")
		s = s[:pos]
		b.Reset()
		b.WriteString(s)
		b.WriteString(" spr(1,8,120,4,4,frame%2==1,false)\n spr(5,48,120,4,4,false,frame%2==1)\n")
	}
	b.WriteString("end\n")
	return b.String()
}

const mode0PresentDelaySource = `
var diagnosticCommitIndex uint32
var DiagnosticPresentRequest uint32
var DiagnosticCommitCycles uint32
func diagnosticLateCommit() {
 diagnosticCommitIndex++
 if diagnosticCommitIndex<=2{return}
 lines:=[...]uint16{160,176,184,186,187,188,192,208,212,214,216,218,220,221,222,223,224,226,227}
 index:=diagnosticCommitIndex-3
 line:=lines[(index/3)%uint32(len(lines))]
 phase:=uint32(index%3)*575
 DiagnosticPresentRequest=uint32(line)<<16|phase
 for hw.GetReg16(hw.VCOUNT)!=line-1{}
 for hw.GetReg16(hw.VCOUNT)==line-1{}
 start:=hw.ReadFrameCycleCounter()
 for hw.ReadFrameCycleCounter()-start<phase{}
}
`

func mode0PresentTinyGoWrapper(t *testing.T, dir, real string, renderer transpiler.Renderer) string {
	t.Helper()
	path := filepath.Join(dir, "tinygo-"+renderer.String())
	additions := `debugMailbox.Add("window",presentWindowValue())
debugMailbox.Add("maps",rt.NumVal(rt.Number(ctx.RendererUploadStats().LastMapBytes)))
debugMailbox.Add("tiles",rt.NumVal(rt.Number(ctx.RendererUploadStats().LastTileBytes)))
debugMailbox.Add("objtiles",rt.NumVal(rt.Number(ctx.RendererUploadStats().LastOBJTileBytes)))
debugMailbox.Add("oam",rt.NumVal(rt.Number(ctx.RendererUploadStats().LastOAMBytes)))`
	if strings.Contains(filepath.Base(dir), "audio") {
		additions += "\nna := ctx.AudioStats().Native\nactive:=uint32(0);for _,ch:=range na.Channels{if ch.Active{active++}}\ndebugMailbox.Add(\"audactive\",rt.NumVal(rt.Number(active)))\ndebugMailbox.Add(\"audmax\",rt.NumVal(rt.Number(na.MaxServiceCycles)))\ndebugMailbox.Add(\"audcalls\",rt.NumVal(rt.Number(na.ServiceCalls)))\ndebugMailbox.Add(\"audevents\",rt.NumVal(rt.Number(na.EventsApplied)))\ndebugMailbox.Add(\"auderrors\",rt.NumVal(rt.Number(na.QueueOverflows+na.CommandsRejected+na.InvalidAssets)))"
	}
	if renderer == transpiler.RendererMode0 {
		additions += "\ndebugMailbox.Add(\"request\",rt.NumVal(rt.Number(rt.DiagnosticPresentRequest)))\ndebugMailbox.Add(\"rawcommit\",rt.NumVal(rt.Number(rt.DiagnosticCommitCycles)))"
	}
	frozenMain := ""
	if baseline := os.Getenv("MTILE_MODE0_PRESENT_GENERATED_BASELINE"); baseline != "" {
		data, err := os.ReadFile(filepath.Join(baseline, filepath.Base(dir), renderer.String()+"-build.json"))
		if err != nil {
			t.Fatal(err)
		}
		var prior build.Result
		if err := json.Unmarshal(data, &prior); err != nil {
			t.Fatal(err)
		}
		frozenMain = filepath.Join(prior.WorkDir, "main.go")
	}
	script := "#!/usr/bin/env python3\nimport os,sys,re\nfrom pathlib import Path\nfrozen=" + strconv.Quote(frozenMain) + "\nreal=" + strconv.Quote(real) + "\nargs=sys.argv[1:]\nif args and args[0]=='build':\n p=Path('main.go')\n text=Path(frozen).read_text() if frozen else p.read_text()\n text=re.sub(r'(?m)^\\s*(?:na := ctx.AudioStats\\(\\).Native|active:=uint32\\(0\\);[^\\n]*)\\n','',text)\n text=re.sub(r'(?m)^\\s*debugMailbox.Add\\([^\\n]*\\n','',text)\n text=text.replace('debugMailbox.End()'," + strconv.Quote(additions+"\ndebugMailbox.End()") + ")\n if 'func presentWindowValue()' not in text: text+='\\nfunc presentWindowValue() rt.Value {start,end:=ctx.LastBlitWindow();return rt.NumVal(rt.Number(uint32(start)<<16|uint32(end)))}\\n'\n p.write_text(text)\nos.execv(real,[real]+args)\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

type presentWindowRow struct {
	Tick                                                                     uint32
	LCDSamples                                                               int
	RequestedLine, RequestedPhase, Start, End                                uint16
	CommitCycles, MapBytes, TileBytes, OBJTileBytes, OAMBytes                uint32
	AudioActive, AudioMaxServiceCycles, AudioCalls, AudioEvents, AudioErrors uint32
	Audible                                                                  bool
	Scans                                                                    []presentWindowScan
	FirstSHA, LastSHA                                                        string
	FirstDifferentPixels, LastDifferentPixels                                int
	firstRGBA, lastRGBA                                                      []byte
}

type presentWindowScan struct {
	LCDFrame        uint32
	SHA             string
	DifferentPixels int
	rgba            []byte
}

func capturePresentWindow(helper, rom string, count int) ([]presentWindowRow, error) {
	session, err := emulator.Start(helper, rom)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	var rows []presentWindowRow
	for lcd := 0; lcd < 3000; lcd++ {
		f, err := session.Step(0, false)
		if err != nil {
			return rows, err
		}
		if f.Overflow {
			return rows, fmt.Errorf("audio overflow")
		}
		if !f.Telemetry.DebugValid {
			continue
		}
		s := f.Telemetry.Stats
		if s.HeapError != emulator.HeapOK {
			return rows, fmt.Errorf("heap error: %+v", s)
		}
		if s.Frames == 0 {
			continue
		}
		if len(rows) == 0 || rows[len(rows)-1].Tick != s.Frames {
			if len(rows) == count {
				return rows, nil
			}
			if s.Frames != uint32(len(rows)+1) {
				return rows, fmt.Errorf("skipped tick %d", s.Frames)
			}
			r := presentWindowRow{Tick: s.Frames, CommitCycles: s.LastBlit}
			for _, w := range f.Telemetry.Watches {
				v := uint32(w.Raw)
				switch w.Name {
				case "audactive":
					r.AudioActive = v
				case "audmax":
					r.AudioMaxServiceCycles = v
				case "audcalls":
					r.AudioCalls = v
				case "audevents":
					r.AudioEvents = v
				case "auderrors":
					r.AudioErrors = v
				case "rawcommit":
					r.CommitCycles = v
				case "window":
					r.Start, r.End = uint16(v>>16), uint16(v)
				case "request":
					r.RequestedLine, r.RequestedPhase = uint16(v>>16), uint16(v)
				case "maps":
					r.MapBytes = v
				case "tiles":
					r.TileBytes = v
				case "objtiles":
					r.OBJTileBytes = v
				case "oam":
					r.OAMBytes = v
				}
			}
			rows = append(rows, r)
		}
		r := &rows[len(rows)-1]
		for _, sample := range f.PCM {
			if sample != 0 {
				r.Audible = true
				break
			}
		}
		r.LCDSamples++
		r.Scans = append(r.Scans, presentWindowScan{LCDFrame: f.Number, SHA: fmt.Sprintf("%x", sha256.Sum256(f.RGBA)), rgba: append([]byte(nil), f.RGBA...)})
		r.lastRGBA = append(r.lastRGBA[:0], f.RGBA...)
		r.LastSHA = fmt.Sprintf("%x", sha256.Sum256(f.RGBA))
		if r.LCDSamples == 1 {
			r.firstRGBA = append([]byte(nil), f.RGBA...)
			r.FirstSHA = r.LastSHA
		}
	}
	return rows, fmt.Errorf("only captured %d/%d ticks", len(rows), count)
}
func mode0PresentWriteJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

// Audio cases use the ordinary public music API. Dense fixtures exercise all
// four PSG channels at the maximum accepted tempo and minimum note duration.
// The library variant puts the used sounds at the end of a full256-entry
// library, so its linear sound lookup cost is represented explicitly.
func mode0PresentAudioAssets(t *testing.T, repo, project, kind string) {
	t.Helper()
	src := filepath.Join(repo, "examples/audio-gb-song/assets/audio")
	dst := filepath.Join(project, "assets/audio")
	if err := tileCanvasCopyTree(src, dst, false); err != nil {
		t.Fatal(err)
	}
	if kind == "audio" {
		return
	}
	data, err := os.ReadFile(filepath.Join(dst, "sounds.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sounds map[string]any
	if err = json.Unmarshal(data, &sounds); err != nil {
		t.Fatal(err)
	}
	templates := sounds["sounds"].([]any)[:4]
	var output []any
	count := 4
	if kind == "audio-dense-library" {
		count = 256
	}
	for i := 0; i < count; i++ {
		bytes, err := json.Marshal(templates[i%4])
		if err != nil {
			t.Fatal(err)
		}
		var sound map[string]any
		if err = json.Unmarshal(bytes, &sound); err != nil {
			t.Fatal(err)
		}
		sound["id"] = i
		sound["name"] = fmt.Sprintf("PSG fixture %d", i)
		output = append(output, sound)
	}
	sounds["sounds"] = output
	mode0PresentWriteJSON(t, filepath.Join(dst, "sounds.json"), sounds)
	var clips []any
	lanes := map[string]int{}
	for ch := 0; ch < 4; ch++ {
		var events []any
		for tick := 0; tick < 96; tick++ {
			events = append(events, map[string]any{"tick": tick, "durationTicks": 1, "note": 60 + tick%12, "soundId": count - 4 + ch, "priority": 128})
		}
		name := fmt.Sprintf("ch%d", ch+1)
		lanes[name] = ch
		clips = append(clips, map[string]any{"id": ch, "name": name, "kind": name, "lengthTicks": 96, "events": events})
	}
	music := map[string]any{"version": 2, "ppqn": 96, "clips": clips, "parts": []any{map[string]any{"id": 0, "name": "Dense four-channel IRQ fixture", "lengthTicks": 96, "lanes": lanes}}, "songs": []any{map[string]any{"id": 0, "name": "Maximum tempo", "tempo": 400, "sequence": []int{0}, "loopStart": 0}}}
	mode0PresentWriteJSON(t, filepath.Join(dst, "music.json"), music)
}
