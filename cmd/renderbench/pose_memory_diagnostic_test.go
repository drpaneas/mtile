//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

type poseMemoryNativeBuild struct {
	Renderer   string                 `json:"renderer"`
	ROMPath    string                 `json:"romPath"`
	ROMSHA     string                 `json:"romSha256"`
	ROMBytes   int                    `json:"romBytes"`
	EWRAMUsed  uint64                 `json:"ewramUsed"`
	EWRAMFree  uint64                 `json:"ewramFree"`
	IWRAMUsed  uint64                 `json:"iwramUsed"`
	PoseMemory build.PoseMemoryReport `json:"poseMemory"`
	WorkDir    string                 `json:"keptWorkDir"`
}

type poseMemoryNativeFrame struct {
	LCDStep        int                 `json:"lcdStep"`
	Stats          emulator.DebugStats `json:"stats"`
	PoseNames      []string            `json:"poseNames"`
	ExpectedPoseID int                 `json:"expectedPoseId"`
	FrameSHA       string              `json:"frameSha256"`
	Image          string              `json:"image,omitempty"`
}

type poseMemoryNativeReport struct {
	FirstRuntimeLCDStep int                     `json:"firstRuntimeLCDStep"`
	MedianFrameLCDSteps uint32                  `json:"medianFrameLCDSteps"`
	MaxFrameLCDSteps    uint32                  `json:"maxFrameLCDSteps"`
	SchemaVersion       int                     `json:"schemaVersion"`
	Environment         environmentReport       `json:"environment"`
	Debug               poseMemoryNativeBuild   `json:"debug"`
	Release             poseMemoryNativeBuild   `json:"release"`
	ReleaseBaselinePath string                  `json:"releaseBaselinePath,omitempty"`
	ReleaseBaselineSHA  string                  `json:"releaseBaselineSha256,omitempty"`
	ReleaseIdentical    bool                    `json:"releaseIdentical"`
	WarmupFrames        int                     `json:"warmupFrames"`
	SampledFrames       int                     `json:"sampledFrames"`
	ChangedPoseFrames   int                     `json:"changedPoseFrames"`
	UnchangedPoseFrames int                     `json:"unchangedPoseFrames"`
	DistinctPoses       int                     `json:"distinctPoses"`
	PixelBaselinePath   string                  `json:"pixelBaselinePath,omitempty"`
	PixelsIdentical     bool                    `json:"pixelsIdentical"`
	BaselineFrameCycles objOptimizationCycles   `json:"baselineFrameCycles"`
	FrameCycles         objOptimizationCycles   `json:"frameCycles"`
	CommitCycles        objOptimizationCycles   `json:"commitCycles"`
	ProfilingNote       string                  `json:"profilingNote"`
	Frames              []poseMemoryNativeFrame `json:"representativeFrames"`
}

