package main

import (
	"fmt"
	"strings"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

type developMemoryLine struct {
	text string
	kind uint8 // 0: value, 1: heading, 2: explanation, 3: saving
}

func memoryLine(text string) developMemoryLine    { return developMemoryLine{text: text} }
func memoryHeading(text string) developMemoryLine { return developMemoryLine{text: text, kind: 1} }
func memoryNote(text string) developMemoryLine    { return developMemoryLine{text: text, kind: 2} }

func memoryCount(count int, unit string) string {
	if count != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s", count, unit)
}

func formatMemoryBytes(value uint64) string {
	if value >= 1024*1024 {
		if value%(1024*1024) == 0 {
			return fmt.Sprintf("%d MiB", value/(1024*1024))
		}
		return fmt.Sprintf("%.2f MiB", float64(value)/(1024*1024))
	}
	return formatDevelopBytes(value)
}

// Build data and runtime snapshots have different lifetimes. Never derive ROM
// savings from the current editable sheet, or turn absent telemetry into zero.
func (g *Game) developMemorySections() [4][]developMemoryLine {
	g.runMu.Lock()
	ready, busy, runError := g.runMemoryReady, g.runBusy, g.runError
	romBytes := uint64(max(0, g.runROMSize))
	ewram, free, iwram := g.runEWRAMUsed, g.runEWRAMFree, g.runIWRAMUsed
	report, renderer := g.runPoseMemory, g.runRendererSelected
	currentPreview := g.runID != 0 && g.playRunID == g.runID
	g.runMu.Unlock()
	columns := [4][]developMemoryLine{}
	left, right := &columns[0], &columns[1]
	ram, comparison := &columns[2], &columns[3]
	if !ready || busy {
		state := "Build + Reload to inspect cartridge memory."
		if busy {
			state = "Building cartridge; memory report is pending."
		} else if runError != "" {
			state = "Build failed; see Problems or Build, then retry."
		}
		*left = []developMemoryLine{memoryHeading("CARTRIDGE / ROM"), memoryLine(state), memoryNote("This report describes the last successful build.")}
		*right = []developMemoryLine{memoryHeading("LOADED / POSE CACHE"), memoryLine("Start the preview for live memory information."), memoryNote("Artwork stays in ROM; current poses load into OBJ VRAM.")}
		return columns
	}
	romFree := uint64(0)
	if romBytes < build.ROMCapacity {
		romFree = build.ROMCapacity - romBytes
	}
	*left = append(*left,
		memoryHeading("CARTRIDGE / ROM"),
		memoryLine(fmt.Sprintf("%s used / %s available", formatMemoryBytes(romBytes), formatMemoryBytes(romFree))),
		memoryNote("32 MiB build limit; includes code and all assets."),
		memoryLine(""),
		memoryHeading("POSE LIBRARY / "+memoryCount(report.PoseCount, "pose")+" / "+memoryCount(report.ClipCount, "clip")),
	)
	switch {
	case report.Source == "none":
		*left = append(*left, memoryLine("No pose artwork in this build."))
	case !report.LinkedMemoryAvailable:
		*left = append(*left, memoryLine("Linked artwork sizes unavailable."))
	case report.Source == "shared":
		*left = append(*left, memoryLine("Pixels share the background/sprite sheet."),
			memoryNote(fmt.Sprintf("%s unique pixels referenced by poses.", formatMemoryBytes(report.UniquePixelBytes))))
	case !report.PixelsLinked:
		*left = append(*left, memoryLine("OBJ pixels not retained by this build."),
			memoryNote(fmt.Sprintf("Authored unique artwork: %s.", formatMemoryBytes(report.UniquePixelBytes))))
	default:
		*left = append(*left, memoryLine(fmt.Sprintf("%s pixels stored in ROM", formatMemoryBytes(report.PixelROMBytes))))
	}
	if report.LinkedMemoryAvailable && report.MetadataLinked {
		*left = append(*left, memoryLine(fmt.Sprintf("%s pose/clip/index arrays in ROM", formatMemoryBytes(report.MetadataROMBytes))))
	}
	if report.Source == "obj" && report.LinkedMemoryAvailable && report.PixelsLinked {
		*left = append(*left, memoryLine(fmt.Sprintf("Tile dedup: %s -> %s", formatMemoryBytes(report.SourcePixelBytes), formatMemoryBytes(report.UniquePixelBytes))))
		if report.SourcePixelBytes >= report.UniquePixelBytes {
			*left = append(*left, developMemoryLine{fmt.Sprintf("%s ROM pixels saved", formatMemoryBytes(report.SourcePixelBytes-report.UniquePixelBytes)), 3})
		}
		*left = append(*left, memoryNote("Painted cells only; blank page space is excluded."))
	}
	*ram = append(*ram, memoryLine(""), memoryHeading("RUNTIME RAM / RESERVED BY BUILD"),
		memoryLine(fmt.Sprintf("Main: %s used / %s free", formatMemoryBytes(ewram), formatMemoryBytes(free))),
	)
	if iwram == 0 {
		*ram = append(*ram, memoryLine("Fast RAM usage unavailable."))
	} else {
		*ram = append(*ram, memoryLine(fmt.Sprintf("Fast: %s used / %s free", formatMemoryBytes(iwram), formatMemoryBytes(build.IWRAMCapacity-min(iwram, build.IWRAMCapacity)))))
	}
	*ram = append(*ram,
		memoryNote("Includes runtime state and reserved stacks."),
		memoryNote("Pose buffers are included, not extra RAM."))

	stats := g.playTelemetry.Stats
	*right = append(*right, memoryHeading("LOADED NOW / POSE CACHE"))
	switch {
	case renderer != "mode0":
		*right = append(*right, memoryLine("Bitmap renderer: no streamed OBJ cache."),
			memoryNote("Poses draw from ROM into the framebuffer."))
	case !currentPreview:
		*right = append(*right, memoryLine("Waiting for this build's preview."), memoryNote("Older preview data is not mixed with this build."))
	case !g.playTelemetry.Valid || !g.playTelemetry.DebugValid:
		*right = append(*right, memoryLine("Waiting for live preview telemetry."))
	case !stats.OBJTelemetryValid:
		*right = append(*right, memoryLine("Rebuild the preview for pose cache telemetry."))
	case !stats.OBJStreamAvailable:
		*right = append(*right, memoryLine("No streamed OBJ pose backend in this build."))
	case !stats.OBJStreamResidencyValid:
		*right = append(*right, memoryLine("Pose update pending; residency unavailable."))
	default:
		*right = append(*right,
			memoryLine(fmt.Sprintf("%s cached in OBJ VRAM / %s", formatMemoryBytes(uint64(stats.OBJStreamResidentBytes)), memoryCount(int(stats.OBJStreamResidentSlots), "window"))),
			memoryLine(fmt.Sprintf("%s OBJ upload / frame %d", formatMemoryBytes(uint64(stats.OBJUploadedBytes)), stats.Frames)),
		)
		if stats.OBJStreamResidentSlots == 0 {
			*right = append(*right, memoryLine("No poses cached yet."))
		} else {
			var names []string
			for i := 0; i < min(int(stats.OBJStreamResidentSlots), len(stats.OBJStreamResidentPoseIDs)); i++ {
				id := int(stats.OBJStreamResidentPoseIDs[i])
				name := fmt.Sprintf("pose #%d", id)
				if id < len(report.PoseNames) && report.PoseNames[id] != "" {
					name = fmt.Sprintf("%s (#%d)", report.PoseNames[id], id)
				}
				names = append(names, name)
			}
			if stats.OBJStreamResidentSlots > uint32(len(names)) {
				names = append(names, fmt.Sprintf("+%d more windows", stats.OBJStreamResidentSlots-uint32(len(names))))
			}
			*right = append(*right, memoryLine(strings.Join(names, ", ")))
		}
		*right = append(*right, memoryNote("Cached can include poses not drawn this frame."),
			memoryNote("Upload includes all OBJ tiles; unchanged poses reuse them."))
	}
	*comparison = append(*comparison, memoryHeading("WHY STREAM POSES?"))
	switch {
	case report.Source == "none" || report.UniquePixelBytes == 0:
		*comparison = append(*comparison, memoryLine("Add pose artwork to see the comparison."))
	case renderer != "mode0":
		*comparison = append(*comparison, memoryLine("OBJ streaming comparison applies to Mode 0."))
	case !report.LinkedMemoryAvailable:
		*comparison = append(*comparison, memoryLine("Staging allocation unavailable for this build."))
	case !report.StageLinked:
		*comparison = append(*comparison, memoryLine("No pose staging buffer retained by this build."))
	case report.Source == "shared":
		*comparison = append(*comparison, memoryLine(fmt.Sprintf("Actual pose buffer: %s reserved", formatMemoryBytes(report.StageRAMBytes))),
			memoryNote("Shared-sheet RAM is separate; no net saving calculated."))
	default:
		*comparison = append(*comparison,
			memoryLine(fmt.Sprintf("All art in RAM: %s (comparison)", formatMemoryBytes(report.UniquePixelBytes))),
			memoryLine(fmt.Sprintf("Actual pose buffer: %s reserved", formatMemoryBytes(report.StageRAMBytes))),
		)
		if report.UniquePixelBytes > report.StageRAMBytes {
			*comparison = append(*comparison, developMemoryLine{fmt.Sprintf("%s less pixel-buffer RAM", formatMemoryBytes(report.UniquePixelBytes-report.StageRAMBytes)), 3})
		} else {
			*comparison = append(*comparison, memoryNote("No pixel-buffer saving for this small library."))
		}
		*comparison = append(*comparison, memoryNote("Compares unique library pixels with staging capacity."),
			memoryNote("Artwork only; excludes other RAM and VRAM."))
	}
	return columns
}

func (g *Game) developMemoryLines() []string {
	var lines []string
	for _, column := range g.developMemorySections() {
		for _, line := range column {
			lines = append(lines, line.text)
		}
	}
	return lines
}

// Wrap instead of truncating memory facts, especially long registered pose
// names. Columns share a vertical scroll position and visible scrollbar.
func wrapMemoryLines(lines []developMemoryLine, columns int) []developMemoryLine {
	var wrapped []developMemoryLine
	columns = max(1, columns)
	for _, line := range lines {
		runes := []rune(line.text)
		for len(runes) > columns {
			at := columns
			for i := columns; i > columns/2; i-- {
				if runes[i] == ' ' {
					at = i
					break
				}
			}
			wrapped = append(wrapped, developMemoryLine{string(runes[:at]), line.kind})
			runes = []rune(strings.TrimLeft(string(runes[at:]), " "))
		}
		wrapped = append(wrapped, developMemoryLine{string(runes), line.kind})
	}
	return wrapped
}

func (g *Game) wrappedMemoryColumns(layout developLayout) ([][]developMemoryLine, int) {
	sections := g.developMemorySections()
	columns := [][]developMemoryLine{sections[0], sections[1], sections[3]}
	if layout.Code.Diagnostics.W >= 1400 {
		columns[1] = append(columns[1], sections[2]...)
	} else {
		columns = [][]developMemoryLine{append(sections[0], sections[2]...), append(sections[3], append([]developMemoryLine{memoryLine("")}, sections[1]...)...)}
	}
	width := memoryColumnWidth(layout.Code.Diagnostics.W, len(columns))
	rows := 0
	for i := range columns {
		columns[i] = wrapMemoryLines(columns[i], width/codeCharWidth)
		rows = max(rows, len(columns[i]))
	}
	return columns, rows
}

func memoryColumnWidth(width, count int) int {
	return max(1, (width-42-12*(count-1))/count)
}

func (g *Game) scrollDevelopMemory(layout developLayout, delta int) {
	_, rows := g.wrappedMemoryColumns(layout)
	visible := max(1, (layout.Code.Diagnostics.H-66)/codeLineHeight)
	g.developScroll[developMemory] = min(max(0, g.developScroll[developMemory]+delta), max(0, rows-visible))
}

func (g *Game) drawDevelopMemory(screen *ebiten.Image, layout developLayout) {
	r := layout.Code.Diagnostics
	columns, rows := g.wrappedMemoryColumns(layout)
	visible := max(1, (r.H-66)/codeLineHeight)
	start := min(g.developScroll[developMemory], max(0, rows-visible))
	g.developScroll[developMemory] = start
	colW := memoryColumnWidth(r.W, len(columns))
	for i := 1; i < len(columns); i++ {
		x := r.X + 8 + i*(colW+12)
		drawLine(screen, x, r.Y+44, x, r.Y+r.H-26, colorBorder)
	}
	for i, column := range columns {
		for row := 0; row < visible && start+row < len(column); row++ {
			line := column[start+row]
			clr := colorText
			switch line.kind {
			case 1:
				clr = colorCode
			case 2:
				clr = colorDim
			case 3:
				clr = colorRun
			}
			drawCodeText(screen, line.text, r.X+14+i*(colW+12), r.Y+44+row*codeLineHeight, clr)
		}
	}
	foot := "Last built cartridge / ROM and RAM are separate budgets"
	if rows > visible {
		foot = fmt.Sprintf("Scroll for more / rows %d-%d of %d / last built cartridge", start+1, min(start+visible, rows), rows)
		track := editor.Rect{X: r.X + r.W - 10, Y: r.Y + 44, W: 3, H: r.H - 70}
		fillRect(screen, track, colorBackground)
		thumb := max(12, track.H*visible/rows)
		fillRect(screen, editor.Rect{X: track.X, Y: track.Y + (track.H-thumb)*start/max(1, rows-visible), W: track.W, H: thumb}, colorCode)
	}
	drawDevelopSmallText(screen, truncateDevelopText(foot, r.W-28), r.X+14, r.Y+r.H-20, colorDim)
}
