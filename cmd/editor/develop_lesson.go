package main

import (
	"path/filepath"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/editor"
)

func (g *Game) activeLessonEntry() string {
	document := g.codeDocuments[g.codePath]
	if document == nil || filepath.Ext(document.Relative) != ".lua" || filepath.Dir(filepath.FromSlash(document.Relative)) != "lessons" {
		return ""
	}
	return document.Relative
}

func (g *Game) runDevelopLesson() {
	entry := g.activeLessonEntry()
	if entry == "" {
		return
	}
	if g.startPreviewBuild(entry, g.projectSourceOverlay(), false) {
		g.lessonEntry = entry
		g.queueProjectCheck()
	}
}

func (g *Game) backToMyGame() {
	g.runMu.Lock()
	busy := g.runBusy
	g.runMu.Unlock()
	if busy || g.romExport.ready != nil {
		return
	}
	entry := g.model.Project.Entry
	if entry == "" {
		entry = build.GameFileName
	}
	if err := g.openCodeDocument(filepath.Join(g.model.Dir, filepath.FromSlash(entry))); err != nil {
		g.statusMsg = "open game: " + err.Error()
		return
	}
	if g.startPreviewBuild("", g.projectSourceOverlay(), false) {
		g.lessonEntry = ""
		g.queueProjectCheck()
	}
}

func developBackToGameRect(layout developLayout) editor.Rect {
	stage := layout.Play.Stage
	return editor.Rect{X: stage.X + 12, Y: stage.Y + 5, W: 146, H: 20}
}
