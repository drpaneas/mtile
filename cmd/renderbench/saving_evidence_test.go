package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRecordedSaveEvidence(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "saving-slots-v2")
	requireRecordedBenchmarkParity(t, evidence)
	if evidence.Mode0.P95Cycles >= twoFrameCycleBudget || evidence.Mode4.P95Cycles >= twoFrameCycleBudget || evidence.Mode0.MissedFrames != 0 || evidence.Mode4.MissedFrames != 0 {
		t.Fatal("save stress workload must fit the update budget without missed frames")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "saving-slots-oracle-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		SchemaVersion   int   `json:"schemaVersion"`
		CapturedFrames  int   `json:"capturedFrames"`
		DifferingPixels int   `json:"differingPixels"`
		ZeroSaveErrors  bool  `json:"zeroSaveErrors"`
		CheckedSlots    []int `json:"checkedSlots"`
		CallShapes      bool  `json:"defaultLiteralAndDynamicSlots"`
		InvalidSlots    bool  `json:"invalidSlotsRejected"`
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
	if !slices.Equal(oracle.CheckedSlots, []int{0, 1, 2, 31}) || !oracle.CallShapes || !oracle.InvalidSlots {
		t.Fatal("missing slot API shape proof")
	}
}

func TestRecordedPauseOnlyEvidence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "pause-only-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Schema          int  `json:"schemaVersion"`
		Frames          int  `json:"capturedFrames"`
		Pixels          int  `json:"differingPixels"`
		NoInput         bool `json:"worksWithoutGameInput"`
		DefaultDisabled bool `json:"defaultDisabled"`
		EnableShapes    bool `json:"literalAndDynamicEnable"`
		DisabledAgain   bool `json:"disabledAgain"`
		Ignored         bool `json:"otherButtonsIgnored"`
		Restored        bool `json:"resumeRestoresPixels"`
		Renderers       map[string]struct {
			SHA     string `json:"romSha256"`
			Samples int    `json:"samples"`
			Median  uint32 `json:"medianPauseCycles"`
			P95     uint32 `json:"p95PauseCycles"`
		} `json:"renderers"`
	}
	if err := json.Unmarshal(data, &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Schema != 2 || proof.Frames < 120 || proof.Pixels != 0 || !proof.NoInput || !proof.DefaultDisabled || !proof.EnableShapes || !proof.DisabledAgain || !proof.Ignored || !proof.Restored {
		t.Fatal("incomplete pause-only proof")
	}
	for _, name := range []string{"mode0", "mode4", "auto"} {
		r, ok := proof.Renderers[name]
		if !ok || len(r.SHA) != 64 || r.Samples < 120 || r.Median == 0 || r.P95 >= twoFrameCycleBudget {
			t.Fatal("missing pause timing", name)
		}
	}
	if proof.Renderers["auto"].SHA != proof.Renderers["mode0"].SHA {
		t.Fatal("AUTO differs from Mode 0")
	}
}

func TestRecordedThreeFileExampleEvidence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "saving-example-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Schema       int               `json:"schemaVersion"`
		Names        int               `json:"namedFiles"`
		Pixels       int               `json:"differingPixels"`
		Independent  bool              `json:"independentSlots"`
		Held         bool              `json:"heldASavesOnce"`
		Wait         bool              `json:"waitMessageExpires"`
		Reset        bool              `json:"resetRestoresAllSlots"`
		Load         bool              `json:"loadRestores"`
		Quit         bool              `json:"saveAndQuit"`
		Paused       bool              `json:"pauseFreezesGame"`
		NoActions    bool              `json:"pauseHasNoActions"`
		GameplayOnly bool              `json:"pauseOnlyInGameplay"`
		MenuStart    bool              `json:"menuStartWorks"`
		HeldStart    bool              `json:"heldStartDoesNotPause"`
		Hashes       map[string]string `json:"romSha256"`
	}
	if err := json.Unmarshal(data, &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Schema != 2 || proof.Names != 3 || proof.Pixels != 0 || !proof.Independent || !proof.Held || !proof.Wait || !proof.Reset || !proof.Load || !proof.Quit || !proof.Paused || !proof.NoActions || !proof.GameplayOnly || !proof.MenuStart || !proof.HeldStart || len(proof.Hashes["mode4"]) != 64 || proof.Hashes["auto"] != proof.Hashes["mode4"] {
		t.Fatal("incomplete three-file proof")
	}
}

func TestRecordedPauseOverlaySceneEvidence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "pause-overlay-scenes-v2.json"))
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