// TestPoseMemoryTargetDiagnostic exercises the existing native player with the
// new append-only mailbox fields. It never saves or modifies the user's cart.
// MTILE_POSE_MEMORY_REPORT preserves a small report, complete frame telemetry,
// native ROMs/ELFs/maps, and representative images for debugger UI fixtures.
func TestPoseMemoryTargetDiagnostic(t *testing.T) {
	out := os.Getenv("MTILE_POSE_MEMORY_REPORT")
	if out == "" {
		t.Skip("set MTILE_POSE_MEMORY_REPORT for native debugger evidence")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(out, "project")
	copyOBJOptimizationTree(t, filepath.Join(repository, "examples", "nerdo"), project)
	helper := filepath.Join(repository, "build", "player", "mtile-player")
	service, err := build.NewService("", filepath.Join(repository, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.KeepWork = true, true
	environment, err := benchmarkEnvironment(service, project, helper)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := assetsync.LoadAnimationWorkspace(project)
	if err != nil {
		t.Fatal(err)
	}
	var clip *assetsync.AnimationClip
	for i := range workspace.Clips {
		if workspace.Clips[i].Name == "idle" {
			clip = &workspace.Clips[i]
			break
		}
	}
	if clip == nil || len(clip.Frames) < 2 || clip.Loop != nil && !*clip.Loop {
		t.Fatal("Nerdo diagnostic expects the looping multi-pose idle clip")
	}
	const warmup, samples = 2, 120
	baselinePath := os.Getenv("MTILE_POSE_MEMORY_BASELINE")
	if baselinePath == "" {
		baselinePath = "/private/tmp/mtile-obj-optimization-20260924/phase2-native/nerdo/report.json"
	}
	var baseline *objOptimizationReport
	if data, err := os.ReadFile(baselinePath); err == nil {
		baseline = &objOptimizationReport{}
		if err := json.Unmarshal(data, baseline); err != nil {
			t.Fatal(err)
		}
		if baseline.Environment.ProjectSHA != environment.ProjectSHA || baseline.Environment.HelperSHA != environment.HelperSHA {
			t.Fatal("Nerdo source or native player differs from performance baseline")
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var targets []uint32
	if baseline != nil {
		targets = baseline.Mode0.FrameNumbers
	}
	debugBuild, captured, err := buildAndCaptureAtFrames(service, transpiler.RendererMode0, project, helper, out, warmup, samples, targets)
	if err != nil {
		t.Fatal(err)
	}
	if !debugBuild.PoseMemory.LinkedMemoryAvailable || !debugBuild.PoseMemory.StageLinked || debugBuild.PoseMemory.StageRAMBytes == 0 || debugBuild.PoseMemory.PoseCount == 0 {
		t.Fatalf("missing linked pose memory: %+v", debugBuild.PoseMemory)
	}
	report := poseMemoryNativeReport{SchemaVersion: 1, Environment: environment, Debug: poseMemoryBuild(t, debugBuild), WarmupFrames: warmup, SampledFrames: samples, FrameCycles: objOptimizationCycleSummary(captured.cycles, twoFrameCycleBudget), CommitCycles: objOptimizationCycleSummary(captured.commit, 67*1232), ProfilingNote: "LastFrame measures game work and display commit. DebugSnapshot and mailbox publication run afterward, outside this counter; these comparisons do not measure the full debug-only residency scan cost."}
	if baseline != nil {
		report.PixelBaselinePath, report.PixelsIdentical = baselinePath, true
		report.BaselineFrameCycles = baseline.Mode0.Frame
		for i, pixels := range captured.frames {
			if fmt.Sprintf("%x", sha256.Sum256(pixels)) != baseline.Mode0.FrameSHA[i] {
				report.PixelsIdentical = false
			}
		}
		if !report.PixelsIdentical {
			t.Error("new debug telemetry changed displayed Nerdo pixels")
		}
	}
	session, err := emulator.Start(helper, debugBuild.ROMPath)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var frames []poseMemoryNativeFrame
	seenPoses, seenRepresentative := map[int]bool{}, map[string]bool{}
	lastFrame, lastPose := uint32(0), -1
	var initialMisses uint32
	firstRuntimeStep := -1
	var cadence []uint32
	for step := 0; step < (warmup+samples)*16+240 && len(frames) < samples; step++ {
		frame, err := session.Step(0, false)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Overflow {
			t.Fatal("native audio overflow")
		}
		if !frame.Telemetry.DebugValid {
			continue
		}
		stats := frame.Telemetry.Stats
		if firstRuntimeStep < 0 && stats.Frames > 0 {
			firstRuntimeStep = step
		}
		if stats.HeapError != emulator.HeapOK {
			t.Fatalf("native heap error: %+v", stats)
		}
		if stats.Frames <= warmup {
			initialMisses = stats.MissedFrames
			continue
		}
		if stats.Frames == lastFrame {
			continue
		}
		lastFrame = stats.Frames
		if stats.Frames != captured.frameNumbers[len(frames)] {
			t.Fatal("telemetry run skipped a sampled game frame")
		}
		if !stats.OBJTelemetryValid || !stats.OBJStreamAvailable || !stats.OBJStreamResidencyValid {
			t.Fatalf("invalid streamed pose telemetry at frame%d: %+v", stats.Frames, stats)
		}
		if stats.OBJStreamResidentSlots != 1 {
			t.Fatalf("single Nerdo draw should cache one pose window, got%d", stats.OBJStreamResidentSlots)
		}
		id := int(stats.OBJStreamResidentPoseIDs[0])
		expected := clip.Frames[(int(stats.Frames)*clip.FPS/30)%len(clip.Frames)]
		if id != expected || id < 0 || id >= len(debugBuild.PoseMemory.PoseNames) {
			t.Fatalf("frame%d loaded pose%d, expected authored idle pose%d", stats.Frames, id, expected)
		}
		if stats.OBJStreamResidentBytes == 0 || uint64(stats.OBJStreamResidentBytes) > debugBuild.PoseMemory.StageRAMBytes || stats.OBJStreamResidentBytes > 32*1024 || uint64(stats.OBJUploadedBytes) > debugBuild.PoseMemory.StageRAMBytes || stats.OBJUploadedBytes > 32*1024 {
			t.Fatalf("residency/upload exceeds linked stage or OBJ VRAM: %+v", stats)
		}
		changed := lastPose >= 0 && id != lastPose
		if lastPose >= 0 {
			if changed {
				report.ChangedPoseFrames++
				if stats.OBJUploadedBytes == 0 {
					t.Fatal("changed pose did not report an OBJ upload")
				}
			} else {
				report.UnchangedPoseFrames++
				if stats.OBJUploadedBytes != 0 {
					t.Fatal("unchanged Nerdo pose unexpectedly uploaded OBJ pixels")
				}
			}
		}
		if stats.MissedFrames != initialMisses {
			t.Fatal("debug residency telemetry caused a missed game frame")
		}
		row := poseMemoryNativeFrame{LCDStep: step, Stats: stats, PoseNames: []string{debugBuild.PoseMemory.PoseNames[id]}, ExpectedPoseID: expected, FrameSHA: fmt.Sprintf("%x", sha256.Sum256(frame.RGBA))}
		if row.FrameSHA != fmt.Sprintf("%x", sha256.Sum256(captured.frames[len(frames)])) {
			t.Fatal("repeated native telemetry run changed pixels")
		}
		kind := "cached"
		if changed {
			kind = "changed"
		}
		key := fmt.Sprintf("pose%d-%s", id, kind)
		if !seenRepresentative[key] {
			row.Image = key + ".png"
			if err := writePNG(filepath.Join(out, row.Image), frame.RGBA); err != nil {
				t.Fatal(err)
			}
			report.Frames = append(report.Frames, row)
			seenRepresentative[key] = true
		}
		if len(frames) > 0 {
			cadence = append(cadence, uint32(step-frames[len(frames)-1].LCDStep))
		}
		frames = append(frames, row)
		seenPoses[id] = true
		lastPose = id
	}
	if len(frames) != samples || report.ChangedPoseFrames == 0 || report.UnchangedPoseFrames == 0 || len(seenPoses) != len(clip.Frames) {
		t.Fatalf("incomplete animation telemetry: frames%d changed%d unchanged%d poses%d", len(frames), report.ChangedPoseFrames, report.UnchangedPoseFrames, len(seenPoses))
	}
	report.DistinctPoses = len(seenPoses)
	report.FirstRuntimeLCDStep = firstRuntimeStep
	report.MedianFrameLCDSteps, report.MaxFrameLCDSteps = percentile(cadence, 50), percentile(cadence, 100)
	service.Debug, service.Renderer = false, transpiler.RendererMode0
	release, err := service.Build(project, filepath.Join(out, "release.gba"))
	if err != nil {
		t.Fatal(err)
	}
	report.Release = poseMemoryBuild(t, release)
	releaseBaseline := os.Getenv("MTILE_POSE_MEMORY_RELEASE_BASELINE")
	if releaseBaseline == "" {
		releaseBaseline = "/private/tmp/mtile-nerdo-optimized-release.gba"
	}
	if sha, err := fileFingerprint(releaseBaseline); err == nil {
		report.ReleaseBaselinePath, report.ReleaseBaselineSHA = releaseBaseline, sha
		report.ReleaseIdentical = sha == report.Release.ROMSHA
		if !report.ReleaseIdentical {
			t.Error("release ROM changed after debug-only metrics")
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	writePSprJSON(t, filepath.Join(out, "frames.json"), frames)
	writePSprJSON(t, filepath.Join(out, "report.json"), report)
	if report.MaxFrameLCDSteps != 2 || report.MedianFrameLCDSteps != 2 {
		t.Errorf("debug telemetry disrupted 30Hz cadence: median/max %d/%d LCD scans per game frame, want2/2", report.MedianFrameLCDSteps, report.MaxFrameLCDSteps)
	}
	t.Logf("Nerdo: %d real frames, %d pose changes with uploads, %d unchanged frames without uploads, %d authored poses, stage%dB, debugmedian/p95/max%d/%d/%d, release identical=%t; %s", samples, report.ChangedPoseFrames, report.UnchangedPoseFrames, report.DistinctPoses, debugBuild.PoseMemory.StageRAMBytes, report.FrameCycles.Median, report.FrameCycles.P95, report.FrameCycles.Max, report.ReleaseIdentical, out)
	t.Logf("Cadence: first runtime publication at LCDstep%d; median/max LCDsteps per game frame%d/%d", report.FirstRuntimeLCDStep, report.MedianFrameLCDSteps, report.MaxFrameLCDSteps)
}

func poseMemoryBuild(t *testing.T, result build.Result) poseMemoryNativeBuild {
	t.Helper()
	sha, err := fileFingerprint(result.ROMPath)
	if err != nil {
		t.Fatal(err)
	}
	return poseMemoryNativeBuild{Renderer: result.Renderer.String(), ROMPath: result.ROMPath, ROMSHA: sha, ROMBytes: result.ROMSize, EWRAMUsed: result.EWRAMUsed, EWRAMFree: result.EWRAMFree, IWRAMUsed: result.IWRAMUsed, PoseMemory: result.PoseMemory, WorkDir: result.WorkDir}
}
