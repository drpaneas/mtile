package build

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/lint"
	"github.com/drpaneas/mtile/internal/luaparser"
	"github.com/drpaneas/mtile/internal/sourcebundle"
	"github.com/drpaneas/mtile/internal/transpiler"
)

type DiagnosticSeverity int

const (
	DiagnosticError       DiagnosticSeverity = 1
	DiagnosticWarning     DiagnosticSeverity = 2
	DiagnosticInformation DiagnosticSeverity = 3
)

// Diagnostic uses project-relative sources and one-based byte columns, just
// like the parser. End positions are exclusive. Clients convert these to their
// own coordinate system; compiler messages and renderer codes remain intact.
type Diagnostic struct {
	Source                           string
	Line, Column, EndLine, EndColumn int
	Severity                         DiagnosticSeverity
	Code, Message                    string
}

type ProjectAnalysis struct {
	SourceAnalysis
	Diagnostics []Diagnostic
	Renderer    transpiler.RendererReport
	// RendererReady is false after any source, asset, or transpiler error.
	// A partial report must never be presented as a proven selection.
	RendererReady bool
	// Lint is source-only advice, kept separate from build/renderer errors.
	// It may be ready even when an asset or semantic check later fails.
	Lint      []lint.Diagnostic
	LintReady bool
}

// CheckProject runs the build's front end using unsaved Lua buffers. It loads
// real project assets and runs code generation for semantic validation, but
// never writes generated files, discovers TinyGo, or invokes the linker.
func (s Service) CheckProject(projectDir string, overlay map[string]string) ProjectAnalysis {
	result := ProjectAnalysis{}
	entry := GameFileName
	fail := func(err error, code string) ProjectAnalysis {
		result.Diagnostics = append(result.Diagnostics, projectDiagnostic(projectDir, overlay, entry, err, DiagnosticError, code))
		return result
	}
	root, err := filepath.Abs(projectDir)
	if err != nil {
		return fail(err, "PROJECT")
	}
	projectDir = root
	project, err := assetsync.LoadProject(root)
	if err != nil {
		entry = "project.json"
		return fail(err, "PROJECT")
	}
	entry, err = s.resolveEntry(project)
	if err != nil {
		entry = "project.json"
		return fail(err, "SOURCE")
	}
	s.configureTileCanvasExperiment(root)
	renderer, err := s.resolveRenderer(project)
	if err != nil {
		return fail(err, "PROJECT")
	}
	bundle, err := sourcebundle.ResolveWithOverlay(root, entry, overlay)
	if err != nil {
		return fail(err, "SOURCE")
	}
	result.Lint, err = lint.Analyze(bundle)
	if err != nil {
		return fail(err, "SOURCE")
	}
	result.LintReady = true
	result.SourceAnalysis, err = analyzeResolvedSources(bundle)
	if err != nil {
		return fail(bundle.RemapError(err), "TRANSPILE")
	}
	assets, err := loadBuildAssets(root)
	if err != nil {
		return fail(err, "ASSET")
	}
	if s.tileCanvasExperiment && len(assets.TileLinks) != 0 {
		return fail(errors.New("linked tiles (assets/tilelinks.json) are not supported by -experimental-tilecanvas; build with mode4 or mode0"), "ASSET")
	}
	_, report, err := transpiler.TranspileWithReport(bundle.Entry, bundle.Source, s.transpilerOptions(project, assets, renderer))
	if err != nil {
		return fail(bundle.RemapError(err), "TRANSPILE")
	}
	if _, err := validateLinkedAssetFootprintForPlanes(assets, report.MapPlaneMask, report.NativeAudioLinked); err != nil {
		return fail(err, "ASSET")
	}
	if report.Diagnostic != "" {
		report.Diagnostic = bundle.RemapError(errors.New(report.Diagnostic)).Error()
		result.Diagnostics = append(result.Diagnostics, projectDiagnostic(root, overlay, entry, errors.New(report.Diagnostic), DiagnosticInformation, report.Reason))
	}
	for i, warning := range report.Warnings {
		report.Warnings[i] = bundle.RemapError(errors.New(warning)).Error()
		result.Diagnostics = append(result.Diagnostics, projectDiagnostic(root, overlay, entry, errors.New(report.Warnings[i]), DiagnosticWarning, "ASSET_WARNING"))
	}
	result.Renderer, result.RendererReady = report, true
	return result
}

var diagnosticLocation = regexp.MustCompile(`((?:[A-Za-z]:[\\/])?[^\r\n:]+\.lua):(\d+)(?::(\d+))?`)
var diagnosticCode = regexp.MustCompile(`\[([A-Z][A-Z0-9_]+)\]`)

// The existing compiler exposes semantic errors as strings. Adapt those in
// one place, while retaining structured parser positions whenever available.
func projectDiagnostic(root string, overlay map[string]string, fallback string, err error, severity DiagnosticSeverity, code string) Diagnostic {
	d := Diagnostic{Source: fallback, Line: 1, Column: 1, Severity: severity, Code: code, Message: err.Error()}
	var parseError *luaparser.Error
	if errors.As(err, &parseError) {
		d.Source, d.Line, d.Column = parseError.Pos.Source, parseError.Pos.Line, parseError.Pos.Column
		d.Code = "LUA_SYNTAX"
	} else if match := diagnosticLocation.FindStringSubmatch(d.Message); match != nil {
		d.Source = strings.TrimSpace(match[1])
		d.Line, _ = strconv.Atoi(match[2])
		if match[3] != "" {
			d.Column, _ = strconv.Atoi(match[3])
		}
	}
	if filepath.IsAbs(d.Source) {
		if relative, err := filepath.Rel(root, d.Source); err == nil {
			d.Source = relative
		}
	}
	d.Source = filepath.ToSlash(d.Source)
	if match := diagnosticCode.FindStringSubmatch(d.Message); match != nil {
		d.Code = match[1]
	}
	text, ok := overlay[d.Source]
	if !ok {
		// Resolver errors can name entries that failed its symlink boundary.
		// Keep diagnostic range lookup inside the project as well.
		if dir, err := os.OpenRoot(root); err == nil {
			data, _ := dir.ReadFile(filepath.FromSlash(d.Source))
			_ = dir.Close()
			text = string(data)
		}
	}
	d.EndLine, d.EndColumn = d.Line, d.Column
	lines := strings.Split(text, "\n")
	if d.Line > 0 && d.Line <= len(lines) {
		line := strings.TrimSuffix(lines[d.Line-1], "\r")
		start := min(max(0, d.Column-1), len(line))
		end := start
		for end < len(line) && (line[end] == '_' || line[end] >= 'a' && line[end] <= 'z' || line[end] >= 'A' && line[end] <= 'Z' || line[end] >= '0' && line[end] <= '9') {
			end++
		}
		if end == start && end < len(line) {
			_, width := utf8.DecodeRuneInString(line[end:])
			end += width
		}
		d.Column, d.EndColumn = start+1, end+1
	}
	return d
}

// CheckLuaSource gives clients immediate syntax feedback, including for files
// outside the entry point's require graph. Semantic checks remain project-wide.
func CheckLuaSource(source, text string) []Diagnostic {
	if _, err := luaparser.ParseString(source, text); err != nil {
		return []Diagnostic{projectDiagnostic("", map[string]string{source: text}, source, err, DiagnosticError, "LUA_SYNTAX")}
	}
	return nil
}
