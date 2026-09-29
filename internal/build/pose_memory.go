package build

import (
	"debug/elf"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// PoseMemoryReport separates authored artwork from the storage retained by the
// final ARM link. Zero linked bytes are meaningful only when
// LinkedMemoryAvailable is true. None of these fields estimates total game RAM.
type PoseMemoryReport struct {
	PoseCount, ClipCount int
	// PoseNames retains the last build's labels in runtime pose-ID order.
	PoseNames []string
	// Source is "obj" for a dedicated OBJ bank, "shared" for poses backed by
	// the shared sheet, or "none". OBJ statistics cover its entire library;
	// shared statistics cover only distinct logical cells referenced by poses.
	Source string
	// SourcePixelBytes counts painted logical 8x8 cells once each, before
	// merging equal pixels. UniquePixelBytes counts distinct painted cells.
	// Transparent cells/page capacity and repeated references are excluded.
	// UniquePixelBytes also answers how much RAM the library's unique pixels
	// alone would need if fully resident; this excludes runtime/metadata RAM.
	SourcePixelBytes, UniquePixelBytes uint64
	// PixelROMBytes includes linked dedicated OBJ source and resident-layout
	// banks. The shared spritesheet itself is not charged exclusively to poses.
	PixelROMBytes uint64
	// ResidentVRAMBytes is the immutable pose bank copied from ROM into OBJ
	// VRAM during initialization. It needs no EWRAM staging allocation.
	ResidentVRAMBytes uint64
	// MetadataROMBytes covers named pose/clip/OBJ recipe arrays and the OBJ
	// tile index. Anonymous clip-name strings and alignment gaps are excluded.
	MetadataROMBytes uint64
	// StageRAMBytes is the actual linked pose staging allocation, hence the
	// capacity against which live runtime staging use can be displayed.
	StageRAMBytes                             uint64
	LinkedMemoryAvailable                     bool
	PixelsLinked, MetadataLinked, StageLinked bool
}

func summarizePoseMemory(assets *assetsync.Assets, elfPath string) PoseMemoryReport {
	report := analyzePoseMemory(assets)
	binary, err := elf.Open(elfPath)
	if err != nil {
		return report
	}
	defer binary.Close()
	symbols, err := binary.Symbols()
	if err != nil {
		return report
	}
	applyPoseMemorySymbols(&report, symbols)
	return report
}

func analyzePoseMemory(assets *assetsync.Assets) PoseMemoryReport {
	report := PoseMemoryReport{Source: "none"}
	if assets == nil {
		return report
	}
	report.PoseCount, report.ClipCount = len(assets.PoseIndex), len(assets.AnimationClips)
	report.PoseNames = make([]string, report.PoseCount)
	copy(report.PoseNames, assets.PoseNames)
	if assets.ObjSpriteN > 0 {
		report.Source = "obj"
		analysis := assetsync.AnalyzeObjectSheetCells(assets.ObjSheetROM, assets.ObjSheetW, assets.ObjSpriteN)
		report.SourcePixelBytes = uint64(analysis.PaintedTiles) * 64
		report.UniquePixelBytes = uint64(analysis.UniqueTiles) * 64
		return report
	}
	if len(assets.PoseIndex) == 0 {
		return report
	}
	report.Source = "shared"
	if assets.SheetW < 8 || assets.SheetW%8 != 0 {
		return report
	}
	logical := make(map[uint16]bool)
	unique := make(map[[64]byte]bool)
	cols := assets.SheetW / 8
	for _, part := range assets.PoseParts {
		if logical[part.Tile] {
			continue
		}
		logical[part.Tile] = true
		var cell [64]byte
		valid := true
		for row := 0; row < 8; row++ {
			start := (int(part.Tile)/cols*8+row)*assets.SheetW + int(part.Tile)%cols*8
			if start < 0 || start+8 > len(assets.SheetROM) {
				valid = false
				break
			}
			copy(cell[row*8:], assets.SheetROM[start:start+8])
		}
		if !valid || cell == ([64]byte{}) {
			continue
		}
		report.SourcePixelBytes += 64
		unique[cell] = true
	}
	report.UniquePixelBytes = uint64(len(unique)) * 64
	return report
}

func applyPoseMemorySymbols(report *PoseMemoryReport, symbols []elf.Symbol) {
	report.LinkedMemoryAvailable = true
	// Values come from the target ELF rather than host Go struct sizes. In
	// particular, string-bearing clip records have a different host layout.
	seen := make(map[string]bool)
	for _, symbol := range symbols {
		if symbol.Size == 0 || seen[symbol.Name] {
			continue
		}
		seen[symbol.Name] = true
		inROM := symbol.Value >= 0x08000000 && symbol.Value+symbol.Size <= 0x08000000+ROMCapacity
		switch symbol.Name {
		case "main.objSheetROM":
			if inROM {
				report.PixelROMBytes += symbol.Size
				report.PixelsLinked = true
			}
		case "main.poseOBJResidentROM":
			if inROM {
				report.PixelROMBytes += symbol.Size
				report.ResidentVRAMBytes += symbol.Size
				report.PixelsLinked = true
			}
		case "main.objSheetIndexROM", "main.poseIndexROM", "main.posePartROM", "main.animationClipROM", "main.animationFrameROM", "main.poseOBJIndexROM", "main.poseOBJPartsROM", "main.poseOBJTilesROM":
			if inROM {
				report.MetadataROMBytes += symbol.Size
				report.MetadataLinked = true
			}
		case "main.poseOBJStageRAM":
			if symbol.Value >= gbaEWRAMOrigin && symbol.Value+symbol.Size <= gbaEWRAMEnd {
				report.StageRAMBytes += symbol.Size
				report.StageLinked = true
			}
		}
	}
}
