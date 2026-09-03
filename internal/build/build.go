// Package build implements the reproducible native GBA build pipeline.
// Assets are flattened and validated before a generated Go package is linked
// in a temporary module, so failed builds never leave partial generated code
// in a project directory.
package build

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/sourcebundle"
	"github.com/drpaneas/mtile/internal/transpiler"
	runtimebundle "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
)

const GameFileName = "game.lua"

// Service owns toolchain paths and build policy. Keeping these injectable
// makes CLI behavior testable and lets users select a pinned TinyGo binary.
type Service struct {
	TinyGoBin  string
	RuntimeDir string
	KeepWork   bool
	Trace      func(Event)
	Debug      bool
	Renderer   transpiler.Renderer
	// DisableStableTableFields is the reversible legacy path used for semantic
	// comparison and performance A/B measurements.
	DisableStableTableFields bool
	// DisableMonomorphicNumbers retains tagged rt.Value numeric storage and is
	// the rollback/A-B path for the first-observed-number optimization.
	DisableMonomorphicNumbers bool
	// DisableMonomorphicFunctions retains the tagged user-function ABI while
	// leaving all other monomorphic-number optimizations enabled.
	DisableMonomorphicFunctions bool
	// DisableMonomorphicMultiReturns retains tagged multi-return results while
	// keeping parameter and single-return specialization enabled.
	DisableMonomorphicMultiReturns bool
	// DisableNoAllocFunctionCalls retains root-protected user calls even when
	// static analysis proves that the complete callee chain cannot allocate.
	DisableNoAllocFunctionCalls bool
	// DisableNumericArrays retains generic table storage for fixed numeric
	// arrays and provides the benchmark/rollback path for compact storage.
	DisableNumericArrays bool
	// DisableHeapElision retains collection and target semispaces for A/B tests.
	DisableHeapElision bool
	// DisableNumericConstants retains storage for immutable numeric bindings.
	DisableNumericConstants bool
	// DisableLiteralPrintSpecialization retains the generic allocating-capable
	// print path for literal-string A/B tests.
	DisableLiteralPrintSpecialization bool
	// DisableReadOnlySheetSpecialization retains the generic Mode 4 sheet
	// representation or the Mode 0 dirty-upload scan for paired benchmarks.
	DisableReadOnlySheetSpecialization bool
	// DisableFixedMapLayerSpecialization retains logical-to-storage layer lookup
	// for compile-time layers in compact-map A/B benchmarks.
	DisableFixedMapLayerSpecialization bool
	// DisableStaticStringSpecialization retains all 256 Context static slots.
	DisableStaticStringSpecialization bool
	// UseProjectRenderer reads project.json's renderer policy. Renderer remains
	// an explicit programmatic override for renderbench and build tests.
	UseProjectRenderer bool
}

type Phase string

const (
	PhaseSource    Phase = "source"
	PhaseAssets    Phase = "assets"
	PhaseTranspile Phase = "transpile"
	PhaseWorkspace Phase = "workspace"
	PhaseToolchain Phase = "toolchain"
	PhaseLinkELF   Phase = "link-elf"
	PhaseMemory    Phase = "memory"
	PhaseLinkROM   Phase = "link-rom"
	PhaseValidate  Phase = "validate"
	PhaseWrite     Phase = "write"
	PhaseComplete  Phase = "complete"
)

// Event is a truthful, ordered build-pipeline update.
type Event struct {
	Phase   Phase
	Message string
}

type Result struct {
	ROMPath            string
	ROMSize            int
	MapPath            string
	EWRAMUsed          uint64
	EWRAMFree          uint64
	IWRAMUsed          uint64
	HeapLinked         bool
	WorkDir            string
	AudioROMBytes      int
	AudioVoices        int
	AudioMasterVolume  int
	AudioProfile       string
	AudioPSGRatio      int
	Entry              string
	SourceFiles        []string
	GlobalUsage        GlobalUsage
	GeneratedGo        string
	RendererRequested  transpiler.Renderer
	Renderer           transpiler.Renderer
	RendererReason     string
	RendererProfile    string
	RendererDiagnostic string
	RendererLayers     int
	RendererObjects    bool
	RendererText       bool
	RendererOAMEntries int
	RendererLineCycles int
}

