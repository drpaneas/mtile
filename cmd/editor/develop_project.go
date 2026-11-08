package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/codeeditor"
)

type codeDocument struct {
	Path             string
	Relative         string
	Editor           *codeeditor.Editor
	ScrollLine       int
	ScrollColumn     int
	Diagnostic       string
	DiagnosticLine   int
	DiagnosticColumn int
	LastParsed       string
	DiskSource       string
}

type developProjectEntry struct {
	Path      string
	Relative  string
	Name      string
	Depth     int
	Directory bool
}

func cloneGlobalUsage(usage build.GlobalUsage) build.GlobalUsage {
	usage.Symbols = append([]build.GlobalSymbol(nil), usage.Symbols...)
	return usage
}

func (g *Game) projectSourceOverlay() map[string]string {
	g.syncActiveCodeDocument()
	overlay := make(map[string]string, len(g.codeDocuments))
	for _, document := range g.codeDocuments {
		if document == nil || document.Editor == nil || document.Relative == "" {
			continue
		}
		overlay[document.Relative] = document.Editor.Text()
	}
	return overlay
}

// refreshProjectSourceAnalysis updates the editor's source-file and global
// budget metadata without invoking TinyGo or writing a ROM.
func (g *Game) refreshProjectSourceAnalysis() (build.GlobalUsage, error) {
	if g == nil || g.model == nil {
		return build.GlobalUsage{}, fmt.Errorf("project is unavailable")
	}
	analysis, err := build.AnalyzeProjectSources(g.model.Dir, g.projectSourceOverlay())
	g.queueProjectCheck()
	g.runMu.Lock()
	g.runEntry = analysis.Entry
	g.runSourceFiles = append([]string(nil), analysis.SourceFiles...)
	g.runGlobalUsage = cloneGlobalUsage(analysis.Globals)
	g.sourceAnalysisError = ""
	if err != nil {
		g.sourceAnalysisError = err.Error()
	}
	g.runMu.Unlock()
	g.requestRender()
	return analysis.Globals, err
}

func secureDevelopPath(root, path string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file is outside the project")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	realRelative, err := filepath.Rel(realRoot, realPath)
	if err != nil || realRelative == ".." || strings.HasPrefix(realRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file symlink is outside the project")
	}
	return filepath.Clean(path), nil
}

