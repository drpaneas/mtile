//go:build mtile_editor_bench

package main

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	documentationWidth        = 1600
	documentationHeight       = 1000
	documentationDisplayScale = 2
)

// Capture the real editor renderer without modifying example assets or user
// preferences. Use the window adapter at 2x so exported text, controls, and
// popups have the same physical-pixel detail as the live Retina interface.
func TestDocumentationScreenshots(t *testing.T) {
	out := os.Getenv("MTILE_DOC_SCREENSHOTS")
	if out == "" {
		t.Skip("set MTILE_DOC_SCREENSHOTS to regenerate documentation images")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	preserveDisplayState(t)
	t.Cleanup(func() { setInterfaceTextSize(textComfort); setEditorLayout(defaultScreenWidth, defaultScreenHeight) })
	captures := []struct{ name, project string }{
		{"shared-tiles", "background-layers"}, {"poses", "nerdo"},
		{"animations", "nerdo"}, {"color-variants", "nerdo"},
		{"map-rooms", "background-layers"}, {"audio", "audio-gb-melody"},
		{"develop", "nerdo"}, {"preferences", "nerdo"},
		{"tile-links", "tile-links"}, {"tile-links-map", "tile-links"},
	}
	for _, capture := range captures {
		t.Run(capture.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", capture.project))); err != nil {
				t.Fatal(err)
			}
			g, err := newGame(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer g.closeNativePerformance()
			g.preferences = defaultEditorPreferences()
			g.applyPreferences()
			window := &editorWindow{Game: g}
			width, height := window.layoutAtScale(documentationWidth, documentationHeight, documentationDisplayScale)
			switch capture.name {
			case "shared-tiles":
				g.mode = modeSprites
				g.currentSprite = 1
			case "tile-links":
				g.mode = modeSprites
				g.currentSprite = 4 // the flower marker, linked to clip "sway"
			case "tile-links-map":
				g.mode = modeMap
				g.prepareMapView()
			case "poses", "animations", "color-variants":
				g.mode = modeSprites
				if err := g.setObjEditing(true); err != nil {
					t.Fatal(err)
				}
				g.selectPose(0)
				if capture.name == "animations" {
					g.setAnimationWorkspaceActive(true)
					g.selectAnimationClip(1)
				}
				if capture.name == "color-variants" {
					if err := g.model.CreatePoseColorVariant("alternate"); err != nil {
						t.Fatal(err)
					}
					colors := g.poseVariantColors()
					if len(colors) == 0 {
						t.Fatal("tutorial pose has no colors")
					}
					if err := g.model.SetPoseColorReplacement("alternate", colors[0], 12); err != nil {
						t.Fatal(err)
					}
					g.poseVariants.open = true
					g.poseVariants.selected = len(g.model.Poses.Variants) - 1
				}
			case "map-rooms":
				g.mode = modeMap
				g.prepareMapView()
				g.mapRooms = true
				g.mapRoomSelected = true
				g.mapRoomCol = 0
				g.mapRoomRow = 0
				w, h := g.mapBounds()
				g.mapCam.Fit(w, h, newMapLayout(screenWidth, screenHeight).Canvas)
				g.mapActiveLayer = 1
			case "audio":
				g.mode = modeMusic
			case "develop", "preferences":
				g.mode = modeCode
				g.preferencesOpen = capture.name == "preferences"
			}
			target := ebiten.NewImage(int(width), int(height))
			defer target.Deallocate()
			img := image.NewRGBA(target.Bounds())
			onRenderBenchmarkFrame(func() { g.requestRender(); window.Draw(target); target.ReadPixels(img.Pix) })
			if img.Bounds() != image.Rect(0, 0, documentationWidth*documentationDisplayScale, documentationHeight*documentationDisplayScale) {
				t.Fatalf("documentation framebuffer: %v", img.Bounds())
			}
			// A stretched 1x screenshot has constant 2x2 pixel blocks. Check
			// the workspace body separately from the shared navigation so every
			// exported surface must demonstrate physical-resolution detail.
			var detailBlocks int
			for y := editorTabsHeight * documentationDisplayScale; y < statusBarRect.Y*documentationDisplayScale; y += 2 {
				for x := 0; x < img.Bounds().Dx(); x += 2 {
					pixel := img.RGBAAt(x, y)
					if pixel != img.RGBAAt(x+1, y) || pixel != img.RGBAAt(x, y+1) || pixel != img.RGBAAt(x+1, y+1) {
						detailBlocks++
					}
				}
			}
			if detailBlocks < 200 {
				t.Fatalf("only %d workspace blocks contain native-resolution detail", detailBlocks)
			}
			t.Logf("%dx%d logical -> %dx%d physical; %d detailed workspace blocks", screenWidth, screenHeight, img.Bounds().Dx(), img.Bounds().Dy(), detailBlocks)
			f, err := os.Create(filepath.Join(out, capture.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		})
	}
	manifest := struct {
		Width, Height               int
		LogicalWidth, LogicalHeight int
		DisplayScale                int
		Captures                    []map[string]string
		Note                        string
	}{
		Width: documentationWidth * documentationDisplayScale, Height: documentationHeight * documentationDisplayScale,
		LogicalWidth: documentationWidth, LogicalHeight: documentationHeight, DisplayScale: documentationDisplayScale,
		Note: "Actual editorWindow Draw output at 2x display density; default COMFORT text and CLEAN effects. Disposable copies of checked-in examples. Color Variants adds alternate and remaps the first used opaque color to index 12. Develop is before Build + Reload; no live ROM or telemetry is simulated.",
	}
	// Export capture descriptions with stable JSON field names.
	rows := make([]map[string]string, 0, len(captures))
	for _, c := range captures {
		rows = append(rows, map[string]string{"image": c.name + ".png", "project": "examples/" + c.project})
	}
	manifest.Captures = rows
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "capture.json"), append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