func (s Service) emit(phase Phase, message string) {
	if s.Trace != nil {
		s.Trace(Event{Phase: phase, Message: message})
	}
}

func NewService(tinygoBin, runtimeDir string) (Service, error) {
	if tinygoBin == "" {
		var err error
		tinygoBin, err = resolveTinyGo()
		if err != nil {
			return Service{}, err
		}
	}
	if runtimeDir == "" {
		var err error
		runtimeDir, err = defaultRuntimeDir()
		if err != nil {
			return Service{}, err
		}
	}
	tinygoBin, err := filepath.Abs(tinygoBin)
	if err != nil {
		return Service{}, err
	}
	runtimeDir, err = filepath.Abs(runtimeDir)
	if err != nil {
		return Service{}, err
	}
	for _, name := range []string{"gba.s", "gba.ld"} {
		path := filepath.Join(runtimeDir, "crt0", name)
		info, statErr := os.Stat(path)
		if statErr != nil {
			return Service{}, fmt.Errorf("runtime crt0 input %q is unavailable: %w", name, statErr)
		}
		if info.IsDir() {
			return Service{}, fmt.Errorf("runtime crt0 input %q is a directory", name)
		}
	}
	return Service{TinyGoBin: tinygoBin, RuntimeDir: runtimeDir}, nil
}

func defaultRuntimeDir() (string, error) {
	_, source, _, _ := runtime.Caller(0)
	sourceRuntime := filepath.Join(filepath.Dir(source), "..", "..", "runtime")
	if info, err := os.Stat(filepath.Join(sourceRuntime, "crt0", "gba.ld")); err == nil && !info.IsDir() {
		return sourceRuntime, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate runtime cache: %w", err)
	}
	destination := filepath.Join(cache, "mtile", "runtime", "v2")
	if err := runtimebundle.MaterializeSource(destination); err != nil {
		return "", err
	}
	return destination, nil
}

