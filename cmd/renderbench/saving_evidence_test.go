package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordedSaveEvidence(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "saving-v1")
	requireRecordedBenchmarkParity(t, evidence)
	if evidence.Mode0.P95Cycles >= twoFrameCycleBudget || evidence.Mode4.P95Cycles >= twoFrameCycleBudget || evidence.Mode0.MissedFrames != 0 || evidence.Mode4.MissedFrames != 0 {
		t.Fatal("save stress workload must fit the update budget without missed frames")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "saving-oracle-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		SchemaVersion   int  `json:"schemaVersion"`
		CapturedFrames  int  `json:"capturedFrames"`
		DifferingPixels int  `json:"differingPixels"`
		ZeroSaveErrors  bool `json:"zeroSaveErrors"`
		Mode0           struct {
			ROMSHA string `json:"romSha256"`
		} `json:"mode0"`
		Mode4 struct {
			ROMSHA string `json:"romSha256"`
		} `json:"mode4"`
	}
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	if oracle.SchemaVersion != 1 || oracle.CapturedFrames < 120 || oracle.DifferingPixels != 0 || !oracle.ZeroSaveErrors || oracle.Mode0.ROMSHA != evidence.Mode0.ROMSHA || oracle.Mode4.ROMSHA != evidence.Mode4.ROMSHA {
		t.Fatalf("missing or mismatched ARM save oracle: %+v", oracle)
	}
}

func TestRecordedSharedPauseMenuEvidence(t *testing.T) {
	root := filepath.Join("..", "..", "docs", "benchmarks", "render-modes")
	data, err := os.ReadFile(filepath.Join(root, "pause-menu-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var menu struct {
		Schema    int `json:"schemaVersion"`
		Frames    int `json:"capturedFrames"`
		Pixels    int `json:"differingPixels"`
		Renderers map[string]struct {
			SHA     string `json:"romSha256"`
			Samples int    `json:"samples"`
			Median  uint32 `json:"medianMenuCycles"`
			P95     uint32 `json:"p95MenuCycles"`
		} `json:"renderers"`
	}
	if err := json.Unmarshal(data, &menu); err != nil {
		t.Fatal(err)
	}
	if menu.Schema != 1 || menu.Frames < 120 || menu.Pixels != 0 {
		t.Fatal("missing pause parity proof")
	}
	for _, name := range []string{"mode0", "mode4", "auto"} {
		r, ok := menu.Renderers[name]
		if !ok || len(r.SHA) != 64 || r.Samples < 120 || r.Median == 0 || r.P95 >= twoFrameCycleBudget {
			t.Fatalf("missing menu timing for %s: %+v", name, r)
		}
	}
	if menu.Renderers["auto"].SHA != menu.Renderers["mode0"].SHA {
		t.Fatal("AUTO menu differs from Mode 0")
	}
	data, err = os.ReadFile(filepath.Join(root, "pause-no-input-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plain struct {
		NoInput     bool `json:"worksWithoutGameInput"`
		Unavailable bool `json:"unavailableSavingHasFeedback"`
		Restored    bool `json:"resumeRestoresPixels"`
		Ended       bool `json:"exitEndsSession"`
		Pixels      int  `json:"differingPixels"`
	}
	if err := json.Unmarshal(data, &plain); err != nil {
		t.Fatal(err)
	}
	if !plain.NoInput || !plain.Unavailable || !plain.Restored || !plain.Ended || plain.Pixels != 0 {
		t.Fatal("incomplete plain-game menu proof")
	}
}

func TestRecordedPauseOverlaySceneEvidence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "pause-overlay-scenes-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Schema int `json:"schemaVersion"`
		Pixels int `json:"differingPixels"`
		Scenes map[string]struct {
			Background bool              `json:"backgroundUnchanged"`
			Stable     bool              `json:"pausedSceneStable"`
			Restored   bool              `json:"startRestoresPixels"`
			Pixels     int               `json:"differingPixels"`
			Hashes     map[string]string `json:"romSha256"`
		} `json:"scenes"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != 1 || report.Pixels != 0 {
		t.Fatal("missing overlay pixel proof")
	}
	for _, name := range []string{"tidewalk", "lantern-rooms", "127-objects"} {
		scene, ok := report.Scenes[name]
		if !ok || !scene.Background || !scene.Stable || !scene.Restored || scene.Pixels != 0 {
			t.Fatalf("incomplete scene evidence: %s %+v", name, scene)
		}
		for _, renderer := range []string{"mode0", "mode4", "auto"} {
			if len(scene.Hashes[renderer]) != 64 {
				t.Fatalf("missing %s ROM for %s", renderer, name)
			}
		}
		if scene.Hashes["auto"] != scene.Hashes["mode0"] {
			t.Fatal("AUTO scene is not the forced Mode 0 ROM")
		}
	}
}
