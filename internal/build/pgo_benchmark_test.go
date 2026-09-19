package build

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/sourcebundle"
	"github.com/drpaneas/mtile/internal/transpiler"
	runtimebundle "github.com/drpaneas/mtile/runtime"
)

// Opt-in experiment only. The corpus is frozen before profiling; no TinyGo,
// workspace creation, output hashing, or benchmark bookkeeping is timed.
var pgoTrain = []string{"celeste", "multi-file", "audio-gb-song"}
var pgoHeldout = []string{"platformer", "pong", "mapdemo"}
var pgoControls = []string{"hello", "noalloc-call-stress"}

type pgoPrepared struct {
	project     assetsync.Project
	bundle      sourcebundle.Bundle
	sourceFiles []string
	globals     GlobalUsage
	assets      *assetsync.Assets
	generated   string
	renderer    transpiler.RendererReport
	audioBytes  int
}

// This is Build's unchanged Go prefix through linked-asset validation, using
// default CLI settings (project renderer, Debug=false, no optimization toggles).
// Keep the cold loading/validation and nil-Trace formatting work inside timing.
func pgoPrepare(projectDir string) (prepared pgoPrepared, err error) {
	s := Service{UseProjectRenderer: true}
	var result Result
	s.emit(PhaseSource, "Resolving project")
	projectDir, err = filepath.Abs(projectDir)
	if err != nil {
		return prepared, fmt.Errorf("resolve project directory: %w", err)
	}
	if info, err := os.Stat(projectDir); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		return prepared, fmt.Errorf("project directory: %w", err)
	}
	project, err := assetsync.LoadProject(projectDir)
	if err != nil {
		return prepared, err
	}
	s.configureTileCanvasExperiment(projectDir)
	renderer, err := s.resolveRenderer(project)
	if err != nil {
		return prepared, err
	}
	entry := project.Entry
	if entry == "" {
		entry = GameFileName
	}
	bundle, err := sourcebundle.Resolve(projectDir, entry)
	if err != nil {
		return prepared, fmt.Errorf("resolve Lua sources: %w", err)
	}
	s.emit(PhaseSource, fmt.Sprintf("Resolved %d Lua source file(s), entry %s", len(bundle.Files), bundle.Entry))
	sourceAnalysis, analysisErr := analyzeResolvedSources(bundle)
	result = Result{
		Entry: bundle.Entry, SourceFiles: append([]string(nil), bundle.Files...), GlobalUsage: sourceAnalysis.Globals,
	}
	s.emit(PhaseSource, fmt.Sprintf("Global state: %d/%d persistent fields", sourceAnalysis.Globals.Count, sourceAnalysis.Globals.Limit))
	if analysisErr != nil {
		return prepared, fmt.Errorf("transpile %s: %w", bundle.Entry, bundle.RemapError(analysisErr))
	}
	s.emit(PhaseAssets, "Loading and validating cartridge assets")
	assets, err := loadBuildAssets(projectDir)
	if err != nil {
		return prepared, fmt.Errorf("load assets: %w", err)
	}
	s.emit(PhaseTranspile, "Transpiling Lua to Go")
	generated, rendererReport, err := transpiler.TranspileWithReport(bundle.Entry, bundle.Source, transpiler.Options{
		PackageName:                        "main",
		IWRAMFunctions:                     project.IWRAMFunctions,
		Renderer:                           renderer,
		DisableStableTableFields:           s.DisableStableTableFields,
		DisableMonomorphicNumbers:          s.DisableMonomorphicNumbers,
		DisableMonomorphicFunctions:        s.DisableMonomorphicFunctions,
		DisableMonomorphicMultiReturns:     s.DisableMonomorphicMultiReturns,
		DisableNoAllocFunctionCalls:        s.DisableNoAllocFunctionCalls,
		DisableNumericArrays:               s.DisableNumericArrays,
		DisableHeapElision:                 s.DisableHeapElision,
		DisableNumericConstants:            s.DisableNumericConstants,
		DisableLiteralPrintSpecialization:  s.DisableLiteralPrintSpecialization,
		DisableFixedMapLayerSpecialization: s.DisableFixedMapLayerSpecialization,
		DisableMapRasterCache:              s.DisableMapRasterCache,
		DisableConditionSpecialization:     s.DisableConditionSpecialization,
		DisableStableWriteSpecialization:   s.DisableStableWriteSpecialization,
		DisableConstantDivision:            s.DisableConstantDivision,
		SheetW:                             assets.SheetW, SheetH: assets.SheetH,
		MapW: assets.MapW, MapH: assets.MapH, MapLayers: assets.MapLayers,
		Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM,
		Palette:     assets.PaletteROM[:],
		NativeAudio: assets.NativeAudio,
		Debug:       s.Debug,
	})
	if err != nil {
		return prepared, fmt.Errorf("transpile %s: %w", bundle.Entry, bundle.RemapError(err))
	}
	if rendererReport.Selected == transpiler.RendererMode0 && len(rendererReport.Mode0TextKeys) > 0 {
		runtimebundle.OverlayMode0TextSheet(assets.SheetROM, assets.SheetW, assets.SheetH, rendererReport.Mode0TextKeys)
	}
	if rendererReport.Diagnostic != "" {
		// AUTO fallback diagnostics are successful-build metadata, but need
		// the same original-module positions as rejected forced-mode builds.
		rendererReport.Diagnostic = bundle.RemapError(errors.New(rendererReport.Diagnostic)).Error()
	}
	audioROMBytes, err := validateLinkedAssetFootprintForPlanes(assets, rendererReport.MapPlaneMask, rendererReport.NativeAudioLinked)
	if err != nil {
		return prepared, err
	}
	if s.tileCanvasExperiment {
		s.emit(PhaseTranspile, "Renderer: EXPERIMENTAL tile canvas (up to four regular BGs plus a sparse OBJ foreground canvas)")
	} else {
		s.emit(PhaseTranspile, fmt.Sprintf("Renderer: requested %s -> selected %s (%s)", rendererReport.Requested, rendererReport.Selected, rendererReport.Reason))
	}
	if rendererReport.Profile != "" {
		s.emit(PhaseTranspile, "Renderer AUTO profile: "+rendererReport.Profile)
	}
	if rendererReport.Diagnostic != "" {
		s.emit(PhaseTranspile, "Renderer fallback: "+rendererReport.Diagnostic)
	}

	prepared = pgoPrepared{project: project, bundle: bundle, sourceFiles: result.SourceFiles, globals: result.GlobalUsage, assets: assets, generated: generated, renderer: rendererReport, audioBytes: audioROMBytes}
	return prepared, nil
}

