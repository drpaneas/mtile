// Command verify_learning_games builds and plays every complete graphics lesson.
// It compares the real Mode 0 and Mode 4 LCD pixels at matching game ticks,
// reads debug watches without changing cartridge memory, and can publish native
// previews only after the controls and teaching checkpoints have passed.
//
// Run from the repository root:
//
//	go run ./scripts/docs/verify_learning_games.go -player build/player/mtile-player -out /tmp/mtile-learning-proof -publish
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

type lesson struct {
	project, name string
	shipped       bool
}

var lessons = []lesson{
	{"tidewalk", "01-map", false}, {"tidewalk", "02-camera", false}, {"tidewalk", "03-parallax", true},
	{"moth-study", "01-pose", false}, {"moth-study", "02-loop", false}, {"moth-study", "03-facing", true},
	{"moon-door", "01-open", true}, {"moon-door", "02-pause", false},
	{"lantern-rooms", "01-switch", false}, {"lantern-rooms", "02-crossing", true},
}

type evidence struct {
	Project          string            `json:"project"`
	Lesson           string            `json:"lesson"`
	ROMHashes        map[string]string `json:"rom_sha256"`
	MatchedFrames    int               `json:"matched_game_frames"`
	ComparedPixels   uint64            `json:"compared_pixels"`
	DifferingPixels  uint64            `json:"differing_pixels"`
	Mode0Missed      uint32            `json:"mode0_poststartup_missed_frames"`
	Checks           []string          `json:"checks"`
	Input            []uint16          `json:"input_per_game_tick"`
	PreviewGameFrame uint32            `json:"preview_game_frame"`
	DebugROM         bool              `json:"debug_rom"`
}
type player struct {
	session *emulator.Session
	frame   emulator.Frame
}
type pair struct {
	left, right player
	report      evidence
	baseline    uint32
	frames      []*image.Paletted
	palette     color.Palette
	colors      map[uint32]uint8
	preview     []byte
	out         string
}