func (s Service) Build(projectDir, outputPath string) (result Result, err error) {
	s.emit(PhaseSource, "Resolving project")
	projectDir, err = filepath.Abs(projectDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve project directory: %w", err)
	}
	if info, err := os.Stat(projectDir); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		return Result{}, fmt.Errorf("project directory: %w", err)
	}
	project, err := assetsync.LoadProject(projectDir)
	if err != nil {
		return Result{}, err
	}
	renderer, err := s.resolveRenderer(project)
	if err != nil {
		return Result{}, err
	}
	entry := project.Entry
	if entry == "" {
		entry = GameFileName
	}
	bundle, err := sourcebundle.Resolve(projectDir, entry)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Lua sources: %w", err)
	}
	s.emit(PhaseSource, fmt.Sprintf("Resolved %d Lua source file(s), entry %s", len(bundle.Files), bundle.Entry))
	sourceAnalysis, analysisErr := analyzeResolvedSources(bundle)
	result = Result{
		Entry: bundle.Entry, SourceFiles: append([]string(nil), bundle.Files...), GlobalUsage: sourceAnalysis.Globals,
	}
	s.emit(PhaseSource, fmt.Sprintf("Global state: %d/%d persistent fields", sourceAnalysis.Globals.Count, sourceAnalysis.Globals.Limit))
	if analysisErr != nil {
		return result, fmt.Errorf("transpile %s: %w", bundle.Entry, bundle.RemapError(analysisErr))
	}
	s.emit(PhaseAssets, "Loading and validating cartridge assets")
	assets, err := loadBuildAssets(projectDir)
	if err != nil {
		return result, fmt.Errorf("load assets: %w", err)
	}
	s.emit(PhaseTranspile, "Transpiling Lua to Go")
	generated, rendererReport, err := transpiler.TranspileWithReport(bundle.Entry, bundle.Source, transpiler.Options{
		PackageName:                        "main",
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
		SheetW:                             assets.SheetW, SheetH: assets.SheetH,
		MapW: assets.MapW, MapH: assets.MapH, MapLayers: assets.MapLayers,
		Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM,
		Palette:              assets.PaletteROM[:],
		SFX:                  assets.SFXROM,
		SFXStep:              assets.SFXStepROM,
		Music:                assets.MusicROM,
		Samples:              assets.SampleROM,
		SamplePCM:            assets.SamplePCMROM,
		AudioMasterVolume:    uint16(assets.AudioSettings.MasterVolume),
		AudioSettingsPresent: assets.AudioVersion == assetsync.AudioSettingsSchemaVersion,
		NativeAudio:          assets.NativeAudio,
		Debug:                s.Debug,
	})
	if err != nil {
		return result, fmt.Errorf("transpile %s: %w", bundle.Entry, bundle.RemapError(err))
	}
	audioROMBytes, err := validateLinkedAssetFootprintForPlanes(assets, rendererReport.MapPlaneMask, rendererReport.NativeAudioLinked)
	if err != nil {
		return result, err
	}
	s.emit(PhaseTranspile, fmt.Sprintf("Renderer: requested %s -> selected %s (%s)", rendererReport.Requested, rendererReport.Selected, rendererReport.Reason))
	if rendererReport.Profile != "" {
		s.emit(PhaseTranspile, "Renderer AUTO profile: "+rendererReport.Profile)
	}
	if rendererReport.Diagnostic != "" {
		s.emit(PhaseTranspile, "Renderer fallback: "+rendererReport.Diagnostic)
	}

	s.emit(PhaseWorkspace, "Creating isolated build workspace")
	workDir, err := os.MkdirTemp("", "mtile-build-*")
	if err != nil {
		return result, fmt.Errorf("create build workspace: %w", err)
	}
	if !s.KeepWork {
		defer func() { _ = os.RemoveAll(workDir) }()
	} else {
		defer func() {
			if err != nil {
				err = fmt.Errorf("%w (temporary build workspace kept at %s)", err, workDir)
			}
		}()
	}
	if err := s.writeWorkspace(workDir, generated); err != nil {
		return result, err
	}
	s.emit(PhaseToolchain, "Locating TinyGo toolchain")
	tinygoRoot, err := discoverTinyGoRoot(s.TinyGoBin)
	if err != nil {
		return result, err
	}
	targetPath := filepath.Join(workDir, "target-gba.json")
	if err := GenerateTarget(tinygoRoot, s.RuntimeDir, targetPath); err != nil {
		return result, err
	}

	mapPath := filepath.Join(workDir, "game.map")
	elfPath := filepath.Join(workDir, "game.elf")
	linkReport := rendererReport
	if s.DisableReadOnlySheetSpecialization {
		linkReport.SheetReadOnly = false
	}
	if s.DisableStaticStringSpecialization {
		linkReport.StaticStrings = runtimebundle.MaxStaticStrings
	}
	s.emit(PhaseLinkELF, "Linking analysis ELF and memory map")
	if err := s.runTinyGo(workDir, targetPath, elfPath, mapPath, linkReport); err != nil {
		return result, fmt.Errorf("link ELF: %w", err)
	}
	symbols, err := ParseLinkerMap(mapPath)
	if err != nil {
		return result, err
	}
	if err := ValidateHeapBudget(symbols); err != nil {
		return result, err
	}
	ewramUsed := symbols.EWRAMEnd - gbaEWRAMOrigin
	ewramFree := symbols.HeapEnd - symbols.EWRAMEnd
	iwramUsed := uint64(0)
	if symbols.IWRAMEnd >= gbaIWRAMOrigin && symbols.IWRAMEnd <= gbaIWRAMEnd {
		iwramUsed = symbols.IWRAMEnd - gbaIWRAMOrigin
	}
	s.emit(PhaseMemory, fmt.Sprintf("Memory: EWRAM %d/%d bytes, %d free; IWRAM %d/%d bytes", ewramUsed, EWRAMCapacity, ewramFree, iwramUsed, IWRAMCapacity))
	romPath := filepath.Join(workDir, "game.gba")
	s.emit(PhaseLinkROM, "Linking GBA ROM")
	if err := s.runTinyGo(workDir, targetPath, romPath, "", linkReport); err != nil {
		return result, fmt.Errorf("link ROM: %w", err)
	}
	rom, err := os.ReadFile(romPath)
	if err != nil {
		return result, fmt.Errorf("read linked ROM: %w", err)
	}
	if err := ValidateROM(rom); err != nil {
		return result, err
	}
	s.emit(PhaseValidate, fmt.Sprintf("Validated %d-byte GBA cartridge", len(rom)))
	if outputPath == "" {
		outputPath = filepath.Join(projectDir, "game.gba")
	} else if outputPath, err = filepath.Abs(outputPath); err != nil {
		return result, fmt.Errorf("resolve output path: %w", err)
	}
	if err := atomicWrite(outputPath, rom, 0o644); err != nil {
		return result, fmt.Errorf("write ROM: %w", err)
	}
	s.emit(PhaseWrite, "Wrote "+outputPath)
	audioVoices := 0
	audioProfile := ""
	audioPSGRatio := 0
	audioMasterVolume := assets.AudioSettings.MasterVolume
	if len(assets.SFXROM) != 0 || len(assets.MusicROM) != 0 || len(assets.SampleROM) != 0 {
		audioVoices = audio.SFXMixerVoices
		if len(assets.SampleROM) != 0 {
			audioVoices = audio.SFXSampleMixerVoices
		}
	}
	if rendererReport.NativeAudioLinked {
		audioVoices = 4
		audioProfile = assets.NativeAudio.Settings.Profile
		audioPSGRatio = assets.NativeAudio.Settings.PSGRatio
		audioMasterVolume = 0
	}
	result = Result{
		ROMPath: outputPath, ROMSize: len(rom),
		EWRAMUsed: ewramUsed, EWRAMFree: ewramFree, IWRAMUsed: iwramUsed,
		HeapLinked:    rendererReport.HeapLinked,
		AudioROMBytes: audioROMBytes, AudioVoices: audioVoices, AudioMasterVolume: audioMasterVolume,
		AudioProfile: audioProfile, AudioPSGRatio: audioPSGRatio,
		Entry: bundle.Entry, SourceFiles: append([]string(nil), bundle.Files...), GlobalUsage: sourceAnalysis.Globals, GeneratedGo: generated,
		RendererRequested: rendererReport.Requested,
		Renderer:          rendererReport.Selected, RendererReason: rendererReport.Reason,
		RendererProfile: rendererReport.Profile, RendererDiagnostic: rendererReport.Diagnostic,
		RendererLayers: rendererReport.Mode0Layers, RendererObjects: rendererReport.Mode0Objects, RendererText: rendererReport.Mode0Text,
		RendererOAMEntries: rendererReport.Mode0OAMEntries, RendererLineCycles: rendererReport.Mode0OBJLineCycles,
	}
	if s.KeepWork {
		result.MapPath = mapPath
		result.WorkDir = workDir
	}
	s.emit(PhaseComplete, "Build completed successfully")
	return result, nil
}

