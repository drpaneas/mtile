//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

type objOptimizationCycles struct {
	Median              uint32  `json:"median"`
	P95                 uint32  `json:"p95"`
	Max                 uint32  `json:"max"`
	MedianBudgetPercent float64 `json:"medianBudgetPercent"`
	P95BudgetPercent    float64 `json:"p95BudgetPercent"`
	MaxBudgetPercent    float64 `json:"maxBudgetPercent"`
}

type objOptimizationRenderer struct {
	Renderer      string                `json:"renderer"`
	ROMBytes      int                   `json:"romBytes"`
	ROMSHA        string                `json:"romSha256"`
	EWRAMUsed     uint64                `json:"ewramUsed"`
	EWRAMFree     uint64                `json:"ewramFree"`
	IWRAMUsed     uint64                `json:"iwramUsed"`
	IWRAMFree     uint64                `json:"iwramFree"`
	HeapLinked    bool                  `json:"heapLinked"`
	HeapAllocated uint32                `json:"heapAllocated"`
	HeapLive      uint32                `json:"heapLive"`
	HeapPeakLive  uint32                `json:"heapPeakLive"`
	GCCount       uint32                `json:"gcCount"`
	MissedFrames  uint32                `json:"missedFrames"`
	Frame         objOptimizationCycles `json:"frameCycles"`
	Draw          objOptimizationCycles `json:"drawCycles"`
	Commit        objOptimizationCycles `json:"commitCycles"`
	FrameNumbers  []uint32              `json:"frameNumbers"`
	FrameSHA      []string              `json:"frameSha256"`
	Cycles        []uint32              `json:"cycles"`
	DrawCycles    []uint32              `json:"drawCycleSamples"`
	CommitCycles  []uint32              `json:"commitCycleSamples"`
	LCDFrameSHA   [][]string            `json:"lcdFrameSha256"`
	OAMEntries    int                   `json:"oamEntries"`
	OBJLineCycles int                   `json:"objLineCycles"`
	WorkDir       string                `json:"keptWorkDir"`
}

type objOptimizationReport struct {
	SchemaVersion          int                     `json:"schemaVersion"`
	Fixture                string                  `json:"fixture"`
	Environment            environmentReport       `json:"environment"`
	RuntimeSource          string                  `json:"runtimeSource"`
	Debug                  bool                    `json:"debug"`
	WarmupFrames           int                     `json:"warmupFrames"`
	Samples                int                     `json:"samples"`
	FrameBudget            int                     `json:"frameBudgetCycles"`
	VBlankBudget           int                     `json:"vblankBudgetCycles"`
	DifferingPixels        int                     `json:"differingPixels"`
	LCDScans               int                     `json:"lcdScans"`
	LCDDifferences         int                     `json:"lcdDifferingPixels"`
	BeforeAfterPixelsMatch bool                    `json:"beforeAfterPixelsMatch"`
	Mode0                  objOptimizationRenderer `json:"mode0"`
	Mode4                  objOptimizationRenderer `json:"mode4"`
}

