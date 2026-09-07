//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestRuntimeGCCollectDiagnostic compares an unchanged native workload using
// one frozen runtime snapshot and a copy differing only in heap.go.
// MTILE_PERF_BASELINE_RUNTIME=/path/to/runtime go test ./cmd/renderbench -tags=runtimeperfdiagnostic -run TestRuntimeGCCollectDiagnostic -v
func TestRuntimeGCCollectDiagnostic(t *testing.T) {
	baseline := os.Getenv("MTILE_PERF_BASELINE_RUNTIME")
	if baseline == "" {
		t.Skip("set MTILE_PERF_BASELINE_RUNTIME to the runtime before GC changes")
	}
	out, err := os.MkdirTemp("", "celeste-gc-probe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("evidence:", out)
	candidate := filepath.Join(out, "runtime")
	err = filepath.WalkDir(baseline, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(baseline, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(candidate, rel), 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(candidate, rel), data, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
	heapSource := os.Getenv("MTILE_PERF_GC_HEAP")
	if heapSource == "" {
		heapSource = filepath.Join("..", "..", "runtime", "heap.go")
	}
	heap, err := os.ReadFile(heapSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "heap.go"), heap, 0644); err != nil {
		t.Fatal(err)
	}
	source := `checksum=0
tick=0
live={}
prefixes={"","x","xx","xxx"}
function _init()
 for i=1,60 do add(live,{x=i,y=i+1,vx=i/256,label=prefixes[(i%4)+1]..i}) end
end
function _update()
 tick=tick+1
 local item=live[(tick%60)+1]
 item.x=item.x+0.25
 checksum=checksum+item.x+item.y+#live+#item.label
 for i=1,10 do local trash={tick+i,tick-i,live[(i%60)+1]} end
end
function _draw() cls(0) end
`
	sourcePath := filepath.Join(out, "workload.lua")
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	project, err := stageProjectWithSource(filepath.Join("..", "..", "examples", "hello"), sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Frame, GC, Allocated, Live, Handles uint32
		Watches                             []emulator.Watch
	}
	var beforeRows []row
	var beforeGC []uint32
	var beforeGo string
	for index, runtimeDir := range []string{baseline, candidate} {
		service, err := build.NewService("", runtimeDir)
		if err != nil {
			t.Fatal(err)
		}
		service.Debug, service.KeepWork = true, true
		service.Renderer = transpiler.RendererMode4
		rom := filepath.Join(out, []string{"before.gba", "after.gba"}[index])
		result, err := service.Build(project, rom)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			beforeGo = result.GeneratedGo
		} else if result.GeneratedGo != beforeGo {
			t.Fatal("GC comparison changed generated Go")
		}
		session, err := emulator.Start(filepath.Join("..", "..", "build", "player", "mtile-player"), rom)
		if err != nil {
			t.Fatal(err)
		}
		var rows []row
		var collections []uint32
		var lastFrame, lastGC uint32
		for step := 0; step < 10000 && len(rows) < 300; step++ {
			frame, err := session.Step(0, false)
			if err != nil {
				t.Fatal(err)
			}
			if !frame.Telemetry.DebugValid {
				continue
			}
			stats := frame.Telemetry.Stats
			if stats.HeapError != emulator.HeapOK {
				t.Fatalf("native heap error: %+v", stats)
			}
			if stats.Frames == 0 || stats.Frames == lastFrame {
				continue
			}
			if stats.Frames != uint32(len(rows)+1) {
				t.Fatalf("skipped runtime frame: got%d want%d", stats.Frames, len(rows)+1)
			}
			lastFrame = stats.Frames
			if stats.GCCount != lastGC {
				if stats.GCCount != lastGC+1 {
					t.Fatal("multiple collections hidden in one sample")
				}
				collections = append(collections, stats.LastGC)
				lastGC = stats.GCCount
			}
			rows = append(rows, row{stats.Frames, stats.GCCount, stats.HeapAllocated, stats.HeapLive, stats.HeapHandles, append([]emulator.Watch(nil), frame.Telemetry.Watches...)})
		}
		session.Close()
		if len(rows) != 300 || len(collections) < 3 {
			t.Fatalf("incomplete GC evidence: %d ticks %d collections", len(rows), len(collections))
		}
		data, _ := json.MarshalIndent(struct {
			Rows   []row
			Cycles []uint32
		}{rows, collections}, "", "  ")
		if err := os.WriteFile(filepath.Join(out, []string{"before.json", "after.json"}[index]), data, 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("variant%d GCcycles=%v live=%d handles=%d ROM=%d IWRAM=%d EWRAM=%d work=%s", index, collections, rows[len(rows)-1].Live, rows[len(rows)-1].Handles, result.ROMSize, result.IWRAMUsed, result.EWRAMUsed, result.WorkDir)
		if index == 0 {
			beforeRows, beforeGC = rows, collections
		} else {
			if !reflect.DeepEqual(beforeRows, rows) {
				t.Fatal("collection schedule, heap state or checksum watches differ")
			}
			for i, cycles := range collections {
				if cycles >= beforeGC[i] {
					t.Errorf("collection%d did not improve: %d >=%d", i, cycles, beforeGC[i])
				}
			}
		}
	}
}
