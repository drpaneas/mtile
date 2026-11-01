package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/build"
	projecteditor "github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/lint"
)

const lintEditorEntry = `require("parts")
function _update()
 if btn(4) then
  assemble()
  assemble()
 end
end
function _draw() cls(0) end
`

func newLintEditorFixture(t *testing.T) *Game {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project with spaces")
	if err := build.CreateProject(root, build.NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{"game.lua": lintEditorEntry, "src/parts.lua": "function assemble()\n  cache = {}\nend\n"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g := &Game{model: &projecteditor.Model{Dir: root}, codeDocuments: make(map[string]*codeDocument), codeProjectExpanded: make(map[string]bool), developPanel: developLint}
	if err := g.openCodeDocument(filepath.Join(root, "game.lua")); err != nil {
		t.Fatal(err)
	}
	return g
}

func waitForLintEditorCheck(t *testing.T, g *Game) {
	t.Helper()
	g.pollProjectCheck(time.Now().Add(time.Second))
	deadline := time.Now().Add(5 * time.Second)
	for !g.codeAnalysis.LintReady && time.Now().Before(deadline) {
		g.pollProjectCheck(time.Now())
		time.Sleep(time.Millisecond)
	}
	if !g.codeAnalysis.LintReady {
		t.Fatalf("lint check did not complete: %+v", g.codeAnalysis)
	}
}

func TestDevelopLintUsesDirtyModulesWithoutSavingOrBuilding(t *testing.T) {
	g := newLintEditorFixture(t)
	module := filepath.Join(g.model.Dir, "src", "parts.lua")
	if err := g.openCodeDocument(module); err != nil {
		t.Fatal(err)
	}
	g.code.SelectAll()
	g.code.Insert("function assemble()\n  local label = \"π\"; cache = {}\nend\n")
	g.refreshCodeDiagnostics()
	waitForLintEditorCheck(t, g)
	if g.lintAdviceCount() != 1 || g.hasDevelopProblems() {
		t.Fatalf("advice must be present without being a build error: %+v", g.codeAnalysis)
	}
	lines := strings.Join(g.developLintLines(), "\n")
	if !strings.Contains(lines, "ADVICE [MTL008]") || !strings.Contains(lines, "π") || strings.Contains(lines, "NOTE [MTL001]") {
		t.Fatalf("wrong lint view: %s", lines)
	}
	if !g.code.Dirty() {
		t.Fatal("analysis saved the dirty buffer")
	}
	disk, _ := os.ReadFile(module)
	if strings.Contains(string(disk), "π") {
		t.Fatal("analysis wrote unsaved text")
	}
	if _, err := os.Stat(filepath.Join(g.model.Dir, "game.gba")); !os.IsNotExist(err) {
		t.Fatal("analysis built a ROM")
	}
	// Changes invalidate old lint immediately, before a newer worker completes.
	g.code.Insert("function unfinished(")
	g.refreshCodeDiagnostics()
	if g.codeAnalysis.LintReady || len(g.codeAnalysis.Lint) > 0 || strings.Contains(strings.Join(g.developLintLines(), "\n"), "MTL008") {
		t.Fatal("stale lint remained visible after an edit")
	}
}

func TestDevelopLintNotesKeyboardToggleAndCopy(t *testing.T) {
	preserveDisplayState(t)
	g := newLintEditorFixture(t)
	waitForLintEditorCheck(t, g)
	before := g.code.Text()
	rows := g.developLintRows()
	index := -1
	for i, row := range rows {
		if row.action == lintToggleNotes {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("context notes are not discoverable")
	}
	v := g.developTextView(newDevelopPanelLayout(developLint))
	v.document.SetCursor(v.document.Offset(index, 0), false)
	g.openDevelopLintRow() // The same action as Enter on the selected row.
	if !g.lintShowNotes || !strings.Contains(strings.Join(g.developLintLines(), "\n"), "NOTE [MTL001]") {
		t.Fatal("keyboard did not reveal notes")
	}
	g.developTextView(newDevelopPanelLayout(developLint))
	var copied string
	g.copyDevelopText(true, func(value string) error { copied = value; return nil })
	if !strings.Contains(copied, "MTL008") || !strings.Contains(copied, "replacement reached through assemble()") || !strings.Contains(copied, "Docs:") {
		t.Fatalf("copy lost details: %q", copied)
	}
	if g.code.Text() != before {
		t.Fatal("reading/copying lint changed Lua")
	}
}

func TestDevelopLintOpensPrimaryAndRelatedUnsavedSourceWithRuneColumns(t *testing.T) {
	preserveDisplayState(t)
	g := newLintEditorFixture(t)
	module := filepath.Join(g.model.Dir, "src", "parts.lua")
	if err := g.openCodeDocument(module); err != nil {
		t.Fatal(err)
	}
	text := "function assemble()\n  local label = \"π\"; cache = {}\nend\n"
	g.code.SelectAll()
	g.code.Insert(text)
	g.refreshCodeDiagnostics()
	if err := g.openCodeDocument(filepath.Join(g.model.Dir, "game.lua")); err != nil {
		t.Fatal(err)
	}
	waitForLintEditorCheck(t, g)
	var finding lint.Diagnostic
	for _, d := range g.codeAnalysis.Lint {
		if d.Rule == "MTL008" {
			finding = d
		}
	}
	if finding.Rule == "" {
		t.Fatal("missing discarded construction")
	}
	index := -1
	for i, row := range g.developLintRows() {
		if row.action == lintOpenSource && row.at.Path == finding.Path && row.at.Column == finding.Column {
			index = i
			break
		}
	}
	if index < 0 || !g.activateDevelopLintRow(index, nil) {
		t.Fatal("primary source action missing")
	}
	line := strings.Split(text, "\n")[finding.Line-1]
	wantColumn := utf8.RuneCountInString(line[:finding.Column-1])
	gotLine, gotColumn := g.code.LineColumn(g.code.Cursor())
	if g.codePath != module || gotLine != finding.Line-1 || gotColumn != wantColumn || g.code.Text() != text || !g.code.Dirty() {
		t.Fatalf("navigation changed/lost the dirty document or byte/rune position: %s %d:%d", g.codePath, gotLine, gotColumn)
	}
	// The first and replacing caller locations are independently navigable.
	for _, related := range finding.Related {
		if related.Path != "game.lua" {
			continue
		}
		index = -1
		for i, row := range g.developLintRows() {
			if row.action == lintOpenSource && row.at == related.Location {
				index = i
				break
			}
		}
		if index < 0 {
			t.Fatalf("missing related action: %+v", related)
		}
		g.activateDevelopLintRow(index, nil)
		gotLine, _ = g.code.LineColumn(g.code.Cursor())
		if filepath.Base(g.codePath) != "game.lua" || gotLine != related.Line-1 {
			t.Fatalf("wrong related target: %s:%d", g.codePath, gotLine)
		}
	}
}

func TestDevelopLintDocumentationActionAndError(t *testing.T) {
	g := newLintEditorFixture(t)
	waitForLintEditorCheck(t, g)
	index := -1
	for i, row := range g.developLintRows() {
		if row.action == lintOpenDocs {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("missing explanation action")
	}
	var requested string
	g.activateDevelopLintRow(index, func(raw string) error { requested = raw; return nil })
	if !strings.HasSuffix(requested, "docs/lint.md#mtl008-discarded-construction") {
		t.Fatalf("wrong explanation URL: %q", requested)
	}
	g.activateDevelopLintRow(index, func(string) error { return errors.New("browser unavailable") })
	if !strings.Contains(g.statusMsg, "browser unavailable") {
		t.Fatal("browser failure was silent")
	}
}

func TestDevelopLintDiscardsOldWorkerAndMarksOnlyLintTab(t *testing.T) {
	g := newLintEditorFixture(t)
	g.codeAnalysisResults = make(chan codeAnalysisResult, 1)
	g.queueProjectCheck()
	old := g.codeAnalysisRevision
	g.queueProjectCheck()
	g.codeAnalysisResults <- codeAnalysisResult{root: g.model.Dir, revision: old, analysis: build.ProjectAnalysis{LintReady: true, Lint: []lint.Diagnostic{{Rule: "MTL008", Message: "stale"}}}}
	g.pollProjectCheck(time.Now())
	if g.codeAnalysis.LintReady || strings.Contains(strings.Join(g.developLintLines(), "\n"), "stale") {
		t.Fatal("older lint snapshot was accepted")
	}
	g.codeAnalysis = build.ProjectAnalysis{LintReady: true, Lint: []lint.Diagnostic{{Rule: "MTL008", Severity: "advice", Message: "Contains the word error but is advice"}}}
	if g.hasDevelopProblems() {
		t.Fatal("lint advice is treated as a build problem")
	}
	if developLintColor(developLintRow{role: "advice"}) == colorDanger {
		t.Fatal("advice uses compile-error styling")
	}
}

func TestDevelopLintFitsTabsAndPreviewAtSupportedWindowSizes(t *testing.T) {
	preserveDisplayState(t)
	g := &Game{developPanel: developLint}
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1280, 900}, {1920, 1200}} {
		setEditorLayout(size[0], size[1])
		layout := newDevelopPanelLayout(developLint)
		for _, tab := range g.developPanelTabRects(layout) {
			if !rectInsideIDE(tab, layout.Code.Diagnostics) || rectsOverlapIDE(tab, g.developCopyRect(layout)) {
				t.Fatal("lint tab overlaps panel actions")
			}
		}
		if !rectInsideIDE(layout.Play.Bezel, layout.Play.Stage) {
			t.Fatalf("lint panel covers cartridge preview at %v: bezel=%+v stage=%+v", size, layout.Play.Bezel, layout.Play.Stage)
		}
	}
}