// TestOBJOptimizationTargetDiagnostic preserves paired native ARM measurements.
// Set MTILE_OBJ_OPT_OUT to a new directory. For an optimized run, set
// MTILE_OBJ_OPT_COMPARE to the baseline output: source hashes and every sampled
// game frame are checked, including the first LCD scan after each commit.
func TestOBJOptimizationTargetDiagnostic(t *testing.T) {
	out := os.Getenv("MTILE_OBJ_OPT_OUT")
	if out == "" {
		t.Skip("set MTILE_OBJ_OPT_OUT to preserve native evidence")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(repository, "build", "player", "mtile-player")
	for _, name := range []string{"anim-clock", "anim-clips", "fighters", "full-window", "library-180", "nerdo"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(out, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			project := filepath.Join(dir, "project")
			var before *objOptimizationReport
			if previous := os.Getenv("MTILE_OBJ_OPT_COMPARE"); previous != "" {
				data, err := os.ReadFile(filepath.Join(previous, name, "report.json"))
				if err != nil {
					t.Fatal(err)
				}
				before = &objOptimizationReport{}
				if err := json.Unmarshal(data, before); err != nil {
					t.Fatal(err)
				}
				copyOBJOptimizationTree(t, filepath.Join(previous, name, "project"), project)
			} else if name == "nerdo" {
				copyOBJOptimizationTree(t, filepath.Join(repository, "examples", "nerdo"), project)
			} else {
				if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 32, SheetHeight: 16, MapWidth: 30, MapHeight: 20}); err != nil {
					t.Fatal(err)
				}
				if name == "library-180" {
					writeOBJLibraryFixture(t, project)
				} else {
					writePSprStreamFixture(t, project, name)
				}
			}
			// Explicit path prevents a frozen baseline from using the live runtime.
			service, err := build.NewService("", filepath.Join(repository, "runtime"))
			if err != nil {
				t.Fatal(err)
			}
			service.Debug, service.KeepWork = true, true
			environment, err := benchmarkEnvironment(service, project, helper)
			if err != nil {
				t.Fatal(err)
			}
			if before != nil && (before.Environment.ProjectSHA != environment.ProjectSHA || before.Environment.HelperSHA != environment.HelperSHA) {
				t.Fatal("before/after fixture or native player SHA differs")
			}
			const warmup, samples = 2, 120
			var targets []uint32
			if before != nil {
				targets = before.Mode4.FrameNumbers
			}
			m4build, m4, err := buildAndCaptureAtFrames(service, transpiler.RendererMode4, project, helper, dir, warmup, samples, targets)
			if err != nil {
				t.Fatal(err)
			}
			m0build, m0, err := buildAndCaptureAtFrames(service, transpiler.RendererMode0, project, helper, dir, warmup, samples, m4.frameNumbers)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateSynchronizedFrames(m4.frameNumbers, m0.frameNumbers); err != nil {
				t.Fatal(err)
			}
			scans4, err := capturePresentWindow(helper, m4build.ROMPath, warmup+samples)
			if err != nil {
				t.Fatal(err)
			}
			scans0, err := capturePresentWindow(helper, m0build.ROMPath, warmup+samples)
			if err != nil {
				t.Fatal(err)
			}
			report := objOptimizationReport{SchemaVersion: 1, Fixture: name, Environment: environment, RuntimeSource: service.RuntimeDir, Debug: true, WarmupFrames: warmup, Samples: samples, FrameBudget: twoFrameCycleBudget, VBlankBudget: 67 * 1232, DifferingPixels: compareFrames(m4.frames, m0.frames)}
			for tick := warmup; tick < len(scans0); tick++ {
				for _, scan := range scans0[tick].Scans {
					report.LCDScans++
					report.LCDDifferences += countDifferingPixels(scans4[tick].lastRGBA, scan.rgba)
				}
			}
			report.Mode0 = preserveOBJOptimizationRenderer(t, dir, m0build, m0, scans0[warmup:])
			report.Mode4 = preserveOBJOptimizationRenderer(t, dir, m4build, m4, scans4[warmup:])
			if before != nil {
				report.BeforeAfterPixelsMatch = true
				for _, pair := range [][2]objOptimizationRenderer{{before.Mode0, report.Mode0}, {before.Mode4, report.Mode4}} {
					if !reflect.DeepEqual(pair[0].FrameNumbers, pair[1].FrameNumbers) || !reflect.DeepEqual(pair[0].FrameSHA, pair[1].FrameSHA) {
						report.BeforeAfterPixelsMatch = false
					}
					// A faster implementation can present for fewer LCD scans. Every
					// scan must still show the same frame, including its first scan.
					for i, hashes := range pair[1].LCDFrameSHA {
						for _, hash := range hashes {
							previous := pair[0].LCDFrameSHA[i]
							if hash != previous[len(previous)-1] {
								report.BeforeAfterPixelsMatch = false
							}
						}
					}
				}
			}
			writePSprJSON(t, filepath.Join(dir, "report.json"), report)
			t.Logf("%s: Mode0/4 median=%d/%d p95=%d/%d max=%d/%d commitMax=%d; EWRAM=%d/%d ROM=%d/%d; parity=%d LCD=%d; %s", name, report.Mode0.Frame.Median, report.Mode4.Frame.Median, report.Mode0.Frame.P95, report.Mode4.Frame.P95, report.Mode0.Frame.Max, report.Mode4.Frame.Max, report.Mode0.Commit.Max, report.Mode0.EWRAMUsed, report.Mode4.EWRAMUsed, report.Mode0.ROMBytes, report.Mode4.ROMBytes, report.DifferingPixels, report.LCDDifferences, dir)
			if report.DifferingPixels != 0 || report.LCDDifferences != 0 {
				t.Error("Mode0/Mode4 pixel mismatch")
			}
			if before != nil && !report.BeforeAfterPixelsMatch {
				t.Error("before/after pixel mismatch")
			}
			if report.Mode0.Commit.Max >= uint32(report.VBlankBudget) {
				t.Error("Mode0 commit exceeds usable VBlank window")
			}
			animated := false
			for _, hash := range report.Mode0.FrameSHA[1:] {
				if hash != report.Mode0.FrameSHA[0] {
					animated = true
					break
				}
			}
			if !animated {
				t.Error("animation produced no changing frame across the capture")
			}
		})
	}
}

func objOptimizationCycleSummary(values []uint32, budget int) objOptimizationCycles {
	median, p95, maximum := percentile(values, 50), percentile(values, 95), percentile(values, 100)
	return objOptimizationCycles{median, p95, maximum, 100 * float64(median) / float64(budget), 100 * float64(p95) / float64(budget), 100 * float64(maximum) / float64(budget)}
}

