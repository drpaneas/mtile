package main

import (
	"fmt"
	"time"

	"github.com/drpaneas/mtile/internal/build"
)

type codeAnalysisResult struct {
	root     string
	revision uint64
	analysis build.ProjectAnalysis
}

// These fields belong to the UI thread. The worker receives immutable source
// snapshots and only sends through its result channel.
func (g *Game) queueProjectCheck() {
	if g == nil || g.model == nil {
		return
	}
	g.codeAnalysisRevision++
	g.codeAnalysisDue = time.Now().Add(250 * time.Millisecond)
	g.codeAnalysis = build.ProjectAnalysis{}
	g.requestRender()
}

func (g *Game) pollProjectCheck(now time.Time) {
	if g == nil || g.model == nil {
		return
	}
	if g.codeAnalysisResults == nil {
		g.codeAnalysisResults = make(chan codeAnalysisResult, 1)
	}
	select {
	case result := <-g.codeAnalysisResults:
		g.codeAnalysisBusy = false
		if result.root == g.model.Dir && result.revision == g.codeAnalysisRevision {
			g.codeAnalysis = result.analysis
			g.runMu.Lock()
			g.sourceAnalysisError = ""
			g.runMu.Unlock()
			g.requestRender()
		}
	default:
	}
	if g.codeAnalysisBusy || g.codeAnalysisDue.IsZero() || now.Before(g.codeAnalysisDue) {
		return
	}
	g.codeAnalysisBusy = true
	g.codeAnalysisDue = time.Time{}
	root, revision, overlay, results := g.model.Dir, g.codeAnalysisRevision, g.projectSourceOverlay(), g.codeAnalysisResults
	go func() {
		analysis := (build.Service{UseProjectRenderer: true}).CheckProject(root, overlay)
		results <- codeAnalysisResult{root, revision, analysis}
	}()
}

func (g *Game) liveAnalysisLines() []string {
	var lines []string
	if report := g.codeAnalysis.Renderer; g.codeAnalysis.RendererReady {
		lines = append(lines, fmt.Sprintf("LIVE RENDERER  %s -> %s  [%s]", report.Requested, report.Selected, report.Reason))
	}
	for _, diagnostic := range g.codeAnalysis.Diagnostics {
		severity := "ERROR"
		switch diagnostic.Severity {
		case build.DiagnosticInformation:
			severity = "INFO"
		case build.DiagnosticWarning:
			severity = "WARNING"
		}
		lines = append(lines, severity+"  "+diagnostic.Message)
	}
	return lines
}
