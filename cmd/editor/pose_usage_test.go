package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/luaparser"
)

func TestPoseCopyLuaCallTracksSelectionAndLibraryEdits(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	old := writePoseUsageClipboard
	t.Cleanup(func() { writePoseUsageClipboard = old })
	var copied string
	writePoseUsageClipboard = func(text string) error { copied = text; return nil }
	g.selectPose(1)
	layout := newAnimationDockLayout(g.animationDockRect())
	g.clickAnimationDock(layout, layout.CopyPose.X+1, layout.CopyPose.Y+1)
	if copied != `pspr("idle_1", x, y)` {
		t.Fatalf("copied %q", copied)
	}
	g.selectPose(0)
	g.deleteSelectedPose()
	g.copyPoseUsage()
	if copied != `pspr("idle_1", x, y)` {
		t.Fatalf("deleting another pose changed the copied reference: %q", copied)
	}
	g.model.Poses.Poses[0].Name = ""
	g.copyPoseUsage()
	if copied != `pspr(0, x, y)` {
		t.Fatalf("unnamed pose compatibility: %q", copied)
	}
	g.selectedPose = -1
	copied = ""
	g.copyPoseUsage()
	if copied != "" || !strings.Contains(g.statusMsg, "Select a registered pose") {
		t.Fatalf("invalid selection copied=%q status=%q", copied, g.statusMsg)
	}
	g.selectedPose = 0
	writePoseUsageClipboard = func(string) error { return errors.New("clipboard unavailable") }
	g.copyPoseUsage()
	if !strings.Contains(g.statusMsg, "Could not copy pose call: clipboard unavailable") {
		t.Fatal(g.statusMsg)
	}
}

func TestPoseCopyLuaCallEscapesAssetNames(t *testing.T) {
	g := newTestGameWithPoses(t)
	g.selectedPose = 0
	for _, name := range []string{`hero_"idle"\frame`, "hero\n\x017", "ήρωας"} {
		g.model.Poses.Poses[0].Name = name
		chunk, err := luaparser.ParseString("copy.lua", g.poseUsage())
		if err != nil {
			t.Fatal(err)
		}
		call := chunk.Body[0].(*luaparser.CallStmt).Call
		if got := call.Args[0].(*luaparser.StringExpr).Value; got != name {
			t.Fatalf("copied name=%q want=%q", got, name)
		}
	}
}

func TestDevelopProblemsDisplaysNonfatalPoseWarnings(t *testing.T) {
	g := newTestGameWithPoses(t)
	warning := "Warning: src/player.lua:4:7: [PSPR_ID_OUT_OF_RANGE] pspr ID 99 has no pose"
	g.runBuildEvents = []build.Event{{Phase: build.PhaseTranspile, Message: warning}}
	if !g.hasDevelopProblems() {
		t.Fatal("warning did not mark Problems")
	}
	lines := strings.Join(g.developPanelLines(developProblems), "\n")
	if !strings.Contains(lines, warning) || strings.Contains(lines, "No source or build problems") {
		t.Fatal(lines)
	}
}

func TestDevelopPoseWarningOpensOriginalSourcePosition(t *testing.T) {
	g := newTestGameWithPoses(t)
	dir := filepath.Join(g.model.Dir, "src")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "player.lua")
	if err := os.WriteFile(path, []byte("function draw_actor()\n pspr(99,0,0)\nend\n"), 0644); err != nil {
		t.Fatal(err)
	}
	g.runBuildEvents = []build.Event{{Phase: build.PhaseTranspile, Message: "Warning: src/player.lua:2:7: [PSPR_ID_OUT_OF_RANGE] pspr ID 99 has no pose"}}
	g.openDevelopDiagnostic()
	if g.codePath != path || g.code.Cursor() != g.code.Offset(1, 6) {
		t.Fatalf("warning opened %q cursor %d; status %q", g.codePath, g.code.Cursor(), g.statusMsg)
	}
}

func TestPoseStripCopyControlsFitAllTextSizes(t *testing.T) {
	t.Cleanup(func() { setInterfaceTextSize(textComfort); setEditorLayout(defaultScreenWidth, defaultScreenHeight) })
	g := newTestGameWithPoses(t)
	g.model.ObjSheet = nil // Legacy poses backed by the shared sheet use the strip.
	for size := textCompact; size < interfaceTextSizeCount; size++ {
		setInterfaceTextSize(size)
		setEditorLayout(minimumScreenWidth, minimumScreenHeight)
		strip := g.poseStripRect()
		controls := []editor.Rect{g.poseStripNewRect(), g.poseStripSetRect(), g.poseStripDelRect(), g.poseStripCopyRect()}
		for i, r := range controls {
			if !rectInsideIDE(r, strip) {
				t.Fatalf("%s control %d %+v escapes %+v", size, i, r, strip)
			}
			for _, other := range controls[:i] {
				if rectsOverlapIDE(r, other) {
					t.Fatalf("%s controls overlap", size)
				}
			}
		}
		if smallTextWidth("COPY LUA")+16 > g.poseStripCopyRect().W {
			t.Fatalf("%s copy label does not fit", size)
		}
	}
}