func preserveOBJOptimizationRenderer(t *testing.T, dir string, result build.Result, data capture, scans []presentWindowRow) objOptimizationRenderer {
	t.Helper()
	name := result.Renderer.String()
	if err := writePNG(filepath.Join(dir, name+"-final.png"), data.lastRGB); err != nil {
		t.Fatal(err)
	}
	romSHA, err := fileFingerprint(result.ROMPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"game.elf", "game.map", "main.go", "go.mod", "target-gba.json"} {
		bytes, err := os.ReadFile(filepath.Join(result.WorkDir, file))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+"-"+file), bytes, 0644); err != nil {
			t.Fatal(err)
		}
	}
	r := objOptimizationRenderer{Renderer: name, ROMBytes: result.ROMSize, ROMSHA: romSHA, EWRAMUsed: result.EWRAMUsed, EWRAMFree: result.EWRAMFree, IWRAMUsed: result.IWRAMUsed, IWRAMFree: build.IWRAMCapacity - result.IWRAMUsed, HeapLinked: result.HeapLinked, HeapAllocated: data.heapAllocated, HeapLive: data.heapLive, HeapPeakLive: data.heapPeakLive, GCCount: data.gcCount, MissedFrames: data.missed - data.warmupMissed, Frame: objOptimizationCycleSummary(data.cycles, twoFrameCycleBudget), Draw: objOptimizationCycleSummary(data.draw, twoFrameCycleBudget), Commit: objOptimizationCycleSummary(data.commit, 67*1232), FrameNumbers: data.frameNumbers, Cycles: data.cycles, DrawCycles: data.draw, CommitCycles: data.commit, OAMEntries: result.RendererOAMEntries, OBJLineCycles: result.RendererLineCycles, WorkDir: result.WorkDir}
	for _, rgba := range data.frames {
		r.FrameSHA = append(r.FrameSHA, fmt.Sprintf("%x", sha256.Sum256(rgba)))
	}
	for _, row := range scans {
		hashes := make([]string, len(row.Scans))
		for i, scan := range row.Scans {
			hashes[i] = fmt.Sprintf("%x", sha256.Sum256(scan.rgba))
		}
		r.LCDFrameSHA = append(r.LCDFrameSHA, hashes)
	}
	return r
}

func copyOBJOptimizationTree(t *testing.T, source, target string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(target, rel), 0755)
		}
		if !entry.Type().IsRegular() || strings.HasSuffix(path, ".gba") || strings.HasSuffix(path, ".sav") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(target, rel), data, 0644)
	}); err != nil {
		t.Fatal(err)
	}
}

// Six independent 30-pose heroes, each pose 40x80 pixels (50 cells). All 9,000
// cells have distinct nonzero artwork, so pixel dedup cannot understate storage.
// Only two poses are displayed; the other 178 remain ordinary cartridge data.
func writeOBJLibraryFixture(t *testing.T, project string) {
	t.Helper()
	obj := assetsync.Spritesheet{Width: 480, Height: 1200}
	for id := 0; id < 9000; id++ {
		cell := psprFixtureCell(id, 60, id)
		cell.Pixels[0][0], cell.Pixels[0][1] = 1+id%255, 1+id/255
		obj.Sprites = append(obj.Sprites, cell)
	}
	poses := assetsync.PoseFile{Version: 1}
	for id := 0; id < 180; id++ {
		pose := assetsync.PoseRecord{Name: fmt.Sprintf("hero%d_pose%02d", id/30, id%30)}
		x0, y0 := (id%12)*5, (id/12)*10
		for y := 0; y < 10; y++ {
			for x := 0; x < 5; x++ {
				pose.Parts = append(pose.Parts, assetsync.PosePartRecord{Tile: (y0+y)*60 + x0 + x, X: x * 8, Y: y * 8})
			}
		}
		poses.Poses = append(poses.Poses, pose)
	}
	workspace := assetsync.AnimationWorkspace{Version: 1}
	for hero := 0; hero < 6; hero++ {
		clip := assetsync.AnimationClip{Name: fmt.Sprintf("hero%d", hero), FPS: 8}
		for pose := 0; pose < 30; pose++ {
			clip.Frames = append(clip.Frames, hero*30+pose)
		}
		workspace.Clips = append(workspace.Clips, clip)
	}
	writePSprJSON(t, filepath.Join(project, "assets", "objsheet.json"), obj)
	writePSprJSON(t, filepath.Join(project, "assets", "poses.json"), poses)
	writePSprJSON(t, filepath.Join(project, "assets", "animation.json"), workspace)
	if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte("function _draw()\n cls(4)\n anim(\"hero0\", 60, 70)\n anim(\"hero5\", 150, 70, true)\nend\n"), 0644); err != nil {
		t.Fatal(err)
	}
}
