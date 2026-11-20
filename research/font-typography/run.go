package main

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

type probeFont struct {
	Name       string                `json:"name"`
	Width      int                   `json:"width"`
	Height     int                   `json:"height"`
	CellWidth  int                   `json:"cellWidth"`
	CellHeight int                   `json:"cellHeight"`
	Glyphs     []assetsync.FontGlyph `json:"glyphs"`
}
type measurement struct {
	Variant       string `json:"variant"`
	Workload      string `json:"workload"`
	Renderer      string `json:"renderer"`
	ROMBytes      int    `json:"romBytes"`
	FontROMBytes  uint64 `json:"fontROMBytes"`
	EWRAMUsed     uint64 `json:"ewramUsed"`
	IWRAMUsed     uint64 `json:"iwramUsed"`
	MedianFrame   uint32 `json:"medianFrameCycles"`
	P95Frame      uint32 `json:"p95FrameCycles"`
	MedianDraw    uint32 `json:"medianDrawCycles"`
	MedianCommit  uint32 `json:"medianCommitCycles"`
	MissedFrames  uint32 `json:"missedFrames"`
	HeapAllocated uint32 `json:"heapAllocated"`
	Samples       int    `json:"samples"`
	ParityDiff    int    `json:"mode0ParityDifferingBytes,omitempty"`
	ROMSHA256     string `json:"romSha256"`
}
type capture struct {
	Frames               []uint32
	Pixels               [][]byte
	Cycles, Draw, Commit []uint32
	Missed, Heap         uint32
}

