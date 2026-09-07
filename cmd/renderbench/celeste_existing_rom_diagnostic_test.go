//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Replay already-built diagnostic ROMs without rebuilding or changing their
// runtime, generated program, target, or assets. This keeps ISA and placement
// experiments isolated from concurrent compiler changes.
func TestCelesteExistingROMDiagnostic(t *testing.T) {
	paths := os.Getenv("MTILE_CELESTE_EXISTING_ROMS")
	if paths == "" {
		t.Skip("set MTILE_CELESTE_EXISTING_ROMS to comma-separated frozen diagnostic ROMs")
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	baseline := os.Getenv("MTILE_CELESTE_EXISTING_BASELINE")
	if baseline == "" {
		baseline = filepath.Join(repo, "build", "experiments", "mode0-general-optimizations", "final", "tiles-mode0-traces.json")
	}
	data, err := os.ReadFile(baseline)
	if err != nil {
		t.Fatal(err)
	}
	var before []tileCanvasRow
	if err := json.Unmarshal(data, &before); err != nil {
		t.Fatal(err)
	}
	out, err := os.MkdirTemp("", "celeste-existing-rom-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("evidence:", out)
	helper, err := tileCanvasPlayerWrapper(out, filepath.Join(repo, "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range strings.Split(paths, ",") {
		path = strings.TrimSpace(path)
		name := filepath.Base(filepath.Dir(path))
		t.Run(name, func(t *testing.T) {
			rows, err := tileCanvasCapture(helper, path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name+"-traces.json"), mustCelesteParallaxJSON(t, rows), 0644); err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(before) {
				t.Fatalf("publications %d/%d", len(rows), len(before))
			}
			for i, row := range rows {
				if row.Tick != before[i].Tick || row.RGBASHA != before[i].RGBASHA || !reflect.DeepEqual(tileCanvasGameplayWatches(row.Watches), tileCanvasGameplayWatches(before[i].Watches)) {
					t.Errorf("frozen gameplay/pixel mismatch tick %d", row.Tick)
				}
				if row.FirstSHA != row.RGBASHA || row.BlitStart < 160 || row.BlitEnd < row.BlitStart || row.BlitEnd >= 228 {
					t.Errorf("presentation mismatch tick %d: %d..%d", row.Tick, row.BlitStart, row.BlitEnd)
				}
			}
			tileCanvasReport(t, name, rows, 162)
			t.Logf("%d exact frozen gameplay/pixel comparisons: %s", len(rows), path)
		})
	}
}