func (s Service) resolveRenderer(project assetsync.Project) (transpiler.Renderer, error) {
	if !s.UseProjectRenderer {
		return s.Renderer, nil
	}
	renderer, err := transpiler.ParseRenderer(project.Renderer)
	if err != nil {
		return transpiler.RendererMode4, fmt.Errorf("project renderer: %w", err)
	}
	return renderer, nil
}

func loadBuildAssets(projectDir string) (*assetsync.Assets, error) {
	sheet, err := assetsync.LoadSpritesheet(projectDir)
	if err != nil {
		return nil, err
	}
	mapData, err := assetsync.LoadMap(projectDir)
	if err != nil {
		return nil, err
	}
	if err := validateBuildAssetFootprint(*sheet, *mapData); err != nil {
		return nil, err
	}
	version, err := assetsync.DetectAudioVersion(projectDir)
	if err != nil {
		return nil, err
	}
	if version != assetsync.NativeAudioSchemaVersion {
		return nil, fmt.Errorf("audio version %d is superseded; convert explicitly to four-channel Game Boy audio v2", version)
	}
	return assetsync.Load(projectDir)
}

func validateBuildAssetFootprint(sheet assetsync.Spritesheet, mapData assetsync.Map) error {
	sheetBytes, err := checkedAssetProduct(sheet.Width, sheet.Height, "spritesheet")
	if err != nil {
		return err
	}
	spriteN, err := checkedAssetProduct(sheet.Width/8, sheet.Height/8, "sprite grid")
	if err != nil {
		return err
	}
	mapCells, err := checkedAssetProduct(mapData.Width, mapData.Height, "map")
	if err != nil {
		return err
	}
	if layers := uint64(mapData.LayerCount()); layers != 0 {
		if mapCells > math.MaxUint32/layers {
			return fmt.Errorf("map layers exceed the uint32 asset limit")
		}
		mapCells *= layers
	}
	assetBytes := sheetBytes
	for _, item := range []struct {
		name  string
		bytes uint64
	}{
		{name: "map", bytes: mapCells * 2},
		{name: "flags", bytes: spriteN},
		{name: "palette", bytes: 256 * 2},
	} {
		var sumErr error
		assetBytes, sumErr = addAssetBytes(assetBytes, item.bytes, item.name)
		if sumErr != nil {
			return sumErr
		}
	}
	if assetBytes > uint64(maxGBAROMSize-gbaHeaderSize) {
		return fmt.Errorf("asset payload is %d bytes, exceeds the 32 MiB GBA cartridge capacity", assetBytes)
	}
	return nil
}

