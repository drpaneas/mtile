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

// Capture the real editor renderer without modifying example assets or user
// preferences. The variant is a documented, disposable tutorial edit.
func TestDocumentationScreenshots(t *testing.T) {
	out := os.Getenv("MTILE_DOC_SCREENSHOTS")
	if out == "" {
		t.Skip("set MTILE_DOC_SCREENSHOTS to regenerate documentation images")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { setInterfaceTextSize(textComfort); setEditorLayout(defaultScreenWidth, defaultScreenHeight) })
	captures := []struct{ name, project string }{
		{"shared-tiles", "background-layers"}, {"poses", "nerdo"},
		{"animations", "nerdo"}, {"color-variants", "nerdo"},
		{"map-rooms", "background-layers"}, {"audio", "audio-gb-melody"},
		{"develop", "nerdo"}, {"preferences", "nerdo"},
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
			g.Layout(1600, 1000)
			switch capture.name {
			case "shared-tiles":
				g.mode = modeSprites
				g.currentSprite = 1
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
			target := ebiten.NewImage(1600, 1000)
			defer target.Deallocate()
			pixels := make([]byte, 1600*1000*4)
			onRenderBenchmarkFrame(func() { g.requestRender(); g.Draw(target); target.ReadPixels(pixels) })
			f, err := os.Create(filepath.Join(out, capture.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, &image.NRGBA{Pix: pixels, Stride: 1600 * 4, Rect: image.Rect(0, 0, 1600, 1000)})
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
		Width, Height int
		Captures      []map[string]string
		Note          string
	}{1600, 1000, nil, "Actual editor Draw output; default COMFORT text and CLEAN effects. Disposable copies of checked-in examples. Color Variants adds alternate and remaps the first used opaque color to index 12. Develop is before Build + Reload; no live ROM or telemetry is simulated."}
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