func runExperiment(root, out, selected string) error {
	out, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	cases := []struct{ name, font string }{{"before", filepath.Join(out, "baseline.json")}, {"same-grid", filepath.Join(root, "research/font-typography/5x7.json")}, {"larger-grid", filepath.Join(root, "research/font-typography/6x10.json")}}
	cases = append(cases, struct{ name, font string }{"same-grid-pointer", filepath.Join(out, "5x7-pointer.json")}, struct{ name, font string }{"larger-grid-pointer", filepath.Join(out, "6x10-pointer.json")})
	// Pointer cases isolate the lookup optimization from the bitmap-size change.
	for _, pair := range [][2]string{{filepath.Join(out, "baseline.json"), "baseline-pointer.json"}, {filepath.Join(root, "research/font-typography/5x7.json"), "5x7-pointer.json"}, {filepath.Join(root, "research/font-typography/6x10.json"), "6x10-pointer.json"}} {
		raw, err := os.ReadFile(pair[0])
		if err != nil {
			return err
		}
		var value map[string]any
		if err = json.Unmarshal(raw, &value); err != nil {
			return err
		}
		value["probePointer"] = true
		raw, err = json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(out, pair[1]), raw, 0644); err != nil {
			return err
		}
	}
	cases = append(cases, struct{ name, font string }{"before-pointer", filepath.Join(out, "baseline-pointer.json")})
	compactRaw, err := os.ReadFile(filepath.Join(out, "6x10-pointer.json"))
	if err != nil {
		return err
	}
	var compact map[string]any
	if err = json.Unmarshal(compactRaw, &compact); err != nil {
		return err
	}
	compact["cellWidth"] = 6
	compact["cellHeight"] = 10
	compactRaw, err = json.MarshalIndent(compact, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(out, "6x10-compact.json"), compactRaw, 0644); err != nil {
		return err
	}
	cases = append(cases, struct{ name, font string }{"compact-grid-pointer", filepath.Join(out, "6x10-compact.json")})

	var results []measurement
	if previous, err := os.ReadFile(filepath.Join(out, "measurements.json")); err == nil {
		if err := json.Unmarshal(previous, &results); err != nil {
			return fmt.Errorf("read prior measurements: %w", err)
		}
	}
	for _, variant := range cases {
		if selected != "" && !slices.Contains(strings.Split(selected, ","), variant.name) {
			continue
		}
		results = slices.DeleteFunc(results, func(m measurement) bool { return m.Variant == variant.name })
		raw, err := os.ReadFile(variant.font)
		if err != nil {
			return err
		}
		var f probeFont
		if err = json.Unmarshal(raw, &f); err != nil {
			return err
		}
		dir := filepath.Join(out, variant.name)
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		runtimeDir := filepath.Join(dir, "runtime")
		if err = runCommand(root, "python3", filepath.Join(root, "research/font-typography/probe.py"), "prepare", root, dir, variant.font); err != nil {
			return err
		}
		if err = hostProbe(runtimeDir, dir); err != nil {
			return err
		}
		project := filepath.Join(dir, "project")
		if _, err = os.Stat(project); os.IsNotExist(err) {
			if err = build.CreateProject(project, build.NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
				return err
			}
		}
		p, err := assetsync.LoadProject(project)
		if err != nil {
			return err
		}
		p.Font = "international"
		if err = assetsync.SaveProject(project, p); err != nil {
			return err
		}
		service, err := build.NewService(filepath.Join(dir, "tinygo-probe"), runtimeDir)
		if err != nil {
			return err
		}
		service.Debug = true
		service.KeepWork = true
		works := []string{"empty", "hud", "unicode", "dense", "edges"}
		for _, work := range works {
			source := workload(work, f.CellWidth, f.CellHeight)
			if err = os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0644); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, work+".lua"), []byte(source), 0644); err != nil {
				return err
			}
			var previous *capture
			modes := []transpiler.Renderer{transpiler.RendererMode4}
			if f.Height <= 8 && (work == "empty" || work == "hud" || work == "edges") {
				modes = append(modes, transpiler.RendererMode0)
			}
			for _, mode := range modes {
				name := work + "-" + mode.String()
				service.Renderer = mode
				fmt.Printf("%s %s: build and capture\n", variant.name, name)
				result, err := service.Build(project, filepath.Join(dir, name+".gba"))
				if err != nil {
					return err
				}
				fontBytes, err := fontROMSize(filepath.Join(result.WorkDir, "game.elf"))
				if err != nil {
					return err
				}
				if err = copyFile(filepath.Join(result.WorkDir, "game.elf"), filepath.Join(dir, name+".elf")); err != nil {
					return err
				}
				if err = copyFile(filepath.Join(result.WorkDir, "main.go"), filepath.Join(dir, name+".go.txt")); err != nil {
					return err
				}
				if err = copyFile(filepath.Join(result.WorkDir, "game.map"), filepath.Join(dir, name+".map")); err != nil {
					return err
				}
				if err := os.RemoveAll(result.WorkDir); err != nil {
					return err
				}
				var targets []uint32
				if mode == transpiler.RendererMode0 {
					targets = previous.Frames
				}
				captured, err := captureROM(filepath.Join(root, "build/player/mtile-player"), result.ROMPath, targets)
				if err != nil {
					return err
				}
				if err = writePNG(filepath.Join(dir, name+".png"), captured.Pixels[len(captured.Pixels)-1]); err != nil {
					return err
				}
				record := measurement{Variant: variant.name, Workload: work, Renderer: mode.String(), ROMBytes: result.ROMSize, FontROMBytes: fontBytes, EWRAMUsed: result.EWRAMUsed, IWRAMUsed: result.IWRAMUsed, MedianFrame: percentile(captured.Cycles, 50), P95Frame: percentile(captured.Cycles, 95), MedianDraw: percentile(captured.Draw, 50), MedianCommit: percentile(captured.Commit, 50), MissedFrames: captured.Missed, HeapAllocated: captured.Heap, Samples: len(captured.Frames)}
				rom, err := os.ReadFile(result.ROMPath)
				if err != nil {
					return err
				}
				record.ROMSHA256 = fmt.Sprintf("%x", sha256.Sum256(rom))
				if mode == transpiler.RendererMode0 {
					for i, pixels := range captured.Pixels {
						for n, value := range pixels {
							if value != previous.Pixels[i][n] {
								record.ParityDiff++
							}
						}
					}
					if record.ParityDiff != 0 {
						return fmt.Errorf("%s parity differs: %d bytes", name, record.ParityDiff)
					}
				} else {
					previous = &captured
				}
				fmt.Printf("  frame %d, draw %d, font %d bytes, EWRAM %d, parity %d\n", record.MedianFrame, record.MedianDraw, fontBytes, record.EWRAMUsed, record.ParityDiff)
				results = append(results, record)
				raw, _ := json.MarshalIndent(results, "", "  ")
				if err = os.WriteFile(filepath.Join(out, "measurements.json"), raw, 0644); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func workload(name string, cw, ch int) string {
	switch name {
	case "empty":
		return "function _draw() cls(24) end\n"
	case "hud":
		return `score=0
function _update() score=(score+1)%1000 end
function _draw()
 cls(24)
 print("SCORE",0,0,15)
 print(score,36,0,15)
 print("ABCDABCDABCDABCDABCDABCDABCDABCDABCDABCD",0,16,15)
end
`
	case "unicode":
		lines := []string{"Hello GBA! 0123456789", "Καλημέρα κόσμε!", "Ελλάδα: άέήίόύώ", "Grüße aus Deutschland!", "ÄÖÜ äöü ß ẞ", "Μνήμη 123 / Größe 456", "gjpqy ϊϋ ΐΰ", "HP 100 / SCORE 12345"}
		var b strings.Builder
		b.WriteString("function _draw()\n cls(24)\n")
		for i, line := range lines {
			fmt.Fprintf(&b, " print(%q,8,%d,15)\n", line, 8+i*18)
		}
		b.WriteString("end\n")
		return b.String()
	case "dense":
		var b strings.Builder
		b.WriteString("function _draw()\n cls(24)\n")
		for y := 0; y+ch <= 160; y += ch {
			fmt.Fprintf(&b, " print(%q,0,%d,15)\n", "Καλημέρα! Grüße gjpqy άήΐΰ", y)
		}
		b.WriteString("end\n")
		return b.String()
	case "edges":
		// Identical world coordinates in every candidate. Last-cell ink, clipped
		// partial glyphs, right-edge wrap and negative positions remain bounded.
		return `function _draw()
 cls(24)
 print("Agj",234,0,15)
 print("A",239,159,15)
 print("B",-2,-2,15)
 print("g",0,152,15)
 print("pq",232,151,15)
end
`
	}
	panic(name)
}
func hostProbe(runtimeDir, out string) error {
	cmd := exec.Command("go", "test", "-run", "TestTypography", "-count=1", "-v", ".")
	cmd.Dir = runtimeDir
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/mtile-font-go-cache", "TYPOGRAPHY_OUT="+out)
	raw, err := cmd.CombinedOutput()
	writeErr := os.WriteFile(filepath.Join(out, "host-tests.txt"), raw, 0644)
	if err != nil {
		return fmt.Errorf("host API/edge probe: %w\n%s", err, raw)
	}
	fmt.Print(string(raw))
	return writeErr
}
func fontROMSize(path string) (uint64, error) {
	file, err := elf.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	symbols, err := file.Symbols()
	if err != nil {
		return 0, err
	}
	var total uint64
	count := 0
	for _, symbol := range symbols {
		if strings.Contains(symbol.Name, "builtinFont") {
			return 0, fmt.Errorf("unselected fallback font linked")
		}
		if symbol.Name == "main.projectFontROM" || symbol.Name == "main.projectFontExtraROM" {
			if symbol.Value < 0x08000000 || symbol.Value >= 0x0a000000 {
				return 0, fmt.Errorf("font escaped ROM")
			}
			total += symbol.Size
			if symbol.Name == "main.projectFontROM" {
				count++
			}
		}
	}
	if count != 1 {
		return 0, fmt.Errorf("font bank count %d", count)
	}
	return total, nil
}
func captureROM(helper, rom string, targets []uint32) (capture, error) {
	const warmup = 20
	const samples = 120
	session, err := emulator.Start(helper, rom)
	if err != nil {
		return capture{}, err
	}
	defer func() { _ = session.Close() }()
	var c capture
	var sequence, last uint32
	var misses uint32
	for step := 0; step < 3000 && len(c.Frames) < samples; step++ {
		frame, err := session.Step(0, false)
		if err != nil {
			return c, err
		}
		if !frame.Telemetry.DebugValid || frame.Telemetry.Sequence == sequence {
			continue
		}
		sequence = frame.Telemetry.Sequence
		stats := frame.Telemetry.Stats
		if stats.Frames <= warmup {
			misses = stats.MissedFrames
			continue
		}
		if stats.Frames == last {
			continue
		}
		last = stats.Frames
		if len(targets) > 0 {
			want := targets[len(c.Frames)]
			if last < want {
				continue
			}
			if last > want {
				return c, fmt.Errorf("missed paired frame %d (next %d)", want, last)
			}
		}
		c.Frames = append(c.Frames, last)
		c.Pixels = append(c.Pixels, bytes.Clone(frame.RGBA))
		c.Cycles = append(c.Cycles, stats.LastFrame)
		c.Draw = append(c.Draw, stats.LastDraw)
		c.Commit = append(c.Commit, stats.LastBlit)
		c.Missed = stats.MissedFrames - misses
		c.Heap = stats.HeapAllocated
	}
	if len(c.Frames) != samples {
		return c, fmt.Errorf("only %d profile samples", len(c.Frames))
	}
	return c, nil
}
func percentile(values []uint32, p int) uint32 {
	v := slices.Clone(values)
	slices.Sort(v)
	return v[(len(v)-1)*p/100]
}
func runCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", name, err, raw)
	}
	fmt.Print(string(raw))
	return nil
}
func copyFile(from, to string) error {
	raw, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, raw, 0644)
}
func writePNG(path string, pixels []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(file, &image.RGBA{Pix: pixels, Stride: 240 * 4, Rect: image.Rect(0, 0, 240, 160)})
	return errors.Join(err, file.Close())
}
