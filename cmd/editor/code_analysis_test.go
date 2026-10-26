package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	projecteditor "github.com/drpaneas/mtile/internal/editor"
)

func TestLiveProjectCheckUsesDirtyCodeAndShowsInformationalFallback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	if err := build.CreateProject(root, build.NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	g := &Game{model: &projecteditor.Model{Dir: root}, codeDocuments: make(map[string]*codeDocument)}
	if err := g.openCodeDocument(filepath.Join(root, "game.lua")); err != nil {
		t.Fatal(err)
	}
	// Replace the original buffer with an unsaved complete draw loop.
	g.code.SelectAll()
	g.code.Insert("function _draw() cls(0) circfill(4,4,1,7) end\n")
	g.refreshCodeDiagnostics()
	g.pollProjectCheck(time.Now().Add(time.Second))
	deadline := time.Now().Add(5 * time.Second)
	for !g.codeAnalysis.RendererReady && time.Now().Before(deadline) {
		g.pollProjectCheck(time.Now())
		time.Sleep(time.Millisecond)
	}
	lines := strings.Join(g.developPanelLines(developProblems), "\n")
	if !strings.Contains(lines, "LIVE RENDERER  auto -> mode4") || !strings.Contains(lines, "INFO") || !strings.Contains(lines, "M0_MODE4_BUILTIN") {
		t.Fatalf("live Problems = %q", lines)
	}
	if g.hasDevelopProblems() {
		t.Fatal("a successful AUTO fallback is treated as an error")
	}
	// A new edit invalidates the entire project result immediately.
	g.code.Insert("function unfinished(")
	g.refreshCodeDiagnostics()
	if g.codeAnalysis.RendererReady || len(g.codeAnalysis.Diagnostics) != 0 {
		t.Fatal("stale renderer result remained after edit")
	}
}

func TestLiveProjectCheckDiscardsOlderWorkerResult(t *testing.T) {
	g := &Game{model: &projecteditor.Model{Dir: t.TempDir()}, codeAnalysisResults: make(chan codeAnalysisResult, 1)}
	g.queueProjectCheck()
	old := g.codeAnalysisRevision
	g.queueProjectCheck()
	g.codeAnalysisResults <- codeAnalysisResult{g.model.Dir, old, build.ProjectAnalysis{RendererReady: true}}
	g.pollProjectCheck(time.Now())
	if g.codeAnalysis.RendererReady {
		t.Fatal("older source snapshot was published")
	}
}
