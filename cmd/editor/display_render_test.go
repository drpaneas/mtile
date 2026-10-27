//go:build mtile_editor_bench

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/hajimehoshi/ebiten/v2"
)

func hiDPIProject(tb testing.TB) *Game {
	tb.Helper()
	dir := tb.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "background-layers"))); err != nil {
		tb.Fatal(err)
	}
	g, err := newGame(dir)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(g.closeNativePerformance)
	g.preferences = defaultEditorPreferences()
	g.applyPreferences()
	g.Layout(1200, 800)
	g.mode = modeCode
	return g
}

func captureHiDPIFrame(g *Game, scale float64) *image.RGBA {
	width, height := int(math.Ceil(1200*scale)), int(math.Ceil(800*scale))
	target := ebiten.NewImage(width, height)
	defer target.Deallocate()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	g.requestRender()
	g.drawAtScale(target, scale)
	target.ReadPixels(img.Pix)
	return img
}

func writeHiDPICapture(t *testing.T, name string, img image.Image) {
	t.Helper()
	out := os.Getenv("MTILE_HIDPI_REPORT")
	if out == "" {
		return
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(out, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("write screenshot: %v / %v", err, closeErr)
	}
}

func TestEditorHiDPIRender(t *testing.T) {
	preserveDisplayState(t)
	g := hiDPIProject(t)
	var before, after *image.RGBA
	onRenderBenchmarkFrame(func() {
		nativeScale := ebiten.Monitor().DeviceScaleFactor()
		t.Logf("native monitor density: %gx", nativeScale)
		window := &editorWindow{Game: g}
		width, height := window.LayoutF(1200, 800)
		if width != 1200*nativeScale || height != 800*nativeScale {
			t.Fatalf("native LayoutF: %gx%g at %gx", width, height, nativeScale)
		}
		t.Logf("native adapter: 1200x800 logical -> %gx%g physical", width, height)
		for _, monitor := range ebiten.AppendMonitors(nil) {
			t.Logf("available monitor: %s (%gx)", monitor.Name(), monitor.DeviceScaleFactor())
		}
		before = captureHiDPIFrame(g, 1)
		for _, scale := range []float64{1.25, 1.5, 2} {
			img := captureHiDPIFrame(g, scale)
			writeHiDPICapture(t, fmt.Sprintf("code-%gx", scale), img)
			if scale == 2 {
				after = img
			}
		}
	})
	writeHiDPICapture(t, "code-1x", before)
	// Reconstruct the former final screen exactly: at an integer display
	// scale, Ebitengine's default presentation uses nearest-neighbor scaling.
	legacy := image.NewRGBA(after.Bounds())
	for y := range legacy.Bounds().Dy() {
		for x := range legacy.Bounds().Dx() {
			legacy.SetRGBA(x, y, before.RGBAAt(x/2, y/2))
		}
	}
	writeHiDPICapture(t, "code-before-2x", legacy)
	if after.Bounds() != image.Rect(0, 0, 2400, 1600) {
		t.Fatalf("2x framebuffer: %v", after.Bounds())
	}
	// A high-density code region must contain new detail within logical
	// pixels. Enlarging a 1x font would make every 2x2 block constant.
	textRect := newDevelopLayout().Code.Text
	var detailedBlocks int
	for y := textRect.Y * 2; y < (textRect.Y+min(200, textRect.H))*2; y += 2 {
		for x := (textRect.X + 8) * 2; x < (textRect.X+textRect.W-16)*2; x += 2 {
			c := after.RGBAAt(x, y)
			if c != after.RGBAAt(x+1, y) || c != after.RGBAAt(x, y+1) || c != after.RGBAAt(x+1, y+1) {
				detailedBlocks++
			}
		}
	}
	if detailedBlocks < 100 {
		t.Fatalf("only %d code blocks have physical-pixel detail", detailedBlocks)
	}
	t.Logf("2x code region: %d logical pixel blocks contain additional glyph detail", detailedBlocks)
	// Check overlay ordering at physical resolution: a quick-open popup
	// must cover the underlying code, with all its own text at native density.
	g.codeQuickOpen = true
	g.updateQuickOpenMatches()
	onRenderBenchmarkFrame(func() { writeHiDPICapture(t, "quick-open-2x", captureHiDPIFrame(g, 2)) })
	g.codeQuickOpen = false
	g.code.SetCursor(g.code.Offset(10, 7), false)
	g.openCodeCompletions()
	if !g.codeCompletionOpen {
		t.Fatal("completion fixture did not open")
	}
	onRenderBenchmarkFrame(func() {
		img := captureHiDPIFrame(g, 2)
		writeHiDPICapture(t, "completion-2x", img)
		layout := newDevelopLayout().Code
		line, column := g.code.LineColumn(g.code.Cursor())
		x := min(layout.Text.X+8+column*codeCharWidth, layout.Text.X+layout.Text.W-230)
		y := min(layout.Text.Y+7+(line+1)*codeLineHeight, layout.Text.Y+layout.Text.H-min(8, len(g.codeCompletions))*24-8)
		// Check the popup border at its expected physical anchor, away from
		// text, so a drawing-scale/input-scale mismatch fails visibly.
		if c := img.RGBAAt((x+100)*2, y*2); c != colorCode {
			t.Fatalf("completion anchor pixel = %v, want %v", c, colorCode)
		}
	})
	g.codeCompletionOpen = false
	g.codeScrollLine = 12
	g.code.SetCursor(g.code.Offset(14, 4), false)
	g.code.SetCursor(g.code.Offset(16, 12), true)
	g.updateTheme()
	onRenderBenchmarkFrame(func() { writeHiDPICapture(t, "selection-scroll-2x", captureHiDPIFrame(g, 2)) })
}

func TestEditorHiDPIPixelArtAndIdle(t *testing.T) {
	preserveDisplayState(t)
	g := hiDPIProject(t)
	onRenderBenchmarkFrame(func() {
		for _, scale := range []float64{1, 1.25, 1.5, 2} {
			editorDrawScale = scale
			// Use the actual emulator presentation helper with a known 240x160
			// source; every physical output pixel must remain an original color.
			g.playImage = ebiten.NewImage(emulator.Width, emulator.Height)
			pixels := make([]byte, emulator.Width*emulator.Height*4)
			a, b := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}
			for y := range emulator.Height {
				for x := range emulator.Width {
					c := a
					if (x+y)%2 == 1 {
						c = b
					}
					i := (y*emulator.Width + x) * 4
					copy(pixels[i:], []byte{c.R, c.G, c.B, c.A})
				}
			}
			g.playImage.WritePixels(pixels)
			target := ebiten.NewImage(int(480*scale), int(320*scale))
			g.drawPlayDisplay(target, playLayout{Screen: editor.Rect{W: 480, H: 320}})
			img := image.NewRGBA(target.Bounds())
			target.ReadPixels(img.Pix)
			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					if c := img.RGBAAt(x, y); c != a && c != b {
						t.Fatalf("%gx pixel art blended at %d,%d: %v", scale, x, y, c)
					}
				}
			}
			// Exercise the authored sprite/animation preview as well as the
			// emulator image path. No intermediate colors or gaps are allowed.
			target.Clear()
			g.drawPosePreview(target, editor.PosePreview{Width: 2, Height: 2, Pixels: []int{1, 2, 2, 1}}, 0, 0, 480, 320, editor.Rect{W: 480, H: 320})
			target.ReadPixels(img.Pix)
			colors := [2]color.RGBA{color.RGBAModel.Convert(g.swatch(1)).(color.RGBA), color.RGBAModel.Convert(g.swatch(2)).(color.RGBA)}
			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					index := (x/(img.Bounds().Dx()/2) + y/(img.Bounds().Dy()/2)) % 2
					if c := img.RGBAAt(x, y); c != colors[index] {
						t.Fatalf("%gx sprite preview pixel %d,%d = %v, want %v", scale, x, y, c, colors[index])
					}
				}
			}
			target.Deallocate()
			g.playImage.Deallocate()
			g.playImage = nil
		}
		editorDrawScale = 1
		// Fill a retained target with a sentinel after its initial draw. An
		// idle draw must not touch it; typing must invalidate it and repaint.
		target := ebiten.NewImage(2400, 1600)
		defer target.Deallocate()
		w := &editorWindow{Game: g, scale: 2}
		g.requestRender()
		w.Draw(target)
		target.Fill(color.RGBA{255, 0, 255, 255})
		pixels := make([]byte, 2400*1600*4)
		target.ReadPixels(pixels)
		before := bytes.Clone(pixels)
		w.Draw(target)
		target.ReadPixels(pixels)
		if !bytes.Equal(before, pixels) {
			t.Fatal("idle draw modified retained framebuffer")
		}
		if allocs := testing.AllocsPerRun(100, func() { w.Draw(target) }); allocs != 0 {
			t.Fatalf("idle draw allocated %g times", allocs)
		}
		g.code.Insert("-- typing\n")
		g.finishCodeInput(newDevelopLayout().Code, true)
		g.requestRender()
		w.Draw(target)
		target.ReadPixels(pixels)
		if bytes.Equal(before, pixels) {
			t.Fatal("typing did not repaint retained framebuffer")
		}
	})
}

func BenchmarkEditorHiDPI(b *testing.B) {
	for _, scale := range []float64{1, 2} {
		for _, activity := range []string{"Idle", "Typing"} {
			b.Run(fmt.Sprintf("%gx/%s", scale, activity), func(b *testing.B) {
				g := hiDPIProject(b)
				target := ebiten.NewImage(int(1200*scale), int(800*scale))
				defer target.Deallocate()
				pixels := make([]byte, int(1200*800*scale*scale*4))
				w := &editorWindow{Game: g, scale: scale}
				layout := newDevelopLayout().Code
				var elapsed time.Duration
				draw := func() {
					start := time.Now()
					if activity == "Typing" {
						g.code.Insert("x")
						g.finishCodeInput(layout, true)
						g.requestRender()
					}
					w.Draw(target)
					if activity == "Typing" {
						target.ReadPixels(pixels)
					}
					elapsed += time.Since(start)
				}
				for range 20 {
					onRenderBenchmarkFrame(draw)
				}
				elapsed = 0
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					onRenderBenchmarkFrame(draw)
				}
				b.StopTimer()
				b.ReportMetric(float64(elapsed.Nanoseconds())/float64(b.N), "work-ns/op")
			})
		}
	}
}
