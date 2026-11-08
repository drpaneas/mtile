package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
)

func memoryTestGame() *Game {
	return &Game{
		runID: 1, playRunID: 1, runReportID: 1,
		runMemoryReady: true, runROMSize: 32768, runEWRAMUsed: 12236, runEWRAMFree: 249396, runIWRAMUsed: 10416,
		runRendererSelected: "mode0",
		runPoseMemory: build.PoseMemoryReport{
			PoseCount: 4, ClipCount: 1, PoseNames: []string{"idle_0", "idle_1", "idle_2", "idle_3"}, Source: "obj",
			SourcePixelBytes: 16384, UniquePixelBytes: 10752, PixelROMBytes: 10752, MetadataROMBytes: 6000,
			StageRAMBytes: 2816, LinkedMemoryAvailable: true, PixelsLinked: true, MetadataLinked: true, StageLinked: true,
		},
		playTelemetry: emulator.Telemetry{Valid: true, DebugValid: true, Stats: emulator.DebugStats{
			Frames: 42, OBJTelemetryValid: true, OBJStreamAvailable: true, OBJStreamResidencyValid: true,
			OBJStreamResidentBytes: 2816, OBJStreamResidentSlots: 1, OBJStreamResidentPoseIDs: [4]uint16{2, 65535, 65535, 65535},
		}},
	}
}

func TestDevelopMemorySeparatesROMRAMAndStreaming(t *testing.T) {
	g := memoryTestGame()
	text := strings.Join(g.developPanelLines(developMemory), "\n")
	for _, want := range []string{
		"32 KiB used", "MiB available", "32 MiB build limit", "4 poses / 1 clip",
		"10.5 KiB pixels stored in ROM", "Tile dedup: 16 KiB -> 10.5 KiB", "5.5 KiB ROM pixels saved",
		"Main: 11.9 KiB used / 243.6 KiB free", "Fast: 10.2 KiB used", "2.8 KiB cached in OBJ VRAM / 1 window",
		"idle_2 (#2)", "0 B OBJ upload / frame 42", "not drawn this frame", "All art in RAM: 10.5 KiB (comparison)",
		"Actual pose buffer: 2.8 KiB reserved", "7.8 KiB less pixel-buffer RAM", "excludes other RAM and VRAM",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestDevelopMemoryNeverInventsZeroLiveUsage(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*Game)
	}{
		{"before build", "Build + Reload", func(g *Game) { g.runMemoryReady = false }},
		{"building without preview", "report is pending", func(g *Game) { g.runBusy, g.playRunID = true, 0 }},
		{"failed build", "Build failed", func(g *Game) { g.runMemoryReady, g.runError = false, "link failed" }},
		{"stopped", "Waiting for live preview", func(g *Game) { g.playTelemetry.Valid = false }},
		{"stale preview", "Waiting for this build's preview", func(g *Game) { g.playRunID = 0 }},
		{"old preview", "Rebuild the preview", func(g *Game) { g.playTelemetry.Stats.OBJTelemetryValid = false }},
		{"no stream", "No streamed OBJ", func(g *Game) { g.playTelemetry.Stats.OBJStreamAvailable = false }},
		{"pending", "residency unavailable", func(g *Game) { g.playTelemetry.Stats.OBJStreamResidencyValid = false }},
		{"mode4", "Bitmap renderer", func(g *Game) { g.runRendererSelected = "mode4" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := memoryTestGame()
			tc.change(g)
			text := strings.Join(g.developMemoryLines(), "\n")
			if !strings.Contains(text, tc.want) || strings.Contains(text, "cached in OBJ VRAM") || strings.Contains(text, "idle_2 (#2)") {
				t.Fatalf("incorrect unavailable state:\n%s", text)
			}
		})
	}
}