func validateCompiledAssetFootprint(assets *assetsync.Assets) (int, error) {
	if assets == nil {
		return 0, errors.New("compiled assets are required")
	}
	layers := assets.MapLayers
	if layers == 0 {
		layers = 1
	}
	return validateCompiledAssetFootprintForPlanes(assets, uint8((1<<uint(layers))-1))
}

func validateCompiledAssetFootprintForPlanes(assets *assetsync.Assets, mapPlaneMask uint8) (int, error) {
	return validateLinkedAssetFootprintForPlanes(assets, mapPlaneMask, true)
}

func validateLinkedAssetFootprintForPlanes(assets *assetsync.Assets, mapPlaneMask uint8, includeNativeAudio bool) (int, error) {
	if assets == nil {
		return 0, errors.New("compiled assets are required")
	}
	layers := assets.MapLayers
	if layers == 0 {
		layers = 1
	}
	validMask := uint8((1 << uint(layers)) - 1)
	if mapPlaneMask&^validMask != 0 {
		return 0, fmt.Errorf("map plane mask %#02x exceeds %d logical layers", mapPlaneMask, layers)
	}
	audioBytes := uint64(assetsync.CompiledAudioROMBytes(
		assets.SFXROM, assets.SFXStepROM, assets.MusicROM, assets.SampleROM, assets.SamplePCMROM,
	))
	if includeNativeAudio && assets.NativeAudio != nil {
		audioBytes += uint64(assetsync.CompiledNativeAudioROMBytes(assets.NativeAudio.Library))
	}
	mapCells := uint64(assets.MapW * assets.MapH * mapPlanePopulation(mapPlaneMask))
	total := uint64(len(assets.SheetROM)) + mapCells*2 +
		uint64(len(assets.FlagROM)) + uint64(len(assets.PaletteROM))*2
	var err error
	total, err = addAssetBytes(total, audioBytes, "audio")
	if err != nil {
		return 0, err
	}
	if total > uint64(maxGBAROMSize-gbaHeaderSize) {
		return 0, fmt.Errorf("asset payload is %d bytes, exceeds the 32 MiB GBA cartridge capacity", total)
	}
	if audioBytes > uint64(math.MaxInt) {
		return 0, errors.New("audio asset payload exceeds host int range")
	}
	return int(audioBytes), nil
}

func mapPlanePopulation(mask uint8) int {
	n := 0
	for mask != 0 {
		n += int(mask & 1)
		mask >>= 1
	}
	return n
}

func addAssetBytes(total, amount uint64, name string) (uint64, error) {
	if math.MaxUint64-total < amount {
		return 0, fmt.Errorf("%s asset payload size overflows uint64", name)
	}
	return total + amount, nil
}

func checkedAssetProduct(a, b int, name string) (uint64, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("%s dimensions must be nonnegative, got %dx%d", name, a, b)
	}
	ua, ub := uint64(a), uint64(b)
	if ua != 0 && ub > math.MaxUint32/ua {
		return 0, fmt.Errorf("%s dimensions %dx%d exceed the uint32 asset limit", name, a, b)
	}
	return ua * ub, nil
}