func main() {
	helper := flag.String("player", "build/player/mtile-player", "headless mGBA helper")
	out := flag.String("out", "/tmp/mtile-learning-proof", "builds, captures, and evidence")
	project := flag.String("project", "", "optional single project name")
	name := flag.String("lesson", "", "optional single lesson name without .lua")
	publish := flag.Bool("publish", false, "copy shipped-game captures into the examples")
	gallery := flag.Bool("gallery", false, "assemble the four published native previews without rebuilding")
	flag.Parse()
	if *gallery {
		if err := saveGallery(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(*helper, *out, *project, *name, *publish); err != nil {
		fmt.Fprintln(os.Stderr, "learning games:", err)
		os.Exit(1)
	}
}

func run(helper, output, project, name string, publish bool) error {
	helper, _ = filepath.Abs(helper)
	output, _ = filepath.Abs(output)
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	service, err := build.NewService("", "")
	if err != nil {
		return err
	}
	service.Debug = true
	var reports []evidence
	for _, l := range lessons {
		if project != "" && l.project != project || name != "" && l.name != name {
			continue
		}
		r, err := verify(service, helper, output, l, publish)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", l.project, l.name, err)
		}
		reports = append(reports, r)
		fmt.Printf("PASS %s/%s: %d matching frames, zero pixel differences, %d checks\n", l.project, l.name, r.MatchedFrames, len(r.Checks))
	}
	if len(reports) == 0 {
		return fmt.Errorf("no matching lesson")
	}
	reportName := "report.json"
	if project != "" {
		reportName = "report-" + project + ".json"
	}
	return writeJSON(filepath.Join(output, reportName), reports)
}

func verify(service build.Service, helper, output string, l lesson, publish bool) (evidence, error) {
	p := pair{report: evidence{Project: l.project, Lesson: l.name, ROMHashes: map[string]string{}, DebugROM: true}}
	dir := filepath.Join(output, l.project, l.name)
	p.out = dir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return p.report, err
	}
	stage, err := os.MkdirTemp(output, "lesson-*")
	if err != nil {
		return p.report, err
	}
	defer os.RemoveAll(stage)
	source := filepath.Join("examples", l.project)
	if err := os.CopyFS(stage, os.DirFS(source)); err != nil {
		return p.report, err
	}
	code, err := os.ReadFile(filepath.Join(source, "lessons", l.name+".lua"))
	if err != nil {
		return p.report, err
	}
	if err := os.WriteFile(filepath.Join(stage, "game.lua"), code, 0644); err != nil {
		return p.report, err
	}
	if l.shipped {
		shipped, err := os.ReadFile(filepath.Join(source, "game.lua"))
		if err != nil {
			return p.report, err
		}
		if !bytes.Equal(code, shipped) {
			return p.report, fmt.Errorf("shipped game does not match its documented lesson")
		}
	}
	for i, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		service.Renderer = renderer
		rom := filepath.Join(dir, renderer.String()+".gba")
		result, err := service.Build(stage, rom)
		if err != nil {
			return p.report, err
		}
		if result.Renderer != renderer {
			return p.report, fmt.Errorf("renderer override was not honored")
		}
		data, err := os.ReadFile(rom)
		if err != nil {
			return p.report, err
		}
		p.report.ROMHashes[renderer.String()] = fmt.Sprintf("%x", sha256.Sum256(data))
		s, err := emulator.Start(helper, rom)
		if err != nil {
			return p.report, err
		}
		defer s.Close()
		if i == 0 {
			p.left.session = s
		} else {
			p.right.session = s
		}
	}
	for range 10 {
		if err := p.step(0); err != nil {
			return p.report, err
		}
	}
	p.baseline = p.left.frame.Telemetry.Stats.MissedFrames
	p.takePreview()
	check := func(ok bool, label string) error {
		if !ok {
			return fmt.Errorf("%s failed at frame %d; watches %+v", label, p.left.frame.Telemetry.Stats.Frames, p.left.frame.Telemetry.Watches)
		}
		p.report.Checks = append(p.report.Checks, label)
		return nil
	}
	switch l.project {
	case "moon-door":
		if err := p.tap(emulator.KeyA); err != nil {
			return p.report, err
		}
		if err := check(p.value("opening") != 0 && p.value("opening_ticks") < 4, "A starts at the first frame"); err != nil {
			return p.report, err
		}
		if err := p.hold(0, 6); err != nil {
			return p.report, err
		}
		p.takePreview()
		if l.name == "02-pause" {
			if err := p.tap(emulator.KeyStart); err != nil {
				return p.report, err
			}
			ticks := p.value("opening_ticks")
			pixels := bytes.Clone(p.left.frame.RGBA)
			if err := p.hold(0, 20); err != nil {
				return p.report, err
			}
			if err := check(p.value("paused") != 0 && p.value("opening_ticks") == ticks && bytes.Equal(pixels, p.left.frame.RGBA), "pause freezes both counter and pose"); err != nil {
				return p.report, err
			}
			if err := p.tap(emulator.KeyStart); err != nil {
				return p.report, err
			}
			if err := check(p.value("paused") == 0 && p.value("opening_ticks") > ticks, "resume continues the same action"); err != nil {
				return p.report, err
			}
		}
		if err := p.hold(0, 50); err != nil {
			return p.report, err
		}
		pixels := bytes.Clone(p.left.frame.RGBA)
		if err := p.hold(0, 15); err != nil {
			return p.report, err
		}
		if err := check(p.value("opening_ticks") == 30 && bytes.Equal(pixels, p.left.frame.RGBA), "nonlooping clip holds the final pose"); err != nil {
			return p.report, err
		}
		if err := p.tap(emulator.KeyA); err != nil {
			return p.report, err
		}
		if err := check(p.value("opening_ticks") < 4, "another tap restarts the action"); err != nil {
			return p.report, err
		}
		if err := p.hold(0, 40); err != nil {
			return p.report, err
		}
	case "moth-study":
		initial := bytes.Clone(p.left.frame.RGBA)
		changed := false
		for range 40 {
			if err := p.step(0); err != nil {
				return p.report, err
			}
			changed = changed || !bytes.Equal(initial, p.left.frame.RGBA)
		}
		if err := check(changed == (l.name != "01-pose"), "still pose stays still; saved loop changes pictures"); err != nil {
			return p.report, err
		}
		if l.name == "03-facing" {
			if err := p.hold(emulator.KeyLeft, 12); err != nil {
				return p.report, err
			}
			if err := check(p.value("fluttering") != 0 && p.value("facing_left") != 0, "left selects flutter and left facing"); err != nil {
				return p.report, err
			}
			p.takePreview()
			if err := p.hold(emulator.KeyRight, 12); err != nil {
				return p.report, err
			}
			if err := check(p.value("fluttering") != 0 && p.value("facing_left") == 0, "right changes facing"); err != nil {
				return p.report, err
			}
			if err := p.hold(0, 15); err != nil {
				return p.report, err
			}
			if err := check(p.value("fluttering") == 0, "release returns to idle"); err != nil {
				return p.report, err
			}
		}
	case "tidewalk":
		if err := p.hold(emulator.KeyRight, 200); err != nil {
			return p.report, err
		}
		p.takePreview()
		if err := p.hold(emulator.KeyRight, 300); err != nil {
			return p.report, err
		}
		maximum := 464.0
		if l.name == "01-map" {
			maximum = 224
		}
		if err := check(p.value("x") == maximum, "right edge stops the walk"); err != nil {
			return p.report, err
		}
		if err := p.hold(emulator.KeyLeft, 500); err != nil {
			return p.report, err
		}
		if err := check(p.value("x") == 0, "walk returns to the left edge"); err != nil {
			return p.report, err
		}
	case "lantern-rooms":
		if l.name == "01-switch" {
			if err := p.tap(emulator.KeyA); err != nil {
				return p.report, err
			}
			if err := check(p.value("room") == 1 && p.value("player_x") == 24, "A selects the second room and resets its spawn"); err != nil {
				return p.report, err
			}
			p.takePreview()
			if err := p.tap(emulator.KeyA); err != nil {
				return p.report, err
			}
			if err := check(p.value("room") == 0 && p.value("player_x") == 24, "A returns to the first room at the saved spawn"); err != nil {
				return p.report, err
			}
		} else {
			entered := false
			for range 160 {
				if err := p.step(emulator.KeyRight); err != nil {
					return p.report, err
				}
				if !entered && p.value("room") == 1 {
					if err := check(p.value("player_x") == 8, "right crossing chooses the left-edge spawn"); err != nil {
						return p.report, err
					}
					entered = true
				}
			}
			if err := check(p.value("room") == 1, "crossing the right doorway selects room two"); err != nil {
				return p.report, err
			}
			p.takePreview()
			returned := false
			for range 300 {
				if err := p.step(emulator.KeyLeft); err != nil {
					return p.report, err
				}
				if !returned && p.value("room") == 0 {
					if err := check(p.value("player_x") == 216, "left crossing chooses the right-edge spawn"); err != nil {
						return p.report, err
					}
					returned = true
				}
			}
			if err := check(p.value("room") == 0, "crossing left returns to room one"); err != nil {
				return p.report, err
			}
		}
	}
	p.report.Mode0Missed = p.left.frame.Telemetry.Stats.MissedFrames - p.baseline
	if err := check(p.report.Mode0Missed == 0, "Mode 0 meets every poststartup frame deadline"); err != nil {
		return p.report, err
	}
	if err := savePNG(filepath.Join(dir, "preview.png"), p.preview); err != nil {
		return p.report, err
	}
	if len(p.frames) != 0 {
		f, err := os.Create(filepath.Join(dir, "play.gif"))
		if err != nil {
			return p.report, err
		}
		delays := make([]int, len(p.frames))
		for i := range delays {
			delays[i] = (i+1)*20/3 - i*20/3
		}
		err = gif.EncodeAll(f, &gif.GIF{Image: p.frames, Delay: delays, LoopCount: 0})
		closeErr := f.Close()
		if err != nil {
			return p.report, err
		}
		if closeErr != nil {
			return p.report, closeErr
		}
	}
	if err := writeJSON(filepath.Join(dir, "capture.json"), p.report); err != nil {
		return p.report, err
	}
	if publish && l.shipped {
		for _, name := range []string{"preview.png", "play.gif", "capture.json"} {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return p.report, err
			}
			if err := os.WriteFile(filepath.Join(source, name), data, 0644); err != nil {
				return p.report, err
			}
		}
	}
	return p.report, nil
}