func (g *Game) openCodeDocument(path string) error {
	if g == nil || g.model == nil {
		return fmt.Errorf("project is unavailable")
	}
	path, err := secureDevelopPath(g.model.Dir, path)
	if err != nil {
		return err
	}
	if filepath.Ext(path) != ".lua" {
		return fmt.Errorf("%s is not a Lua source file", filepath.Base(path))
	}
	if existing, ok := g.codeDocuments[path]; ok {
		g.activateCodeDocument(path)
		g.statusMsg = existing.Relative + " active"
		return nil
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	relative, _ := filepath.Rel(g.model.Dir, path)
	document := &codeDocument{
		Path:       path,
		Relative:   filepath.ToSlash(relative),
		Editor:     codeeditor.New(string(source)),
		DiskSource: string(source),
		Diagnostic: "No problems detected. Ready to build.",
	}
	g.codeDocuments[path] = document
	g.codeTabs = append(g.codeTabs, path)
	g.activateCodeDocument(path)
	g.refreshCodeDiagnostics()
	return nil
}

func (g *Game) syncActiveCodeDocument() {
	if g == nil || g.codePath == "" {
		return
	}
	document := g.codeDocuments[g.codePath]
	if document == nil {
		return
	}
	document.Editor = g.code
	document.ScrollLine = g.codeScrollLine
	document.ScrollColumn = g.codeScrollColumn
	document.Diagnostic = g.codeDiagnostic
	document.DiagnosticLine = g.codeDiagnosticLine
	document.DiagnosticColumn = g.codeDiagnosticColumn
	document.LastParsed = g.codeLastParsed
}

func (g *Game) activateCodeDocument(path string) {
	document := g.codeDocuments[path]
	if document == nil {
		return
	}
	g.syncActiveCodeDocument()
	g.codePath = document.Path
	g.code = document.Editor
	g.codeScrollLine = document.ScrollLine
	g.codeScrollColumn = document.ScrollColumn
	g.codeDiagnostic = document.Diagnostic
	g.codeDiagnosticLine = document.DiagnosticLine
	g.codeDiagnosticColumn = document.DiagnosticColumn
	g.codeLastParsed = document.LastParsed
	g.codeActiveTab = slices.Index(g.codeTabs, path)
	g.codeDragging = false
	g.codeHorizontalDragging = false
	g.codeVerticalDragging = false
	g.codeWheelX, g.codeWheelY = 0, 0
	g.codePointer = codePointerState{}
	g.codeSearchOpen = false
	g.codeCompletionOpen = false
	g.codeClosePending = ""
	for directory := filepath.Dir(path); directory != g.model.Dir && directory != "."; directory = filepath.Dir(directory) {
		relative, err := filepath.Rel(g.model.Dir, directory)
		if err != nil || strings.HasPrefix(relative, "..") {
			break
		}
		g.codeProjectExpanded[filepath.ToSlash(relative)] = true
	}
}

func (g *Game) saveCodeDocument(document *codeDocument) bool {
	if document == nil || document.Editor == nil || !document.Editor.Dirty() {
		return true
	}
	diskSource, err := os.ReadFile(document.Path)
	if err != nil {
		g.statusMsg = "save code: " + err.Error()
		return false
	}
	if string(diskSource) != document.DiskSource {
		g.codeReloadPending = document.Path
		g.codeReloadPendingAt = time.Now()
		g.statusMsg = "file changed outside editor; Cmd/Ctrl+Shift+R reloads it"
		return false
	}
	temporary := document.Path + ".tmp"
	if err := os.WriteFile(temporary, []byte(document.Editor.Text()), 0o644); err != nil {
		g.statusMsg = "save code: " + err.Error()
		return false
	}
	if err := os.Rename(temporary, document.Path); err != nil {
		_ = os.Remove(temporary)
		g.statusMsg = "save code: " + err.Error()
		return false
	}
	document.Editor.MarkSaved()
	document.DiskSource = document.Editor.Text()
	return true
}

func (g *Game) reloadActiveCodeDocument() {
	document := g.codeDocuments[g.codePath]
	if document == nil {
		return
	}
	if document.Editor.Dirty() && (g.codeReloadPending != document.Path || time.Since(g.codeReloadPendingAt) > 4*time.Second) {
		g.codeReloadPending = document.Path
		g.codeReloadPendingAt = time.Now()
		g.statusMsg = "unsaved changes; reload again within 4 seconds to discard"
		return
	}
	source, err := os.ReadFile(document.Path)
	if err != nil {
		g.statusMsg = "reload code: " + err.Error()
		return
	}
	// Activation copies the live editor and viewport back into the tab.
	g.code = codeeditor.New(string(source))
	document.DiskSource = string(source)
	g.codeScrollLine, g.codeScrollColumn = 0, 0
	g.activateCodeDocument(document.Path)
	g.refreshCodeDiagnostics()
	// Diagnostics are displayed through sourceAnalysisError; keep the project open.
	_, _ = g.refreshProjectSourceAnalysis()
	g.codeReloadPending = ""
	g.statusMsg = document.Relative + " reloaded from disk"
}

func (g *Game) saveAllCode() bool {
	g.syncActiveCodeDocument()
	saved := 0
	for _, path := range g.codeTabs {
		document := g.codeDocuments[path]
		if document != nil && document.Editor != nil && document.Editor.Dirty() {
			if !g.saveCodeDocument(document) {
				return false
			}
			saved++
		}
	}
	usage, analysisErr := g.refreshProjectSourceAnalysis()
	if analysisErr != nil {
		g.statusMsg = fmt.Sprintf("saved %d source file(s); source analysis found a problem", saved)
	} else if saved == 0 {
		g.statusMsg = "all source files already saved"
	} else {
		g.statusMsg = fmt.Sprintf("saved %d source file(s); globals %d/%d", saved, usage.Count, usage.Limit)
	}
	g.codeClosePending = ""
	return true
}

func (g *Game) hasDirtyCode() bool {
	g.syncActiveCodeDocument()
	for _, path := range g.codeTabs {
		if document := g.codeDocuments[path]; document != nil && document.Editor != nil && document.Editor.Dirty() {
			return true
		}
	}
	return false
}

func (g *Game) prepareCodeForBuild() bool {
	if g.preferences.AutoSaveBeforeBuild {
		return g.saveAllCode()
	}
	if g.hasDirtyCode() {
		g.statusMsg = "autosave is off; save source files before building"
		return false
	}
	return true
}

func (g *Game) closeCodeDocument(path string) {
	if len(g.codeTabs) <= 1 {
		g.statusMsg = "the last source tab stays open"
		return
	}
	document := g.codeDocuments[path]
	if document == nil {
		return
	}
	if document.Editor.Dirty() && (g.codeClosePending != path || time.Since(g.codeClosePendingAt) > 4*time.Second) {
		g.codeClosePending = path
		g.codeClosePendingAt = time.Now()
		g.statusMsg = "unsaved " + document.Relative + "; close again to discard or save first"
		return
	}
	index := slices.Index(g.codeTabs, path)
	if index < 0 {
		return
	}
	delete(g.codeDocuments, path)
	g.codeTabs = append(g.codeTabs[:index], g.codeTabs[index+1:]...)
	next := min(index, len(g.codeTabs)-1)
	g.activateCodeDocument(g.codeTabs[next])
	// Diagnostics are displayed through sourceAnalysisError; keep the project open.
	_, _ = g.refreshProjectSourceAnalysis()
	g.statusMsg = document.Relative + " closed"
}

func (g *Game) scanDevelopProject() {
	if g == nil || g.model == nil {
		return
	}
	entries := make([]developProjectEntry, 0, 64)
	_ = filepath.WalkDir(g.model.Dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == g.model.Dir {
			return nil
		}
		relative, err := filepath.Rel(g.model.Dir, path)
		if err != nil {
			return nil
		}
		relative = filepath.ToSlash(relative)
		base := entry.Name()
		if entry.IsDir() && (base == ".git" || base == ".idea" || base == ".vscode") {
			return filepath.SkipDir
		}
		if strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".tmp") || base == "game.gba" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		depth := strings.Count(relative, "/")
		entries = append(entries, developProjectEntry{Path: path, Relative: relative, Name: base, Depth: depth, Directory: entry.IsDir()})
		if len(entries) >= 600 {
			return fs.SkipAll
		}
		return nil
	})
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		adir, bdir := filepath.ToSlash(filepath.Dir(a.Relative)), filepath.ToSlash(filepath.Dir(b.Relative))
		if adir == bdir && a.Directory != b.Directory {
			return a.Directory
		}
		return strings.ToLower(a.Relative) < strings.ToLower(b.Relative)
	})
	g.codeProjectEntries = entries
	if g.codePath != "" {
		g.activateCodeDocument(g.codePath)
	}
	g.updateQuickOpenMatches()
}

