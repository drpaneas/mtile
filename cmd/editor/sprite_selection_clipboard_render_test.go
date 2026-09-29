//go:build mtile_editor_bench

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/hajimehoshi/ebiten/v2"
)

// This fixture is entirely disposable. In particular, it never reads or saves
// the author's live artwork and never reads or changes the system clipboard.
func TestSelectionClipboardLiveRender(t *testing.T) {
	out := os.Getenv("MTILE_SELECTION_CLIPBOARD_REPORT")
	if out == "" {
		t.Skip("set MTILE_SELECTION_CLIPBOARD_REPORT to capture selection placement")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for _, size := range []struct {
		name          string
		width, height int
	}{{"compact", minimumScreenWidth, minimumScreenHeight}, {"desktop", 1920, 1200}} {
		t.Run(size.name, func(t *testing.T) {
			stubSelectionClipboard(t)
			g, source := newSelectionClipboardGame(t)
			g.preferences = defaultEditorPreferences()
			g.applyPreferences()
			g.Layout(size.width, size.height)
			for index, color := range []assetsync.RGB15{{R: 31, G: 6, B: 4}, {R: 31, G: 27, B: 3}, {R: 3, G: 28, B: 24}, {R: 28, G: 5, B: 26}} {
				g.model.ActivePalette.Colors[4+index] = color
			}
			g.invalidateAnimationPreview()
			if err := g.refreshPreview(); err != nil {
				t.Fatal(err)
			}
			capture := func(name string) {
				t.Helper()
				target := ebiten.NewImage(size.width, size.height)
				defer target.Deallocate()
				pixels := make([]byte, size.width*size.height*4)
				onRenderBenchmarkFrame(func() {
					g.requestRender()
					g.Draw(target)
					target.ReadPixels(pixels)
				})
				file, err := os.Create(filepath.Join(out, size.name+"-"+name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, &image.NRGBA{Pix: pixels, Stride: size.width * 4, Rect: image.Rect(0, 0, size.width, size.height)})
				closeErr := file.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}

			g.copyCurrentSprite()
			g.pasteSpriteClipboard()
			if g.spritePaste == nil || !g.spritePaste.selection {
				t.Fatal("copied tiles did not start selection placement")
			}
			g.moveSpritePastePlacement(16, 12)
			capture("copy-preview")
			g.cancelSpritePastePlacement()

			g.cutCurrentSprite()
			if g.copiedSprite.cut == nil {
				t.Fatal("cut did not retain its source marker")
			}
			capture("cut-source")
			g.selectObjPage(1)
			g.pasteSpriteClipboard()
			if g.spritePaste == nil || g.spritePaste.cut == nil {
				t.Fatal("page switch lost the pending cut")
			}
			destCol, destRow := 16, g.currentObjPageRect().Y/8+12
			g.moveSpritePastePlacement(destCol, destRow)
			capture("cross-page-preview")
			if !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 16, 16), source) {
				t.Fatal("cut preview changed its source artwork")
			}
			if err := g.commitSpritePastePlacement(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(selectionObjectBlock(g, destCol, destRow, 16, 16), source) || !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 16, 16), make([]int, 16*16)) {
				t.Fatal("committed move did not transfer the complete selection")
			}
			capture("committed-move")
		})
	}
}