func (s Service) writeWorkspace(workDir string, generated string) error {
	module := "module mtile-generated\n\ngo 1.22\n\nrequire github.com/drpaneas/mtile/runtime v0.0.0\n\nreplace github.com/drpaneas/mtile/runtime => " + filepath.ToSlash(s.RuntimeDir) + "\n"
	if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte(module), 0o644); err != nil {
		return fmt.Errorf("write generated go.mod: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte(generated), 0o644); err != nil {
		return fmt.Errorf("write generated main.go: %w", err)
	}
	return nil
}

func (s Service) runTinyGo(workDir, targetPath, outputPath, mapPath string, report transpiler.RendererReport) error {
	// TinyGo defaults to -opt=z (size). These carts are far under the 32 MB
	// ROM cap; -opt=2 keeps the per-VBlank _update/_draw/cls path in the
	// speed-optimized LLVM pipeline instead of the size one.
	args := []string{"build", "-gc=none", "-opt=2", "-target=" + targetPath}
	if tags := runtimeBuildTags(report); tags != "" {
		args = append(args, "-tags="+tags)
	}
	if mapPath != "" {
		args = append(args, "-ldflags=-extldflags=-Map="+mapPath)
	}
	args = append(args, "-o", outputPath, ".")
	cmd := exec.Command(s.TinyGoBin, args...)
	cmd.Dir = workDir
	cmd.Env = setEnv(os.Environ(), "GOWORK", "off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text != "" {
			return fmt.Errorf("%w: %s", err, text)
		}
		return err
	}
	return nil
}

func runtimeBuildTags(report transpiler.RendererReport) string {
	tags := ""
	if report.Selected == transpiler.RendererMode0 {
		mapTag := "mtile_map_compact"
		if report.MapStreaming {
			// Target measurements show that constant streaming removes material
			// Mode 0 commit work. The same tag regresses the Mode 4 draw loop,
			// so Mode 4 deliberately retains its generic streaming path below.
			mapTag = "mtile_map_streaming"
		}
		tags = mode0BuildTags(report) + "," + mapTag
	} else if !report.MapStreaming {
		tags = "mtile_map_compact"
	}
	if !report.HeapLinked {
		if tags != "" {
			tags += ","
		}
		tags += "mtile_heapless"
	}
	if report.SheetReadOnly {
		if tags != "" {
			tags += ","
		}
		if report.Selected == transpiler.RendererMode0 {
			// The full immutable-sheet representation regressed Mode 0's measured
			// draw path. This narrower tag preserves the generic representation while
			// deleting the impossible dirty-tile scan from every frame.
			tags += "mtile_mode0_sheet_immutable"
		} else {
			tags += "mtile_sheet_readonly"
		}
	}
	staticTag := ""
	switch {
	case report.StaticStrings <= 1:
		staticTag = "mtile_static_strings_1"
	case report.StaticStrings <= 8:
		staticTag = "mtile_static_strings_8"
	case report.StaticStrings <= 32:
		staticTag = "mtile_static_strings_32"
	case report.StaticStrings <= 128:
		staticTag = "mtile_static_strings_128"
	}
	if staticTag != "" {
		if tags != "" {
			tags += ","
		}
		tags += staticTag
	}
	return tags
}

func mode0BuildTags(report transpiler.RendererReport) string {
	tags := "mtile_mode0"
	if report.Mode0Layers != 1 {
		tags += fmt.Sprintf(",mtile_mode0_layers%d", report.Mode0Layers)
	}
	// Text and packing implementations embed the ordinary OBJ state. Keep the
	// dependency explicit here as a defense against an inconsistent report.
	if report.Mode0Objects || report.Mode0ObjectPacking || report.Mode0Text {
		tags += ",mtile_mode0_obj"
	}
	if report.Mode0ObjectPacking {
		tags += ",mtile_mode0_objpack"
	}
	if report.Mode0Text {
		tags += ",mtile_mode0_text"
	}
	return tags
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				result = append(result, prefix+value)
				replaced = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !replaced {
		result = append(result, prefix+value)
	}
	return result
}
