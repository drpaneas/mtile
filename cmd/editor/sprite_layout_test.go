package main

import (
	"fmt"
	"testing"

	"github.com/drpaneas/mtile/internal/editor"
)

func TestSpritePanelHeadingGapsAreConsistent(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for textSize := textCompact; textSize < interfaceTextSizeCount; textSize++ {
		for _, size := range [][2]int{{1024, 900}, {1280, 1024}, {1541, 1058}, {1920, 1200}, {2560, 1440}, {3840, 2160}} {
			t.Run(fmt.Sprintf("%s/%dx%d", textSize, size[0], size[1]), func(t *testing.T) {
				setInterfaceTextSize(textSize)
				setEditorLayout(size[0], size[1])
				g := &Game{}
				sections := map[string][2]editor.Rect{
					"sprite":       {spriteCanvasPanelRect, canvasRect},
					"draw tools":   {spriteToolsPanelRect, spriteToolButtonRect(0)},
					"color":        {spritePalettePanelRect, swatchRect(0)},
					"sprite flags": {spriteFlagsPanelRect, flagCheckboxRect(0)},
					"spritesheet":  {sheetPanelRect, g.sheetGridCanvas()},
				}
				for name, section := range sections {
					_, titleY := spritePanelTitlePosition(section[0])
					if gap := section[1].Y - (titleY + uiTextHeight); gap != 8 {
						t.Errorf("%s heading-to-content gap = %d, want 8", name, gap)
					}
				}
				_, titleY := colorPickerTitlePosition()
				firstContent := pickerSliderRect(0)
				if spritePickerPanelRect.W == 0 {
					firstContent = colorPickerPreviewRect()
					if gap := pickerSliderRect(0).Y - (firstContent.Y + firstContent.H); gap != 8 {
						t.Errorf("compact preview-to-sliders gap = %d, want 8", gap)
					}
				}
				if gap := firstContent.Y - (titleY + uiTextHeight); gap != 8 {
					t.Errorf("change color heading-to-content gap = %d, want 8", gap)
				}
			})
		}
	}
}

func TestSpriteInspectorFitsAcrossTextSizesAndHeightBreakpoints(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for textSize := textCompact; textSize < interfaceTextSizeCount; textSize++ {
		setInterfaceTextSize(textSize)
		// Check both sides of every compact/desktop transition, not just a
		// handful of common window sizes. Typography changes that breakpoint.
		for height := minimumScreenHeight; height <= 1250; height++ {
			setEditorLayout(minimumScreenWidth, height)
			panels := []editor.Rect{spriteCanvasPanelRect, spriteToolsPanelRect, spritePalettePanelRect, spriteFlagsPanelRect}
			pickerParent := spritePickerPanelRect
			if pickerParent.W > 0 {
				panels = append(panels, pickerParent)
			} else {
				pickerParent = spritePalettePanelRect
				if rectsOverlapIDE(paletteRect, pickerRect) {
					t.Fatalf("%s/%d compact palette overlaps picker", textSize, height)
				}
			}
			for index, panel := range panels {
				if !rectInsideIDE(panel, spriteRailRect) {
					t.Fatalf("%s/%d panel %d escapes rail: %#v in %#v", textSize, height, index, panel, spriteRailRect)
				}
				if index > 0 && rectsOverlapIDE(panels[index-1], panel) {
					t.Fatalf("%s/%d panels %d and %d overlap", textSize, height, index-1, index)
				}
			}
			for name, pair := range map[string][2]editor.Rect{
				"canvas":         {canvasRect, spriteCanvasPanelRect},
				"last tool":      {spriteToolButtonRect(spriteToolButtonCount - 1), spriteToolsPanelRect},
				"palette":        {paletteRect, spritePalettePanelRect},
				"last flag":      {flagCheckboxRect(7), spriteFlagsPanelRect},
				"picker preview": {colorPickerPreviewRect(), pickerParent},
				"last slider":    {pickerSliderRect(2), pickerParent},
			} {
				if !rectInsideIDE(pair[0], pair[1]) {
					t.Fatalf("%s/%d %s escapes panel: %#v in %#v", textSize, height, name, pair[0], pair[1])
				}
			}
			footerBottom := canvasRect.Y + canvasRect.H + spriteCanvasFooterGap + uiSmallTextHeight
			if footerBottom+spritePanelPadding > spriteCanvasPanelRect.Y+spriteCanvasPanelRect.H {
				t.Fatalf("%s/%d canvas footer is clipped", textSize, height)
			}
			if height == defaultScreenHeight && (canvasRect.W < 320 || spritePickerPanelRect.W == 0) {
				t.Fatalf("%s default-height window lost its desktop editing scale", textSize)
			}
		}
	}
}

func TestSpriteCanvasZoomControlsStayInsideFooter(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for textSize := textCompact; textSize < interfaceTextSizeCount; textSize++ {
		setInterfaceTextSize(textSize)
		for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}, {3840, 2160}} {
			setEditorLayout(size[0], size[1])
			minus, plus := spriteCanvasZoomButtonRect(-1), spriteCanvasZoomButtonRect(1)
			for name, button := range map[string]editor.Rect{"zoom out": minus, "zoom in": plus} {
				if !rectInsideIDE(button, spriteCanvasPanelRect) {
					t.Fatalf("%s/%dx%d %s button escapes canvas panel: %#v", textSize, size[0], size[1], name, button)
				}
				if button.Y-canvasRect.Y-canvasRect.H != spriteCanvasFooterGap {
					t.Fatalf("%s/%dx%d %s button has inconsistent footer gap", textSize, size[0], size[1], name)
				}
			}
			if rectsOverlapIDE(minus, plus) {
				t.Fatalf("%s/%dx%d zoom buttons overlap", textSize, size[0], size[1])
			}
		}
	}
}