func TestDevelopMemoryDoesNotPromiseSavingsForUnlinkedOrSharedArt(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*Game)
	}{
		{"missing ELF", "Linked artwork sizes unavailable", func(g *Game) { g.runPoseMemory.LinkedMemoryAvailable = false }},
		{"elided bank", "OBJ pixels not retained", func(g *Game) { g.runPoseMemory.PixelsLinked, g.runPoseMemory.StageLinked = false, false }},
		{"shared", "Pixels share the background", func(g *Game) { g.runPoseMemory.Source = "shared" }},
		{"bitmap", "comparison applies to Mode 0", func(g *Game) { g.runRendererSelected = "mode4" }},
		{"small bank", "No pixel-buffer saving", func(g *Game) { g.runPoseMemory.UniquePixelBytes = 64 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := memoryTestGame()
			tc.change(g)
			text := strings.Join(g.developMemoryLines(), "\n")
			if !strings.Contains(text, tc.want) {
				t.Fatalf("missing explanation %q:\n%s", tc.want, text)
			}
			if strings.Contains(text, "less pixel-buffer RAM") {
				t.Fatalf("invented RAM saving:\n%s", text)
			}
			if tc.name == "shared" && strings.Contains(text, "ROM pixels saved") {
				t.Fatalf("charged shared-sheet ROM to poses:\n%s", text)
			}
		})
	}
}

func TestDevelopMemoryDoesNotTreatMissingIWRAMAsFree(t *testing.T) {
	g := memoryTestGame()
	g.runIWRAMUsed = 0
	text := strings.Join(g.developMemoryLines(), "\n")
	if !strings.Contains(text, "Fast RAM usage unavailable") || strings.Contains(text, "Fast: 0 B used") {
		t.Fatalf("missing linker data shown as empty fast RAM:\n%s", text)
	}
}

func TestDevelopMemoryListsCachedWindowsWithoutReadingPastIDs(t *testing.T) {
	g := memoryTestGame()
	g.playTelemetry.Stats.OBJStreamResidentSlots = 7
	g.playTelemetry.Stats.OBJStreamResidentPoseIDs = [4]uint16{0, 1, 1, 65535}
	text := strings.Join(g.developMemoryLines(), "\n")
	for _, want := range []string{"idle_0 (#0)", "idle_1 (#1), idle_1 (#1)", "pose #65535", "+3 more windows"} {
		if !strings.Contains(text, want) {
			t.Fatalf("cached windows missing %q:\n%s", want, text)
		}
	}
}

func TestDevelopMemoryLayoutFitsPreviewAndWrapsAllFacts(t *testing.T) {
	t.Cleanup(func() { setInterfaceTextSize(textComfort); setEditorLayout(defaultScreenWidth, defaultScreenHeight) })
	g := memoryTestGame()
	g.runPoseMemory.PoseNames[2] = strings.Repeat("long_pose_", 20)
	g.mode, g.developPanel = modeCode, developMemory
	for _, size := range [][2]int{{1024, 900}, {1920, 1200}, {2560, 1440}, {3840, 2160}} {
		for font := textCompact; font < interfaceTextSizeCount; font++ {
			t.Run(fmt.Sprintf("%dx%d/%d", size[0], size[1], font), func(t *testing.T) {
				setInterfaceTextSize(font)
				setEditorLayout(size[0], size[1])
				layout := newDevelopPanelLayout(developMemory)
				if g.activeCodeLayout().Text != layout.Code.Text {
					t.Fatal("source cursor uses the wrong viewport while Memory is open")
				}
				if !rectInsideIDE(layout.Play.Bezel, layout.Play.Stage) || rectsOverlapIDE(layout.Code.Diagnostics, layout.Play.Inspector) || rectsOverlapIDE(layout.Code.Diagnostics, layout.Code.Editor) {
					t.Fatal("memory panel crowds code or GBA preview")
				}
				for _, tab := range g.developPanelTabRects(layout) {
					if !rectInsideIDE(tab, layout.Code.Diagnostics) {
						t.Fatal("debug tab escapes panel")
					}
				}
				columns, rows := g.wrappedMemoryColumns(layout)
				limit := memoryColumnWidth(layout.Code.Diagnostics.W, len(columns)) / codeCharWidth
				for _, col := range columns {
					for _, line := range col {
						if len([]rune(line.text)) > limit {
							t.Fatalf("memory fact clipped: %q", line.text)
						}
					}
				}
				g.scrollDevelopMemory(layout, 10000)
				visible := max(1, (layout.Code.Diagnostics.H-66)/codeLineHeight)
				if g.developScroll[developMemory] != max(0, rows-visible) {
					t.Fatal("last memory fact cannot be reached by scrolling")
				}
				g.scrollDevelopMemory(layout, -10000)
				if g.developScroll[developMemory] != 0 {
					t.Fatal("cannot return to start of memory panel")
				}
			})
		}
	}
}
