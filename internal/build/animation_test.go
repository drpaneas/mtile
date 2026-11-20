package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestBuildRejectsMissingAnimationBeforeToolchain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "animation")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte("function _draw()\n cls()\n anim(\"idle\",20,30)\nend"), 0644); err != nil {
		t.Fatal(err)
	}
	service := Service{TinyGoBin: filepath.Join(t.TempDir(), "must-not-run"), RuntimeDir: t.TempDir(), Renderer: transpiler.RendererMode4}
	_, err := service.Build(dir, "")
	if err == nil || !strings.Contains(err.Error(), "[ANIM_UNKNOWN_CLIP]") || !strings.Contains(err.Error(), "game.lua:3") {
		t.Fatalf("missing clip should fail before invoking TinyGo: %v", err)
	}
}

func TestBuildAnimationAllocationProofMatchesLinkedMemory(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("real TinyGo link")
	}
	for _, tc := range []struct {
		name, prelude, draw string
		heap                bool
	}{
		{"literal", "", `anim("idle",100,60)`, false},
		{"static-choice", "", `local name="idle" if btn(0) then name="hold" end anim(name,100,60)`, false},
		{"static-global-choice", `name="idle" missing="absent" function actor(x) anim(name,x,60) end function _update() if btn(0) then name="hold" else name="idle" end end`, `actor(100) anim(missing,0,0)`, false},
		{"pose", "", `pspr(0,100,60)`, false},
		{"allocated-name", "", `anim("id".."le",100,60)`, true},
		{"allocated-global-name", `name="idle" function _update() name="id".."le" end`, `anim(name,100,60)`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "animation")
			if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
				t.Fatal(err)
			}
			if err := assetsync.SavePoses(dir, &assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "pose", Parts: []assetsync.PosePartRecord{{Tile: 0}}}}}); err != nil {
				t.Fatal(err)
			}
			if err := assetsync.SaveAnimationWorkspace(dir, &assetsync.AnimationWorkspace{Version: 1, Clips: []assetsync.AnimationClip{{Name: "idle", FPS: 8, Frames: []int{0}}, {Name: "hold", FPS: 12, Frames: []int{0}}}}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(tc.prelude+"\nfunction _draw() cls() "+tc.draw+" end"), 0600); err != nil {
				t.Fatal(err)
			}
			service, err := NewService("", "")
			if err != nil {
				t.Fatal(err)
			}
			service.KeepWork = true
			service.Renderer = transpiler.RendererMode0
			result, err := service.Build(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.RemoveAll(result.WorkDir) }()
			if result.HeapLinked != tc.heap {
				t.Fatalf("HeapLinked=%t want%t", result.HeapLinked, tc.heap)
			}
			linkMap, err := os.ReadFile(result.MapPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, symbol := range []string{"heapSpaceA", "heapSpaceB", "collectCore"} {
				if strings.Contains(string(linkMap), symbol) != tc.heap {
					t.Errorf("linked %s=%t want%t", symbol, !tc.heap, tc.heap)
				}
			}
			if !tc.heap && result.EWRAMUsed >= 16*1024 {
				t.Fatalf("heapless one-tile scene uses %d EWRAM bytes", result.EWRAMUsed)
			}
		})
	}
}
