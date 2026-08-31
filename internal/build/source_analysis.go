package build

import (
	"fmt"
	"path/filepath"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/sourcebundle"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// GlobalSymbol is one distinct persistent Lua name and the first project
// source position that caused the transpiler to classify it as global.
type GlobalSymbol struct {
	Name   string
	Source string
	Line   int
	Column int
}

// GlobalUsage is the build-time persistent-state budget exposed to the editor.
type GlobalUsage struct {
	Count   int
	Limit   int
	Symbols []GlobalSymbol
}

// SourceAnalysis is the source-only part of a build. It does not load assets,
// select a renderer, invoke TinyGo, or write a ROM.
type SourceAnalysis struct {
	Entry       string
	SourceFiles []string
	Globals     GlobalUsage
}

// AnalyzeProjectSources resolves the explicit require graph and reports its
// persistent globals. Overlay keys are project-relative Lua paths whose text
// replaces the on-disk contents, allowing the editor to include dirty tabs.
func AnalyzeProjectSources(projectDir string, overlay map[string]string) (SourceAnalysis, error) {
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return SourceAnalysis{}, fmt.Errorf("resolve project directory: %w", err)
	}
	project, err := assetsync.LoadProject(projectDir)
	if err != nil {
		return SourceAnalysis{}, err
	}
	entry := project.Entry
	if entry == "" {
		entry = GameFileName
	}
	bundle, err := sourcebundle.ResolveWithOverlay(projectDir, entry, overlay)
	if err != nil {
		return SourceAnalysis{}, fmt.Errorf("resolve Lua sources: %w", err)
	}
	analysis, err := analyzeResolvedSources(bundle)
	if err != nil {
		return analysis, fmt.Errorf("transpile %s: %w", bundle.Entry, bundle.RemapError(err))
	}
	return analysis, nil
}

func analyzeResolvedSources(bundle sourcebundle.Bundle) (SourceAnalysis, error) {
	report, err := transpiler.AnalyzeGlobals(bundle.Entry, bundle.Source)
	analysis := SourceAnalysis{
		Entry:       bundle.Entry,
		SourceFiles: append([]string(nil), bundle.Files...),
		Globals: GlobalUsage{
			Count: report.Count,
			Limit: report.Limit,
		},
	}
	analysis.Globals.Symbols = make([]GlobalSymbol, 0, len(report.Symbols))
	for _, symbol := range report.Symbols {
		source, line, column := symbol.Position.Source, symbol.Position.Line, symbol.Position.Column
		if relative, sourceLine, sourceColumn, ok := bundle.Position(line, column); ok {
			source, line, column = relative, sourceLine, sourceColumn
		}
		analysis.Globals.Symbols = append(analysis.Globals.Symbols, GlobalSymbol{
			Name: symbol.Name, Source: source, Line: line, Column: column,
		})
	}
	return analysis, err
}
