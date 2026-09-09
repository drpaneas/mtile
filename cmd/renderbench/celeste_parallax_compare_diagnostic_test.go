//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
)

// Compare fresh source snapshots using one frozen runtime and identical input.
// The source root override can point at a frozen checkout; Go still compiles
// this diagnostic and the transpiler from the checkout where it is invoked.
//
//	MTILE_CELESTE_PARALLAX_COMPARE=1 MTILE_CELESTE_PARALLAX_OUT=/tmp/celeste-compare \
//	  GOCACHE=/tmp/mtile-go-cache go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
//	  -run '^TestCelesteParallaxCompareDiagnostic$' -count=1 -timeout=20m -v
//
// Optional MTILE_CELESTE_PARALLAX_ROOT supplies examples/ and runtime/.
// Outputs remain in the evidence directory. Editable sources are never built
// in place. Cross-source RNG/cosmetic changes are measured, not called parity.
func TestCelesteParallaxCompareDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_CELESTE_PARALLAX_COMPARE") != "1" {
		t.Skip("set MTILE_CELESTE_PARALLAX_COMPARE=1 for the fresh Celeste comparison")
	}
	t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_PROJECT", "")
	t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS", "")
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("MTILE_CELESTE_PARALLAX_ROOT")
	if root == "" {
		root = repository
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("MTILE_CELESTE_PARALLAX_OUT")
	if out == "" {
		out, err = os.MkdirTemp("", "mtile-celeste-parallax-compare-")
	} else {
		out, err = filepath.Abs(out)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	// Refuse to mix a rerun with old evidence or overwrite an existing snapshot.
	if err := os.Mkdir(filepath.Join(out, "snapshot"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Log("evidence:", out)
	report := celesteParallaxComparisonReport{
		SchemaVersion: 1, StartedUTC: time.Now().UTC().Format(time.RFC3339),
		SourceRoot: root, CompilerRoot: repository, GoVersion: runtime.Version(),
		Sources: make(map[string]string), Variants: make(map[string]celesteParallaxVariantSummary),
		Scope: "280 deterministic logical updates: spawn, completion route, completion UI, Start, injected centered death, respawn; native mGBA, debug/audio enabled",
	}
	defer func() {
		report.Passed = !t.Failed() && len(report.Variants) == 3 && report.RendererParity
		if err := os.WriteFile(filepath.Join(out, "summary.json"), mustCelesteParallaxJSON(t, report), 0644); err != nil {
			t.Error(err)
		}
	}()
	frozenRuntime := filepath.Join(out, "snapshot", "runtime")
	if err := tileCanvasCopyTree(filepath.Join(root, "runtime"), frozenRuntime, false); err != nil {
		t.Fatal(err)
	}
	report.RuntimeSHA = celesteParallaxFingerprint(t, frozenRuntime)
	service, err := build.NewService("", frozenRuntime)
	if err != nil {
		t.Fatal(err)
	}
	report.TinyGoVersion, err = commandOutput(service.TinyGoBin, "version")
	if err != nil {
		t.Fatal(err)
	}
	report.PlayerSHA, err = fileFingerprint(filepath.Join(repository, "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	projects := make(map[string]string)
	var inputs []int
	for _, name := range []string{"celeste", "celeste-parallax"} {
		source := filepath.Join(root, "examples", name)
		snapshot := filepath.Join(out, "snapshot", name)
		if err := tileCanvasCopyTree(source, snapshot, true); err != nil {
			t.Fatal(err)
		}
		routeData, err := os.ReadFile(filepath.Join(source, "tests", "level_completion_route.json"))
		if err != nil {
			t.Fatal(err)
		}
		var route struct{ Inputs []int }
		if err := json.Unmarshal(routeData, &route); err != nil || len(route.Inputs) == 0 {
			t.Fatalf("%s completion route: %v", name, err)
		}
		if inputs == nil {
			inputs = route.Inputs
		} else if !reflect.DeepEqual(inputs, route.Inputs) {
			t.Fatal("source projects have different completion inputs")
		}
		if err := os.WriteFile(filepath.Join(snapshot, "completion-route.json"), routeData, 0644); err != nil {
			t.Fatal(err)
		}
		report.Sources[name] = celesteParallaxFingerprint(t, snapshot)
		project := filepath.Join(out, name+"-instrumented")
		if err := tileCanvasCopyTree(snapshot, project, true); err != nil {
			t.Fatal(err)
		}
		if err := tileCanvasInstrument(project, inputs); err != nil {
			t.Fatal(err)
		}
		projects[name] = project
	}
	report.RouteUpdates = len(inputs)
	rows := make(map[string][]tileCanvasRow)
	for _, variant := range []struct {
		name, source string
		canvas       bool
	}{
		{"tilecanvas-baseline", "celeste", true},
		{"tilecanvas-candidate", "celeste-parallax", true},
		{"mode4-candidate", "celeste-parallax", false},
	} {
		display := uint16(0x0404)
		if variant.canvas {
			display = 0x1f00
		}
		rows[variant.name] = runCelesteParallaxVariant(t, out, projects[variant.source], frozenRuntime,
			variant.name, variant.canvas, variant.canvas, display, len(inputs))
		report.Variants[variant.name] = celesteParallaxSummarize(t, out, variant.name, rows[variant.name], len(inputs))
	}
	baseline, candidate, oracle := rows["tilecanvas-baseline"], rows["tilecanvas-candidate"], rows["mode4-candidate"]
	compareCelesteParallaxPixels(t, out, "mode4-candidate", "tilecanvas-candidate", oracle, candidate)
	report.RendererParity = celesteParallaxExactRows(oracle, candidate)
	report.CrossSource = celesteParallaxCompareSources(t, baseline, candidate)
	if celesteParallaxFingerprint(t, frozenRuntime) != report.RuntimeSHA {
		t.Error("the frozen runtime changed during compilation")
	}
	for name, fingerprint := range report.Sources {
		if celesteParallaxFingerprint(t, filepath.Join(out, "snapshot", name)) != fingerprint {
			t.Errorf("the frozen %s source changed during compilation", name)
		}
	}
	t.Logf("cross-source: core watch mismatches=%d cosmetic watch differences=%d RGB-different frames=%d/%d; candidate Mode4/hybrid exact=%t",
		report.CrossSource.CoreWatchMismatchFrames, report.CrossSource.CosmeticWatchDifferenceFrames,
		report.CrossSource.RGBDifferentFrames, len(candidate), report.RendererParity)
}

type celesteParallaxComparisonReport struct {
	SchemaVersion  int                                      `json:"schemaVersion"`
	StartedUTC     string                                   `json:"startedUTC"`
	SourceRoot     string                                   `json:"sourceRoot"`
	CompilerRoot   string                                   `json:"compilerRoot"`
	GoVersion      string                                   `json:"goVersion"`
	TinyGoVersion  string                                   `json:"tinyGoVersion"`
	PlayerSHA      string                                   `json:"playerSHA256"`
	RuntimeSHA     string                                   `json:"runtimeSHA256"`
	Sources        map[string]string                        `json:"sourceSHA256"`
	RouteUpdates   int                                      `json:"routeUpdates"`
	Scope          string                                   `json:"scope"`
	Variants       map[string]celesteParallaxVariantSummary `json:"variants"`
	RendererParity bool                                     `json:"candidateMode4HybridExact"`
	CrossSource    celesteParallaxSourceComparison          `json:"crossSource"`
	Passed         bool                                     `json:"passed"`
}

type celesteParallaxCycleSummary struct {
	P50 uint32 `json:"p50"`
	P95 uint32 `json:"p95"`
	Max uint32 `json:"max"`
}

type celesteParallaxPhaseSummary struct {
	Samples       int                         `json:"samples"`
	Frame         celesteParallaxCycleSummary `json:"frameCycles"`
	Update        celesteParallaxCycleSummary `json:"updateCycles"`
	Draw          celesteParallaxCycleSummary `json:"drawCycles"`
	Commit        celesteParallaxCycleSummary `json:"prepareCommitCycles"`
	OverBudget    int                         `json:"updatesOver561792Cycles"`
	ClosedTicks   int                         `json:"closedTicks"`
	LCDScans      int                         `json:"lcdScans"`
	UpdatesPerSec float64                     `json:"updatesPerSecond"`
}

type celesteParallaxVariantSummary struct {
	ROMPath    string                                 `json:"romPath"`
	ROMSHA     string                                 `json:"romSHA256"`
	ROMBytes   int                                    `json:"romBytes"`
	IWRAMUsed  uint64                                 `json:"iwramUsed"`
	EWRAMUsed  uint64                                 `json:"ewramUsed"`
	EWRAMFree  uint64                                 `json:"ewramFree"`
	TracesSHA  string                                 `json:"tracesSHA256"`
	RGBSHA     []string                               `json:"rgbSHA256ByLogicalUpdate"`
	MaxObjects uint32                                 `json:"maxObjects"`
	Phases     map[string]celesteParallaxPhaseSummary `json:"phases"`
}

func celesteParallaxFingerprint(t *testing.T, root string) string {
	t.Helper()
	fingerprint, err := projectFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func celesteParallaxSummarize(t *testing.T, out, name string, rows []tileCanvasRow, routeLength int) celesteParallaxVariantSummary {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, name+"-build.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result build.Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	summary := celesteParallaxVariantSummary{
		ROMPath: result.ROMPath, ROMBytes: result.ROMSize, IWRAMUsed: result.IWRAMUsed,
		EWRAMUsed: result.EWRAMUsed, EWRAMFree: result.EWRAMFree,
		Phases: make(map[string]celesteParallaxPhaseSummary),
	}
	summary.ROMSHA, err = fileFingerprint(result.ROMPath)
	if err != nil {
		t.Fatal(err)
	}
	summary.TracesSHA, err = fileFingerprint(filepath.Join(out, name+"-traces.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		summary.RGBSHA = append(summary.RGBSHA, row.RGBASHA)
		if row.OBJCount > summary.MaxObjects {
			summary.MaxObjects = row.OBJCount
		}
	}
	cycles := func(values []uint32) celesteParallaxCycleSummary {
		return celesteParallaxCycleSummary{percentile(values, 50), percentile(values, 95), percentile(values, 100)}
	}
	for _, phase := range []string{"spawn", "route", "complete", "restart", "death", "respawn", "settled"} {
		var frame, update, draw, commit []uint32
		var metrics celesteParallaxPhaseSummary
		for index, row := range rows {
			if tileCanvasPhase(row, routeLength) != phase {
				continue
			}
			metrics.Samples++
			stats := row.Stats
			frame, update = append(frame, stats.LastFrame), append(update, stats.LastUpdate)
			draw, commit = append(draw, stats.LastDraw), append(commit, stats.LastBlit)
			if stats.LastFrame > twoFrameCycleBudget {
				metrics.OverBudget++
			}
			if index != len(rows)-1 {
				metrics.ClosedTicks++
				metrics.LCDScans += row.LCDSamples
			}
		}
		metrics.Frame, metrics.Update, metrics.Draw, metrics.Commit = cycles(frame), cycles(update), cycles(draw), cycles(commit)
		if metrics.LCDScans > 0 {
			metrics.UpdatesPerSec = (16777216.0 / (float64(twoFrameCycleBudget) / 2)) * float64(metrics.ClosedTicks) / float64(metrics.LCDScans)
		}
		summary.Phases[phase] = metrics
	}
	return summary
}

type celesteParallaxSourceComparison struct {
	CoreWatchMismatchFrames       int `json:"coreWatchMismatchFrames"`
	CosmeticWatchDifferenceFrames int `json:"cosmeticWatchDifferenceFrames"`
	RGBDifferentFrames            int `json:"rgbDifferentFrames"`
	DifferentPixels               int `json:"differentPixels"`
}

func celesteParallaxCompareSources(t *testing.T, before, after []tileCanvasRow) celesteParallaxSourceComparison {
	t.Helper()
	var comparison celesteParallaxSourceComparison
	if len(before) != len(after) {
		t.Fatalf("cross-source publication count: %d/%d", len(before), len(after))
	}
	// Only screen-shake offsets are excluded from core watch equality. They
	// consume RNG also used by removed cloud simulation, smoke and particles.
	// Physics, velocity, freeze, completion and lifecycle watches must match.
	core := func(row tileCanvasRow) []emulator.Watch {
		var watches []emulator.Watch
		for _, watch := range tileCanvasGameplayWatches(row.Watches) {
			if watch.Name != "p_shake_x" && watch.Name != "p_shake_y" {
				watches = append(watches, watch)
			}
		}
		return watches
	}
	for index, left := range before {
		right := after[index]
		if left.Tick != right.Tick || !reflect.DeepEqual(core(left), core(right)) {
			comparison.CoreWatchMismatchFrames++
			if comparison.CoreWatchMismatchFrames == 1 {
				t.Errorf("cross-source core gameplay first differs at tick %d: %+v / %+v", left.Tick, core(left), core(right))
			}
		}
		if left.Value("p_shake_x") != right.Value("p_shake_x") || left.Value("p_shake_y") != right.Value("p_shake_y") {
			comparison.CosmeticWatchDifferenceFrames++
		}
		if count := countDifferingPixels(left.rgba, right.rgba); count != 0 {
			comparison.RGBDifferentFrames++
			comparison.DifferentPixels += count
		}
	}
	return comparison
}

func celesteParallaxExactRows(before, after []tileCanvasRow) bool {
	if len(before) != len(after) {
		return false
	}
	for index, left := range before {
		right := after[index]
		if left.Tick != right.Tick || !reflect.DeepEqual(tileCanvasGameplayWatches(left.Watches), tileCanvasGameplayWatches(right.Watches)) ||
			countDifferingPixels(left.rgba, right.rgba) != 0 {
			return false
		}
	}
	return true
}
