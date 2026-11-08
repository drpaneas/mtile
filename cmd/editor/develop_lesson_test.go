package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
)

func TestDevelopLessonAndBackUseBuffersWithoutChangingProject(t *testing.T) {
	if os.Getenv("MTILE_VERIFY_ROM_LESSON") == "" {
		t.Skip("set MTILE_VERIFY_ROM_LESSON to exercise TinyGo and mGBA")
	}
	projectDir := filepath.Join(t.TempDir(), "lesson-project")
	if err := build.CreateProject(projectDir, build.NewProjectOptions{}); err != nil {
		t.Fatal(err)
	}
	project, err := assetsync.LoadProject(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	project.Entry = "main.lua"
	if err := assetsync.SaveProject(projectDir, project); err != nil {
		t.Fatal(err)
	}
	lesson := filepath.Join(projectDir, "lessons", "01-lesson.lua")
	if err := os.Mkdir(filepath.Dir(lesson), 0o755); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(projectDir, project.Entry)
	release := filepath.Join(projectDir, "game.gba")
	for path, contents := range map[string]string{mainFile: "function _draw() cls(0) end\n", lesson: "function \n", release: "last release"} {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	unchanged := make(map[string][]byte)
	for _, path := range []string{mainFile, lesson, release, filepath.Join(projectDir, "project.json")} {
		unchanged[path] = read(path)
	}
	g, err := newGame(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.closeNativePerformance)
	t.Cleanup(g.closePlayMode)
	g.mode = modeCode
	g.code.SelectAll()
	g.code.Insert("function \n") // An invalid dirty main buffer cannot block the lesson.
	if err := g.openCodeDocument(lesson); err != nil {
		t.Fatal(err)
	}
	g.code.SelectAll()
	g.code.Insert("function _draw() cls(7) end\n")
	load := func(entry string) []byte {
		t.Helper()
		var launch playLaunch
		select {
		case launch = <-g.runReady:
		case <-time.After(90 * time.Second):
			t.Fatal("preview build timed out")
		}
		if launch.err != nil || launch.session == nil || launch.result.Entry != entry {
			launch.discard()
			t.Fatalf("preview entry = %q, error = %v", launch.result.Entry, launch.err)
		}
		var pixels []byte
		for range 12 {
			frame, err := launch.session.Step(0, false)
			if err != nil {
				launch.discard()
				t.Fatal(err)
			}
			pixels = frame.RGBA
		}
		g.acceptPlayLaunch(launch)
		return pixels
	}
	g.runDevelopLesson()
	lessonPixels := load("lessons/01-lesson.lua")
	if g.playLessonEntry != "lessons/01-lesson.lua" {
		t.Fatalf("preview is not labeled as a lesson: %q", g.playLessonEntry)
	}
	mainBuffer := g.codeDocuments[mainFile].Editor
	mainBuffer.SelectAll()
	mainBuffer.Insert("function _draw() cls(8) end\n")
	g.backToMyGame()
	gamePixels := load("main.lua")
	if g.playLessonEntry != "" || g.codePath != mainFile || !g.code.Dirty() || bytes.Equal(lessonPixels, gamePixels) {
		t.Fatalf("Back: lesson %q, entry %q (want %q), dirty %v, same pixels %v, status %q, error %q", g.playLessonEntry, g.codePath, mainFile, g.code.Dirty(), bytes.Equal(lessonPixels, gamePixels), g.runStatus, g.runError)
	}
	for path, before := range unchanged {
		if !bytes.Equal(before, read(path)) {
			t.Errorf("Run Lesson or Back wrote %s", path)
		}
	}
}