func (p *player) advance(keys uint16) error {
	last := p.frame.Telemetry.Stats.Frames
	limit := 40
	if last == 0 {
		limit = 2400
	}
	for range limit {
		f, err := p.session.Step(keys, false)
		if err != nil {
			return err
		}
		if f.Overflow {
			return fmt.Errorf("audio overflow")
		}
		if f.Telemetry.DebugValid && f.Telemetry.Stats.Frames > last {
			if f.Telemetry.Stats.HeapError != emulator.HeapOK {
				return fmt.Errorf("heap error")
			}
			p.frame = f
			return nil
		}
	}
	return fmt.Errorf("no new game frame")
}
func (p *pair) step(keys uint16) error {
	if err := p.left.advance(keys); err != nil {
		return err
	}
	if err := p.right.advance(keys); err != nil {
		return err
	}
	a, b := p.left.frame, p.right.frame
	if a.Telemetry.Stats.Frames != b.Telemetry.Stats.Frames {
		return fmt.Errorf("game tick synchronization failed")
	}
	if !bytes.Equal(a.RGBA, b.RGBA) {
		_ = savePNG(filepath.Join(p.out, "mode0-first-difference.png"), a.RGBA)
		_ = savePNG(filepath.Join(p.out, "mode4-first-difference.png"), b.RGBA)
		return fmt.Errorf("renderer pixels differ at game frame %d (captures in %s)", a.Telemetry.Stats.Frames, p.out)
	}
	if fmt.Sprint(a.Telemetry.Watches) != fmt.Sprint(b.Telemetry.Watches) {
		return fmt.Errorf("gameplay watches differ")
	}
	p.report.MatchedFrames++
	p.report.ComparedPixels += 240 * 160
	p.report.Input = append(p.report.Input, keys)
	if p.report.MatchedFrames%2 == 0 && len(p.frames) < 300 {
		if p.colors == nil {
			p.colors = map[uint32]uint8{}
		}
		im := image.NewPaletted(image.Rect(0, 0, 480, 320), nil)
		for y := range 160 {
			for x := range 240 {
				i := (y*240 + x) * 4
				key := uint32(a.RGBA[i])<<16 | uint32(a.RGBA[i+1])<<8 | uint32(a.RGBA[i+2])
				index, ok := p.colors[key]
				if !ok {
					if len(p.palette) >= 256 {
						return fmt.Errorf("native GIF exceeds 256 colors")
					}
					index = uint8(len(p.palette))
					p.colors[key] = index
					p.palette = append(p.palette, color.RGBA{a.RGBA[i], a.RGBA[i+1], a.RGBA[i+2], 255})
				}
				at := y*2*im.Stride + x*2
				im.Pix[at], im.Pix[at+1], im.Pix[at+im.Stride], im.Pix[at+im.Stride+1] = index, index, index, index
			}
		}
		im.Palette = p.palette
		p.frames = append(p.frames, im)
	}
	return nil
}
func (p *pair) hold(keys uint16, n int) error {
	for range n {
		if err := p.step(keys); err != nil {
			return err
		}
	}
	return nil
}
func (p *pair) tap(keys uint16) error {
	for _, k := range []uint16{0, keys, 0} {
		if err := p.step(k); err != nil {
			return err
		}
	}
	return nil
}
func (p *pair) value(name string) float64 {
	for _, w := range p.left.frame.Telemetry.Watches {
		if w.Name == name {
			if w.Kind == 1 {
				return float64(w.Raw)
			}
			return float64(w.Raw) / 65536
		}
	}
	panic("missing watch " + name)
}
func (p *pair) takePreview() {
	p.preview = bytes.Clone(p.left.frame.RGBA)
	p.report.PreviewGameFrame = p.left.frame.Telemetry.Stats.Frames
}
func savePNG(path string, pixels []byte) error {
	im := image.NewRGBA(image.Rect(0, 0, 720, 480))
	for y := range 480 {
		for x := range 720 {
			i := (y/3*240 + x/3) * 4
			copy(im.Pix[y*im.Stride+x*4:], pixels[i:i+4])
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, im)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func saveGallery() error {
	canvas := image.NewRGBA(image.Rect(0, 0, 1440, 960))
	for index, project := range []string{"tidewalk", "moth-study", "moon-door", "lantern-rooms"} {
		f, err := os.Open(filepath.Join("examples", project, "preview.png"))
		if err != nil {
			return err
		}
		picture, err := png.Decode(f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if picture.Bounds().Dx() != 720 || picture.Bounds().Dy() != 480 {
			return fmt.Errorf("unexpected native preview dimensions for %s", project)
		}
		at := image.Pt(index%2*720, index/2*480)
		draw.Draw(canvas, image.Rectangle{Min: at, Max: at.Add(image.Pt(720, 480))}, picture, picture.Bounds().Min, draw.Src)
	}
	f, err := os.Create(filepath.Join("docs", "media", "learning-games.png"))
	if err != nil {
		return err
	}
	err = png.Encode(f, canvas)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
