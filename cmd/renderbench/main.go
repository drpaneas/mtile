// Command renderbench builds the same project with both renderer tags, captures
// deterministic headless-mGBA frames/telemetry, and writes a paired JSON gate
// report plus final-frame PNGs. It is an engineering tool, not a public mtile
// subcommand while Mode 0 remains experimental.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

const (
	reportSchemaVersion = 2
	defaultWarmup       = 20
	defaultSamples      = 120
	minimumEWRAMGain    = 24 * 1024
	minimumP95Headroom  = 56179
	mode0MedianPercent  = 80
	twoFrameCycleBudget = 2 * 1232 * 228
	gateFull            = "full"
	gateParity          = "parity"
)

type environmentReport struct {
	GoVersion   string `json:"goVersion"`
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
	TinyGo      string `json:"tinygo"`
	GitRevision string `json:"gitRevision,omitempty"`
	GitDirty    bool   `json:"gitDirty"`
	ProjectSHA  string `json:"projectSha256"`
	HelperSHA   string `json:"helperSha256"`
}

type capture struct {
	frames        [][]byte
	frameNumbers  []uint32
	cycles        []uint32
	draw          []uint32
	commit        []uint32
	missed        uint32
	cold          uint32
	heapAllocated uint32
	heapLive      uint32
	heapPeakLive  uint32
	heapHandles   uint32
	gcCount       uint32
	lastRGB       []byte
}

type rendererResult struct {
	Renderer        string `json:"renderer"`
	ROMPath         string `json:"romPath"`
	ROMSHA          string `json:"romSha256"`
	ROMBytes        int    `json:"romBytes"`
	EWRAMFree       uint64 `json:"ewramFree"`
	CycleSamples    int    `json:"cycleSamples"`
	MedianCycles    uint32 `json:"medianCycles"`
	P95Cycles       uint32 `json:"p95Cycles"`
	MedianDraw      uint32 `json:"medianDrawCycles"`
	MedianCommit    uint32 `json:"medianCommitCycles"`
	ColdUpload      uint32 `json:"coldUploadCycles"`
	MissedFrames    uint32 `json:"missedFrames"`
	FinalFramePNG   string `json:"finalFramePng"`
	SelectionReason string `json:"selectionReason"`
	OAMEntries      int    `json:"mode0OAMEntries,omitempty"`
	OBJLineCycles   int    `json:"mode0OBJLineCycles,omitempty"`
}