func (g *Game) visibleDevelopProjectEntries() []developProjectEntry {
	visible := make([]developProjectEntry, 0, len(g.codeProjectEntries))
	for _, entry := range g.codeProjectEntries {
		show := true
		parent := filepath.ToSlash(filepath.Dir(entry.Relative))
		for parent != "." && parent != "" {
			if !g.codeProjectExpanded[parent] {
				show = false
				break
			}
			parent = filepath.ToSlash(filepath.Dir(parent))
		}
		if show {
			visible = append(visible, entry)
		}
	}
	return visible
}

func (g *Game) activateProjectEntry(entry developProjectEntry) {
	if entry.Directory {
		g.codeProjectExpanded[entry.Relative] = !g.codeProjectExpanded[entry.Relative]
		return
	}
	clean := filepath.ToSlash(entry.Relative)
	switch {
	case filepath.Ext(entry.Path) == ".lua":
		if err := g.openCodeDocument(entry.Path); err != nil {
			g.statusMsg = "open code: " + err.Error()
		}
	case clean == "assets/spritesheet.json":
		g.setMode(modeSprites)
	case clean == "assets/map.json":
		g.setMode(modeMap)
	case strings.HasPrefix(clean, "assets/audio/"):
		g.setMode(modeMusic)
	default:
		g.statusMsg = entry.Relative + " is visible but has no built-in text editor"
	}
}

func (g *Game) updateQuickOpenMatches() {
	needle := strings.ToLower(strings.TrimSpace(g.codeQuickDraft))
	g.codeQuickMatches = g.codeQuickMatches[:0]
	for _, entry := range g.codeProjectEntries {
		if entry.Directory || filepath.Ext(entry.Path) != ".lua" {
			continue
		}
		candidate := strings.ToLower(entry.Relative)
		if needle == "" || strings.Contains(candidate, needle) {
			g.codeQuickMatches = append(g.codeQuickMatches, entry.Path)
		}
	}
	if len(g.codeQuickMatches) == 0 {
		g.codeQuickIndex = 0
	} else {
		g.codeQuickIndex = min(g.codeQuickIndex, len(g.codeQuickMatches)-1)
	}
}