func pgoRoot(tb testing.TB) string {
	tb.Helper()
	root := os.Getenv("MTILE_PGO_INPUT_ROOT")
	if root == "" {
		tb.Skip("set MTILE_PGO_INPUT_ROOT to the frozen experiment corpus")
	}
	if os.Getenv("MTILE_EXPERIMENTAL_TILECANVAS_PROJECT") != "" {
		tb.Fatal("PGO corpus requires ordinary project renderer settings")
	}
	return root
}

func pgoDigest(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

type pgoManifestRow struct {
	Project                     string
	SourceFiles                 []string
	SourceBytes, GeneratedBytes int
	Globals                     int
	Requested, Selected         string
	Hashes                      map[string]string
}

type pgoBuildBoundary struct{}

// Exercise the actual Build method outside timing and inspect its generated
// source after writeWorkspace, stopping before discovering or invoking TinyGo.
func pgoCheckActualBuild(t *testing.T, project string, want pgoPrepared) {
	t.Helper()
	workspaceRoot := t.TempDir()
	t.Setenv("TMPDIR", workspaceRoot)
	var events []Event
	runtimeDir, err := filepath.Abs(filepath.Join("..", "..", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	service := Service{UseProjectRenderer: true, KeepWork: true, RuntimeDir: runtimeDir}
	service.Trace = func(e Event) {
		events = append(events, e)
		if e.Phase == PhaseToolchain {
			panic(pgoBuildBoundary{})
		}
	}
	reached := false
	func() {
		defer func() {
			if p := recover(); p != nil {
				if _, ok := p.(pgoBuildBoundary); ok {
					reached = true
				} else {
					panic(p)
				}
			}
		}()
		if _, err := service.Build(project, "unused.gba"); err != nil {
			t.Fatal(err)
		}
	}()
	if !reached {
		t.Fatal("actual Build did not reach the pre-TinyGo boundary")
	}
	paths, err := filepath.Glob(filepath.Join(workspaceRoot, "mtile-build-*", "main.go"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("generated workspace: %v %v", paths, err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want.generated {
		t.Fatal("helper differs from actual Build generated source")
	}
	required := []string{
		fmt.Sprintf("Resolved %d Lua source file(s), entry %s", len(want.bundle.Files), want.bundle.Entry),
		fmt.Sprintf("Global state: %d/%d persistent fields", want.globals.Count, want.globals.Limit),
		fmt.Sprintf("Renderer: requested %s -> selected %s (%s)", want.renderer.Requested, want.renderer.Selected, want.renderer.Reason),
	}
	if want.renderer.Profile != "" {
		required = append(required, "Renderer AUTO profile: "+want.renderer.Profile)
	}
	if want.renderer.Diagnostic != "" {
		required = append(required, "Renderer fallback: "+want.renderer.Diagnostic)
	}
	for _, message := range required {
		found := false
		for _, event := range events {
			found = found || event.Message == message
		}
		if !found {
			t.Fatalf("actual Build omitted expected metadata %q", message)
		}
	}
}

func TestPGOPreparationManifest(t *testing.T) {
	root := pgoRoot(t)
	t.Logf("GOMAXPROCS=%d NumCPU=%d", runtime.GOMAXPROCS(0), runtime.NumCPU())
	var rows []pgoManifestRow
	names := append(append(append([]string{}, pgoTrain...), pgoHeldout...), pgoControls...)
	for _, name := range names {
		prepared, err := pgoPrepare(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if os.Getenv("MTILE_PGO_VERIFY_BUILD") == "1" {
			pgoCheckActualBuild(t, filepath.Join(root, name), prepared)
		}
		// Independent reload verifies complete flattened assets, including authored
		// audio that a particular generated game may not link.
		assets, err := loadBuildAssets(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if prepared.renderer.Selected == transpiler.RendererMode0 && len(prepared.renderer.Mode0TextKeys) > 0 {
			runtimebundle.OverlayMode0TextSheet(assets.SheetROM, assets.SheetW, assets.SheetH, prepared.renderer.Mode0TextKeys)
		}
		if !reflect.DeepEqual(assets, prepared.assets) {
			t.Fatal("asset reload differs")
		}
		rows = append(rows, pgoManifestRow{Project: name, SourceFiles: prepared.sourceFiles, SourceBytes: len(prepared.bundle.Source), GeneratedBytes: len(prepared.generated), Globals: prepared.globals.Count, Requested: prepared.renderer.Requested.String(), Selected: prepared.renderer.Selected.String(), Hashes: map[string]string{
			"generatedGo": pgoDigest(prepared.generated), "assets": pgoDigest(prepared.assets), "project": pgoDigest(prepared.project), "sources": pgoDigest(prepared.bundle), "globals": pgoDigest(prepared.globals), "renderer": pgoDigest(prepared.renderer), "audioBytes": pgoDigest(prepared.audioBytes),
		}})
	}
	path := os.Getenv("MTILE_PGO_MANIFEST")
	if path == "" {
		t.Fatal("set MTILE_PGO_MANIFEST for output equality evidence")
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

var pgoSink pgoPrepared

// One CPU profile, exactly one thirty-second loop over only the training set.
// All setup and profile file handling are excluded from timed evaluation.
func TestPGOTraining(t *testing.T) {
	root := pgoRoot(t)
	path := os.Getenv("MTILE_PGO_PROFILE")
	if path == "" {
		t.Skip("set MTILE_PGO_PROFILE to collect the one training profile")
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := pprof.StartCPUProfile(file); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()
	deadline := time.Now().Add(30 * time.Second)
	rounds := 0
	for time.Now().Before(deadline) {
		for _, name := range pgoTrain {
			pgoSink, err = pgoPrepare(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
		}
		rounds++
	}
	t.Logf("training rounds=%d projects=%s", rounds, strings.Join(pgoTrain, ","))
}

func benchmarkPGOCorpus(b *testing.B, names []string) {
	root := pgoRoot(b)
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(root, name)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, path := range paths {
			var err error
			pgoSink, err = pgoPrepare(path)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
func BenchmarkPGOHeldout(b *testing.B) { benchmarkPGOCorpus(b, pgoHeldout) }
func BenchmarkPGOHello(b *testing.B)   { benchmarkPGOCorpus(b, pgoControls[:1]) }
func BenchmarkPGONoalloc(b *testing.B) { benchmarkPGOCorpus(b, pgoControls[1:]) }