type gateReport struct {
	SchemaVersion       int               `json:"schemaVersion"`
	GatePolicy          string            `json:"gatePolicy"`
	Project             string            `json:"project"`
	WorkloadSource      string            `json:"workloadSource,omitempty"`
	Helper              string            `json:"helper"`
	Environment         environmentReport `json:"environment"`
	WarmupFrames        int               `json:"warmupFrames"`
	CapturedFrames      int               `json:"capturedFrames"`
	DifferingPixels     int               `json:"differingPixels"`
	EWRAMGain           int64             `json:"ewramGain"`
	Mode0MedianSavingPC float64           `json:"mode0MedianSavingPercent"`
	Mode0P95Headroom    int64             `json:"mode0P95Headroom"`
	Mode4               rendererResult    `json:"mode4"`
	Mode0               rendererResult    `json:"mode0"`
	ParityPass          bool              `json:"parityPass"`
	MemoryPass          bool              `json:"memoryPass"`
	MedianCyclePass     bool              `json:"medianCyclePass"`
	P95HeadroomPass     bool              `json:"p95HeadroomPass"`
	MissedFramePass     bool              `json:"missedFramePass"`
	Pass                bool              `json:"pass"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "renderbench:", err)
		os.Exit(1)
	}
}

func run() error {
	project := flag.String("project", "examples/testio", "project directory")
	source := flag.String("source", "", "optional game.lua workload override (project is copied to a temporary directory)")
	helper := flag.String("player", "build/player/mtile-player", "headless mtile-player path")
	output := flag.String("out", "", "artifact directory (default: temporary directory)")
	warmup := flag.Int("warmup", defaultWarmup, "mGBA frames discarded before capture")
	samples := flag.Int("samples", defaultSamples, "mGBA frames captured per renderer")
	gate := flag.String("gate", gateFull, "gate policy: full or parity")
	flag.Parse()
	if *warmup < 0 || *samples < 2 {
		return errors.New("warmup must be nonnegative and samples must be at least 2")
	}
	if *gate != gateFull && *gate != gateParity {
		return fmt.Errorf("unknown gate policy %q (want full or parity)", *gate)
	}
	projectPath, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	helperPath, err := filepath.Abs(*helper)
	if err != nil {
		return err
	}
	if info, err := os.Stat(helperPath); err != nil || info.IsDir() {
		return fmt.Errorf("headless player %q is unavailable", helperPath)
	}
	buildProjectPath := projectPath
	workloadSource := ""
	if *source != "" {
		workloadSource, err = filepath.Abs(*source)
		if err != nil {
			return err
		}
		buildProjectPath, err = stageProjectWithSource(projectPath, workloadSource)
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(buildProjectPath) }()
	}
	artifactDir := *output
	if artifactDir == "" {
		artifactDir, err = os.MkdirTemp("", "mtile-renderbench-*")
	} else {
		artifactDir, err = filepath.Abs(artifactDir)
		if err == nil {
			err = os.MkdirAll(artifactDir, 0o755)
		}
	}
	if err != nil {
		return err
	}

	service, err := build.NewService("", "")
	if err != nil {
		return err
	}
	service.Debug = true
	environment, err := benchmarkEnvironment(service, buildProjectPath, helperPath)
	if err != nil {
		return err
	}
	mode4Build, mode4Capture, err := buildAndCapture(service, transpiler.RendererMode4, buildProjectPath, helperPath, artifactDir, *warmup, *samples)
	if err != nil {
		return fmt.Errorf("mode4: %w", err)
	}
	mode0Build, mode0Capture, err := buildAndCaptureAtFrames(service, transpiler.RendererMode0, buildProjectPath, helperPath, artifactDir, *warmup, *samples, mode4Capture.frameNumbers)
	if err != nil {
		return fmt.Errorf("mode0: %w", err)
	}
	if err := validateSynchronizedFrames(mode4Capture.frameNumbers, mode0Capture.frameNumbers); err != nil {
		return err
	}

	differing := compareFrames(mode4Capture.frames, mode0Capture.frames)
	firstDiffIndex, firstDiffPixels := firstDifferingFrame(mode4Capture.frames, mode0Capture.frames)
	mode4PNG := filepath.Join(artifactDir, "mode4-final.png")
	mode0PNG := filepath.Join(artifactDir, "mode0-final.png")
	if err := writePNG(mode4PNG, mode4Capture.lastRGB); err != nil {
		return err
	}
	if err := writePNG(mode0PNG, mode0Capture.lastRGB); err != nil {
		return err
	}
	if firstDiffIndex >= 0 {
		frameNumber := mode4Capture.frameNumbers[firstDiffIndex]
		if err := writePNG(filepath.Join(artifactDir, fmt.Sprintf("mode4-first-diff-%d.png", frameNumber)), mode4Capture.frames[firstDiffIndex]); err != nil {
			return err
		}
		if err := writePNG(filepath.Join(artifactDir, fmt.Sprintf("mode0-first-diff-%d.png", frameNumber)), mode0Capture.frames[firstDiffIndex]); err != nil {
			return err
		}
	}
	mode4Result, err := summarize(mode4Build, mode4Capture, mode4PNG)
	if err != nil {
		return err
	}
	mode0Result, err := summarize(mode0Build, mode0Capture, mode0PNG)
	if err != nil {
		return err
	}
	ewramGain := int64(mode0Build.EWRAMFree) - int64(mode4Build.EWRAMFree)
	saving := 0.0
	if mode4Result.MedianCycles != 0 {
		saving = 100 * (1 - float64(mode0Result.MedianCycles)/float64(mode4Result.MedianCycles))
	}
	headroom := int64(twoFrameCycleBudget) - int64(mode0Result.P95Cycles)
	report := gateReport{
		SchemaVersion: reportSchemaVersion, Project: projectPath, WorkloadSource: workloadSource, Helper: helperPath, Environment: environment,
		WarmupFrames: *warmup, CapturedFrames: *samples,
		DifferingPixels: differing, EWRAMGain: ewramGain, Mode0MedianSavingPC: saving, Mode0P95Headroom: headroom,
		Mode4: mode4Result, Mode0: mode0Result,
		ParityPass:      differing == 0,
		MemoryPass:      ewramGain >= minimumEWRAMGain,
		MedianCyclePass: uint64(mode0Result.MedianCycles)*100 <= uint64(mode4Result.MedianCycles)*mode0MedianPercent,
		P95HeadroomPass: headroom >= minimumP95Headroom,
		MissedFramePass: mode4Capture.missed == 0 && mode0Capture.missed == 0,
	}
	if err := applyGatePolicy(&report, *gate); err != nil {
		return err
	}
	reportPath := filepath.Join(artifactDir, "report.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("report %s\n", reportPath)
	fmt.Printf("gate %s, pixels %d, EWRAM %+d, median %.1f%%, Mode0 p95 headroom %d, pass=%t\n", report.GatePolicy, differing, ewramGain, saving, headroom, report.Pass)
	if firstDiffIndex >= 0 {
		fmt.Printf("first parity mismatch at runtime frame %d: %d pixels\n", mode4Capture.frameNumbers[firstDiffIndex], firstDiffPixels)
	}
	if !report.Pass {
		return errors.New("one or more renderer gates failed; inspect report.json")
	}
	return nil
}

func applyGatePolicy(report *gateReport, policy string) error {
	if report == nil {
		return errors.New("renderer gate report is required")
	}
	report.GatePolicy = policy
	switch policy {
	case gateFull:
		report.Pass = report.ParityPass && report.MemoryPass && report.MedianCyclePass && report.P95HeadroomPass && report.MissedFramePass
	case gateParity:
		// Characterization workloads lock down exact output. Frame delivery and
		// resource measurements remain visible evidence, but are not acceptance
		// criteria and do not widen the AUTO performance profile.
		report.Pass = report.ParityPass
	default:
		return fmt.Errorf("unknown gate policy %q (want full or parity)", policy)
	}
	return nil
}

func stageProjectWithSource(projectPath, sourcePath string) (string, error) {
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read workload source: %w", err)
	}
	staged, err := os.MkdirTemp("", "mtile-renderbench-project-*")
	if err != nil {
		return "", err
	}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(staged)
		}
	}()
	err = filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(projectPath, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(staged, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".gba" || extension == ".sav" || relative == build.GameFileName {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return closeErr
	})
	if err != nil {
		return "", fmt.Errorf("stage project: %w", err)
	}
	if err := os.WriteFile(filepath.Join(staged, build.GameFileName), source, 0o644); err != nil {
		return "", fmt.Errorf("stage workload source: %w", err)
	}
	failed = false
	return staged, nil
}

func benchmarkEnvironment(service build.Service, projectPath, helperPath string) (environmentReport, error) {
	projectSHA, err := projectFingerprint(projectPath)
	if err != nil {
		return environmentReport{}, fmt.Errorf("fingerprint project: %w", err)
	}
	helperSHA, err := fileFingerprint(helperPath)
	if err != nil {
		return environmentReport{}, fmt.Errorf("fingerprint headless player: %w", err)
	}
	tinygo, err := commandOutput(service.TinyGoBin, "version")
	if err != nil {
		return environmentReport{}, fmt.Errorf("identify TinyGo: %w", err)
	}
	repository := filepath.Dir(service.RuntimeDir)
	revision, _ := commandOutput("git", "-C", repository, "rev-parse", "HEAD")
	dirtyOutput, dirtyErr := commandOutput("git", "-C", repository, "status", "--porcelain")
	return environmentReport{
		GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		TinyGo: tinygo, GitRevision: revision, GitDirty: dirtyErr == nil && dirtyOutput != "",
		ProjectSHA: projectSHA, HelperSHA: helperSHA,
	}, nil
}

func commandOutput(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func fileFingerprint(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// projectFingerprint identifies the complete benchmark input while ignoring
// ROM/save outputs produced by local runs. Paths are included in the digest so
// moving bytes between files cannot accidentally preserve the fingerprint.
func projectFingerprint(root string) (string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".gba" || extension == ".sav" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	digest := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = digest.Write([]byte(filepath.ToSlash(relative)))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(data)
		_, _ = digest.Write([]byte{0})
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func buildAndCapture(service build.Service, renderer transpiler.Renderer, project, helper, output string, warmup, samples int) (build.Result, capture, error) {
	return buildAndCaptureAtFrames(service, renderer, project, helper, output, warmup, samples, nil)
}

// buildAndCaptureAtFrames captures exact runtime frame numbers when targets is
// non-empty. Paired renderer runs must compare the same game ticks: a slow ROM
// can publish fewer debug snapshots per host step, so ordinal capture slots are
// not sufficient synchronization for moving scenes.
func buildAndCaptureAtFrames(service build.Service, renderer transpiler.Renderer, project, helper, output string, warmup, samples int, targets []uint32) (build.Result, capture, error) {
	if len(targets) != 0 && len(targets) != samples {
		return build.Result{}, capture{}, fmt.Errorf("capture target count %d does not match requested samples %d", len(targets), samples)
	}
	service.Renderer = renderer
	rom := filepath.Join(output, renderer.String()+".gba")
	result, err := service.Build(project, rom)
	if err != nil {
		return build.Result{}, capture{}, err
	}
	session, err := emulator.Start(helper, rom)
	if err != nil {
		return build.Result{}, capture{}, err
	}
	defer func() { _ = session.Close() }()

	resultCapture := capture{
		frames:       make([][]byte, 0, samples),
		frameNumbers: make([]uint32, 0, samples),
		cycles:       make([]uint32, 0, samples),
	}
	var lastSequence, lastProfileFrame uint32
	// Emulator startup reaches the generated loop at a renderer-dependent LCD
	// scan. Comparing the Nth host Step therefore aliases moving scenes even
	// when both renderers are exact. Capture unique debug publications keyed by
	// the runtime's 30 Hz frame counter instead: frame 21 is then frame 21 in
	// both ROMs, independent of boot and presentation latency.
	// Moving multi-BG Mode 4 scenes can publish slightly fewer unique debug
	// frames per host step during startup. Keep a generous bounded ceiling so
	// the requested sample count, rather than startup phase, decides the run.
	maxProfileFrame := warmup + samples
	if len(targets) != 0 && int(targets[len(targets)-1]) > maxProfileFrame {
		maxProfileFrame = int(targets[len(targets)-1])
	}
	maxSteps := maxProfileFrame*5 + 240
	for step := 0; step < maxSteps && len(resultCapture.frames) < samples; step++ {
		frame, err := session.Step(0, false)
		if err != nil {
			return build.Result{}, capture{}, err
		}
		if !frame.Telemetry.DebugValid || frame.Telemetry.Sequence == lastSequence {
			continue
		}
		lastSequence = frame.Telemetry.Sequence
		stats := frame.Telemetry.Stats
		if stats.LastBlit > resultCapture.cold {
			resultCapture.cold = stats.LastBlit
		}
		if stats.Frames == lastProfileFrame || stats.Frames <= uint32(warmup) {
			continue
		}
		lastProfileFrame = stats.Frames
		if len(targets) != 0 {
			want := targets[len(resultCapture.frames)]
			if stats.Frames < want {
				continue
			}
			if stats.Frames > want {
				return build.Result{}, capture{}, fmt.Errorf("debug telemetry skipped target runtime frame %d (next publication is %d)", want, stats.Frames)
			}
		}
		resultCapture.cycles = append(resultCapture.cycles, stats.LastFrame)
		resultCapture.draw = append(resultCapture.draw, stats.LastDraw)
		resultCapture.commit = append(resultCapture.commit, stats.LastBlit)
		resultCapture.missed = stats.MissedFrames
		resultCapture.heapAllocated = stats.HeapAllocated
		resultCapture.heapLive = stats.HeapLive
		resultCapture.heapPeakLive = stats.HeapPeakLive
		resultCapture.heapHandles = stats.HeapHandles
		resultCapture.gcCount = stats.GCCount
		pixels := append([]byte(nil), frame.RGBA...)
		resultCapture.frames = append(resultCapture.frames, pixels)
		resultCapture.frameNumbers = append(resultCapture.frameNumbers, stats.Frames)
		resultCapture.lastRGB = pixels
	}
	if len(resultCapture.frames) != samples || len(resultCapture.cycles) != samples {
		return build.Result{}, capture{}, fmt.Errorf("debug telemetry produced %d/%d synchronized frame samples", len(resultCapture.frames), samples)
	}
	return result, resultCapture, nil
}

func validateSynchronizedFrames(left, right []uint32) error {
	if len(left) != len(right) {
		return fmt.Errorf("renderer captures contain %d and %d runtime frame numbers", len(left), len(right))
	}
	for index := range left {
		if left[index] != right[index] {
			return fmt.Errorf("renderer capture %d compares runtime frame %d with %d", index, left[index], right[index])
		}
	}
	return nil
}

func summarize(result build.Result, captured capture, pngPath string) (rendererResult, error) {
	romSHA, err := fileFingerprint(result.ROMPath)
	if err != nil {
		return rendererResult{}, fmt.Errorf("fingerprint %s ROM: %w", result.Renderer, err)
	}
	coldUpload := uint32(0)
	if result.Renderer == transpiler.RendererMode0 {
		coldUpload = captured.cold
	}
	return rendererResult{
		Renderer: result.Renderer.String(), ROMPath: result.ROMPath, ROMSHA: romSHA, ROMBytes: result.ROMSize, EWRAMFree: result.EWRAMFree,
		CycleSamples: len(captured.cycles), MedianCycles: percentile(captured.cycles, 50), P95Cycles: percentile(captured.cycles, 95),
		MedianDraw: percentile(captured.draw, 50), MedianCommit: percentile(captured.commit, 50), ColdUpload: coldUpload, MissedFrames: captured.missed,
		FinalFramePNG: pngPath, SelectionReason: result.RendererReason,
		OAMEntries: result.RendererOAMEntries, OBJLineCycles: result.RendererLineCycles,
	}, nil
}

func percentile(values []uint32, percent int) uint32 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]uint32(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := (len(ordered)*percent + 99) / 100
	if index < 1 {
		index = 1
	}
	return ordered[index-1]
}

func compareFrames(left, right [][]byte) int {
	count := len(left)
	if len(right) < count {
		count = len(right)
	}
	differing := 0
	for frame := 0; frame < count; frame++ {
		differing += countDifferingPixels(left[frame], right[frame])
	}
	return differing
}

func firstDifferingFrame(left, right [][]byte) (index, pixels int) {
	count := len(left)
	if len(right) < count {
		count = len(right)
	}
	for frame := 0; frame < count; frame++ {
		if differing := countDifferingPixels(left[frame], right[frame]); differing != 0 {
			return frame, differing
		}
	}
	return -1, 0
}

func countDifferingPixels(left, right []byte) int {
	differing := 0
	for pixel := 0; pixel+3 < len(left) && pixel+3 < len(right); pixel += 4 {
		if left[pixel] != right[pixel] || left[pixel+1] != right[pixel+1] || left[pixel+2] != right[pixel+2] || left[pixel+3] != right[pixel+3] {
			differing++
		}
	}
	return differing
}

func writePNG(path string, rgba []byte) error {
	if len(rgba) != emulator.Width*emulator.Height*4 {
		return fmt.Errorf("write PNG: got %d RGBA bytes", len(rgba))
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	imageValue := &image.RGBA{Pix: rgba, Stride: emulator.Width * 4, Rect: image.Rect(0, 0, emulator.Width, emulator.Height)}
	encodeErr := png.Encode(file, imageValue)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}
