package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/audioimport"
)

func waitPCM(t *testing.T, g *Game) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for g.pcm.job != nil {
		if time.Now().After(deadline) {
			t.Fatal("PCM background job timed out")
		}
		g.pcmPollJob()
		time.Sleep(time.Millisecond)
	}
}

func TestPCMEditorPreparesAndKeepsCustomNameAcrossQualityChanges(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	path := filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/sfx_bell.wav")
	g.pcmPrepare(path)
	waitPCM(t, g)
	if g.pcm.phase != "review" || g.pcm.prepared.After.Bits != 8 || g.pcm.prepared.After.Channels != 1 {
		t.Fatalf("prepare state: %+v", g.pcm)
	}
	g.pcm.name = "door_bell"
	g.pcm.quality = audioimport.QualitySmall
	g.pcmPrepare(path)
	waitPCM(t, g)
	if g.pcm.name != "door_bell" || g.pcm.prepared.After.Rate > 8000 {
		t.Fatal("quality changed name or ignored rate")
	}
	before := g.pcm.prepared
	g.pcmStartJob("test", func() pcmJobResult { return pcmJobResult{kind: "prepare", err: errors.New("bad WAV")} })
	waitPCM(t, g)
	if g.pcm.message != "bad WAV" || string(g.pcm.prepared.Data) != string(before.Data) {
		t.Fatal("failed prepare replaced valid audio")
	}
}

func TestPCMEditorJobDoesNotPublishUIStateFromWorker(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	ready := make(chan struct{})
	g.pcmStartJob("Working", func() pcmJobResult {
		<-ready
		return pcmJobResult{kind: "prepare", prepared: audioimport.Prepared{Name: "jump"}}
	})
	g.pcmStartJob("Duplicate", func() pcmJobResult { t.Error("overlapping job ran"); return pcmJobResult{} })
	if g.pcm.phase != "choose" || g.pcm.busy != "Working" {
		t.Fatal("worker mutated UI state")
	}
	close(ready)
	waitPCM(t, g)
	if g.pcm.phase != "review" || g.pcm.name != "jump" {
		t.Fatal("result not applied")
	}
}

func TestPCMEditorRealImportReplaceAndDelete(t *testing.T) {
	tool := os.Getenv("MTILE_PCM_MMUTIL")
	if tool == "" {
		t.Skip("set MTILE_PCM_MMUTIL for real converter/editor integration")
	}
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	g.pcm.converter = tool
	path := filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/sfx_bell.wav")
	g.pcmPrepare(path)
	waitPCM(t, g)
	g.pcm.name = "jump"
	g.pcmImport()
	waitPCM(t, g)
	if g.pcm.phase != "done" || g.pcm.command != "sample(0)" || len(g.pcm.entries) != 1 {
		t.Fatalf("import: %s %s", g.pcm.phase, g.pcm.message)
	}
	if _, err := os.Stat(filepath.Join(g.model.Dir, "assets/audio/direct/source/sfx_jump.wav")); err != nil {
		t.Fatal(err)
	}
	g.pcmPrepare(path)
	waitPCM(t, g)
	g.pcm.name = "jump"
	g.pcmImport()
	if !g.pcm.confirmReplace || g.pcm.job != nil {
		t.Fatal("replacement was not confirmed")
	}
	g.pcmImport()
	waitPCM(t, g)
	if g.pcm.command != "sample(0)" || len(g.pcm.entries) != 1 {
		t.Fatal("replacement changed ID")
	}
	g.pcmSelectEntry(0)
	waitPCM(t, g)
	g.pcmDelete()
	a, err := assetsync.LoadDirectAudio(g.model.Dir)
	if err != nil || a.Manifest.Samples[0].Deleted {
		t.Fatal("first click deleted asset", err)
	}
	g.pcmDelete()
	waitPCM(t, g)
	a, err = assetsync.LoadDirectAudio(g.model.Dir)
	if err != nil || !a.Manifest.Samples[0].Deleted || len(g.pcm.entries) != 0 || !strings.Contains(g.pcm.message, "Other IDs are unchanged") {
		t.Fatal("delete failed", err, g.pcm.message)
	}
}

func TestPCMLayoutFitsShippedDesktopSizes(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{1024, 900}, {1920, 1200}, {2560, 1440}} {
		setEditorLayout(size[0], size[1])
		l := newPCMLayout()
		for _, r := range []struct {
			name       string
			x, y, w, h int
		}{
			{"list", l.List.X, l.List.Y, l.List.W, l.List.H}, {"body", l.Body.X, l.Body.Y, l.Body.W, l.Body.H},
			{"import", l.Commit.X, l.Commit.Y, l.Commit.W, l.Commit.H}, {"name", l.Name.X, l.Name.Y, l.Name.W, l.Name.H},
		} {
			if r.w <= 0 || r.h <= 0 || r.x < l.Panel.X || r.y < l.Panel.Y || r.x+r.w > l.Panel.X+l.Panel.W || r.y+r.h > l.Panel.Y+l.Panel.H {
				t.Fatalf("%v %s outside panel", size, r.name)
			}
		}
		if pcmButtonRect().X+pcmButtonRect().W > size[0] {
			t.Fatalf("PCM navigation outside %v", size)
		}
	}
}
