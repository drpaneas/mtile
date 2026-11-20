package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// runProduction measures the shipped implementation without patching runtime or
// generated font data. Historical experiments must run at their recorded commit.
func runProduction(root, out string) error {
	dir := filepath.Join(out, "production-b")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	project := filepath.Join(dir, "project")
	if _, err := os.Stat(project); os.IsNotExist(err) {
		if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
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
	service, err := build.NewService("", "")
	if err != nil {
		return err
	}
	service.Debug, service.KeepWork = true, true
	var records []measurement
	for _, work := range []string{"empty", "hud", "unicode", "dense", "edges"} {
		source := productionWorkload(work)
		if err = os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0644); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, work+".lua"), []byte(source), 0644); err != nil {
			return err
		}
		modes := []transpiler.Renderer{transpiler.RendererMode4}
		if work == "empty" || work == "hud" || work == "edges" {
			modes = append(modes, transpiler.RendererMode0)
		}
		var reference capture
		for _, mode := range modes {
			name := work + "-" + mode.String()
			fmt.Println("production-b", name)
			service.Renderer = mode
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
			if err := os.RemoveAll(result.WorkDir); err != nil {
				return err
			}
			var targets []uint32
			if mode == transpiler.RendererMode0 {
				targets = reference.Frames
			}
			captured, err := captureROM(filepath.Join(root, "build/player/mtile-player"), result.ROMPath, targets)
			if err != nil {
				return err
			}
			if err = writePNG(filepath.Join(dir, name+".png"), captured.Pixels[len(captured.Pixels)-1]); err != nil {
				return err
			}
			record := measurement{Variant: "production-b", Workload: work, Renderer: mode.String(), ROMBytes: result.ROMSize, FontROMBytes: fontBytes, EWRAMUsed: result.EWRAMUsed, IWRAMUsed: result.IWRAMUsed, MedianFrame: percentile(captured.Cycles, 50), P95Frame: percentile(captured.Cycles, 95), MedianDraw: percentile(captured.Draw, 50), MedianCommit: percentile(captured.Commit, 50), MissedFrames: captured.Missed, HeapAllocated: captured.Heap, Samples: len(captured.Frames)}
			raw, err := os.ReadFile(result.ROMPath)
			if err != nil {
				return err
			}
			record.ROMSHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
			if mode == transpiler.RendererMode0 {
				for i, pixels := range captured.Pixels {
					for n, v := range pixels {
						if v != reference.Pixels[i][n] {
							record.ParityDiff++
						}
					}
				}
				if record.ParityDiff != 0 {
					return fmt.Errorf("%s parity differs by %d bytes", name, record.ParityDiff)
				}
			} else {
				reference = captured
			}
			records = append(records, record)
			raw, err = json.MarshalIndent(records, "", "  ")
			if err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(out, "production.json"), raw, 0644); err != nil {
				return err
			}
			fmt.Printf("  median %d cycles, ROM %d, font %d, EWRAM %d, IWRAM %d, parity %d\n", record.MedianFrame, record.ROMBytes, fontBytes, record.EWRAMUsed, record.IWRAMUsed, record.ParityDiff)
		}
	}
	return nil
}

func productionWorkload(name string) string {
	if name == "hud" {
		return `score=0
function _update() score=(score+1)%1000 end
function _draw()
 cls(24)
 print("HP",0,0,15)
 print(score,36,0,15)
 print("HP12HP12HP12HP12HP12HP12HP12HP12HP12HP12",0,16,15)
end
`
	}
	return workload(name, 6, 10)
}
