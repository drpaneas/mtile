//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// The dynamic substring lengths cover every aligned string payload from 4 to
// 32 bytes. Dynamic negative keys keep the 44-byte tables on NewTableWithCaps,
// rather than the prebuilt-image path. Twelve nested roots survive collections.
const sizeAllocSmallSource = `tick=0
checksum=0
roots={}
alphabet="abcdefghijklmnopqrstuvwxyz0123456789"
function _init()
 for i=1,12 do roots[i]={[-i]=sub(alphabet,1,i)} end
end
function _update()
 tick=tick+1
 checksum=0
 for i=1,12 do
  local slot=((tick+i)%12)+1
  local old=roots[slot][-slot]
  checksum=checksum+#old+ord(old,1)
  local n=((tick+i)%30)+1
  local start=((tick+i)%7)+1
  local label=sub(alphabet,start,start+n-1)
  roots[slot]={[-slot]=label}
  checksum=checksum+#label
 end
end
function _draw()
 cls(0)
 spr(1,checksum%232,60)
end
`

// Larger strings exercise the generic allocator branch after the small-size
// dispatch. Retained roots and changing checksums keep allocation observable.
const sizeAllocLargeSource = `tick=0
checksum=0
roots={}
alphabet="abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789"
function _init()
 for i=1,12 do roots[i]=sub(alphabet,1,64+i) end
end
function _update()
 tick=tick+1
 checksum=0
 for i=1,12 do
  local slot=((tick+i)%12)+1
  checksum=checksum+#roots[slot]+ord(roots[slot],1)
  local n=65+((tick+i)%64)
  local start=((tick+i)%7)+1
  roots[slot]=sub(alphabet,start,start+n-1)
  checksum=checksum+#roots[slot]
 end
end
function _draw()
 cls(0)
 spr(1,checksum%232,60)
end
`

func TestSizeSpecializedAllocationWorkloads(t *testing.T) {
	for name, source := range map[string]string{"small": sizeAllocSmallSource, "large": sizeAllocLargeSource} {
		generated, err := transpiler.Transpile(name+".lua", source, transpiler.Options{})
		if err != nil {
			t.Fatal(err)
		}
		checkSizeAllocWorkload(t, name, generated)
	}
}

func checkSizeAllocWorkload(t *testing.T, name, generated string) {
	t.Helper()
	if name != "small" && name != "large" {
		return
	}
	_, update, ok := strings.Cut(generated, "func (g *Game) gluUpdate(")
	if !ok {
		t.Fatal("generated workload has no update function")
	}
	update, _, _ = strings.Cut(update, "\nfunc ")
	if !strings.Contains(update, "gluSub(c,") {
		t.Fatal("workload must allocate dynamic strings through Context.Sub")
	}
	if name == "small" && (!strings.Contains(update, "c.NewTableWithCaps(1, 1)") || strings.Contains(update, "NewTableFromImage(")) {
		t.Fatal("small workload must allocate ordinary dynamic-key tables")
	}
}

// TestSizeSpecializedAllocationDiagnostic compares two frozen runtimes with
// identical generated code. Set MTILE_SIZE_ALLOC_BASELINE_RUNTIME,
// MTILE_SIZE_ALLOC_CANDIDATE_RUNTIME and MTILE_SIZE_ALLOC_OUT. Timing changes
// are reported, not gated: the experiment's keep/drop decision needs the full
// benefit, code-size and regression picture. Eight pairs require sixteen builds.
func TestSizeSpecializedAllocationDiagnostic(t *testing.T) {
	before, after, out := os.Getenv("MTILE_SIZE_ALLOC_BASELINE_RUNTIME"), os.Getenv("MTILE_SIZE_ALLOC_CANDIDATE_RUNTIME"), os.Getenv("MTILE_SIZE_ALLOC_OUT")
	if before == "" || after == "" || out == "" {
		t.Skip("set MTILE_SIZE_ALLOC_BASELINE_RUNTIME, MTILE_SIZE_ALLOC_CANDIDATE_RUNTIME and MTILE_SIZE_ALLOC_OUT")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	leftService, err := build.NewService("", before)
	if err != nil {
		t.Fatal(err)
	}
	rightService, err := build.NewService("", after)
	if err != nil {
		t.Fatal(err)
	}
	leftService.Debug, rightService.Debug = true, true
	leftService.KeepWork, rightService.KeepWork = true, true
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")
	metadata := map[string]any{"dateUTC": time.Now().UTC().Format(time.RFC3339)}
	for name, path := range map[string]string{"baselineRuntimeSHA256": before, "candidateRuntimeSHA256": after} {
		metadata[name], err = projectFingerprint(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	metadata["tinygo"], err = commandOutput(leftService.TinyGoBin, "version")
	if err != nil {
		t.Fatal(err)
	}
	metadata["go"], err = commandOutput("go", "version")
	if err != nil {
		t.Fatal(err)
	}
	metadata["playerSHA256"], err = fileFingerprint(helper)
	if err != nil {
		t.Fatal(err)
	}
	writeSizeAllocJSON(t, filepath.Join(out, "metadata.json"), metadata)
	for _, workload := range []struct{ name, source, fixture, project string }{
		{"small", sizeAllocSmallSource, "", "hello"},
		{"large", sizeAllocLargeSource, "", "hello"},
		{"hash", "", "numeric_hash_keys.lua", "hello"},
		{"camera", "", "camera.lua", "testio"},
	} {
		for _, mode := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
			t.Run(workload.name+"-"+mode.String(), func(t *testing.T) {
				dir := filepath.Join(out, workload.name+"-"+mode.String())
				for _, variant := range []string{"baseline", "candidate"} {
					if err := os.MkdirAll(filepath.Join(dir, variant), 0755); err != nil {
						t.Fatal(err)
					}
				}
				source := []byte(workload.source)
				if workload.fixture != "" {
					source, err = os.ReadFile(filepath.Join("testdata", "workloads", workload.fixture))
					if err != nil {
						t.Fatal(err)
					}
				}
				sourcePath := filepath.Join(dir, "workload.lua")
				if err := os.WriteFile(sourcePath, source, 0644); err != nil {
					t.Fatal(err)
				}
				project, err := stageProjectWithSource(filepath.Join("..", "..", "examples", workload.project), sourcePath)
				if err != nil {
					t.Fatal(err)
				}
				defer os.RemoveAll(project)
				left, lc, err := buildAndCapture(leftService, mode, project, helper, filepath.Join(dir, "baseline"), 20, 300)
				if err != nil {
					t.Fatal(err)
				}
				right, rc, err := buildAndCaptureAtFrames(rightService, mode, project, helper, filepath.Join(dir, "candidate"), 20, 300, lc.frameNumbers)
				if err != nil {
					t.Fatal(err)
				}
				checkSizeAllocWorkload(t, workload.name, left.GeneratedGo)
				if left.GeneratedGo != right.GeneratedGo {
					t.Fatal("runtime comparison changed generated Go")
				}
				if err := os.WriteFile(filepath.Join(dir, "generated.go"), []byte(left.GeneratedGo), 0644); err != nil {
					t.Fatal(err)
				}
				if err := validateSynchronizedFrames(lc.frameNumbers, rc.frameNumbers); err != nil {
					t.Fatal(err)
				}
				lh := sizeAllocHeapTrace(t, helper, left.ROMPath, lc.frameNumbers)
				rh := sizeAllocHeapTrace(t, helper, right.ROMPath, rc.frameNumbers)
				diff := compareFrames(lc.frames, rc.frames)
				report := map[string]any{
					"sourceSHA256":    fmt.Sprintf("%x", sha256.Sum256(source)),
					"generatedSHA256": fmt.Sprintf("%x", sha256.Sum256([]byte(left.GeneratedGo))),
					"differingPixels": diff, "watchesEqual": reflect.DeepEqual(lc.watches, rc.watches),
					"heapTracesEqual": reflect.DeepEqual(lh, rh),
				}
				for _, v := range []struct {
					name string
					b    build.Result
					c    capture
					heap []sizeAllocHeapRow
				}{{"baseline", left, lc, lh}, {"candidate", right, rc, rh}} {
					romSHA, err := fileFingerprint(v.b.ROMPath)
					if err != nil {
						t.Fatal(err)
					}
					pixels := make([]string, len(v.c.frames))
					for i, frame := range v.c.frames {
						pixels[i] = fmt.Sprintf("%x", sha256.Sum256(frame))
					}
					report[v.name] = map[string]any{
						"cycles": v.c.cycles, "median": percentile(v.c.cycles, 50), "p95": percentile(v.c.cycles, 95), "max": percentile(v.c.cycles, 100),
						"drawCycles": v.c.draw, "commitCycles": v.c.commit, "frames": v.c.frameNumbers, "watches": v.c.watches, "rgbSHA256": pixels,
						"ROMBytes": v.b.ROMSize, "ROMSHA256": romSHA, "IWRAMBytes": v.b.IWRAMUsed, "EWRAMBytes": v.b.EWRAMUsed, "EWRAMFreeBytes": v.b.EWRAMFree,
						"heap": v.heap, "missedFrames": v.c.missed, "warmupMissedFrames": v.c.warmupMissed, "workDir": v.b.WorkDir,
					}
					t.Logf("%s median=%d p95=%d max=%d rom=%d iwram=%d ewram=%d heap=%d live=%d handles=%d gc=%d missed=%d", v.name,
						percentile(v.c.cycles, 50), percentile(v.c.cycles, 95), percentile(v.c.cycles, 100), v.b.ROMSize, v.b.IWRAMUsed, v.b.EWRAMUsed,
						v.c.heapAllocated, v.c.heapLive, v.c.heapHandles, v.c.gcCount, v.c.missed)
				}
				writeSizeAllocJSON(t, filepath.Join(dir, "report.json"), report)
				t.Logf("saving=%.2f%% differing_pixels=%d", 100*(1-float64(percentile(rc.cycles, 50))/float64(percentile(lc.cycles, 50))), diff)
				if diff != 0 || !reflect.DeepEqual(lc.watches, rc.watches) || !reflect.DeepEqual(lh, rh) {
					t.Fatal("pixel, watch or per-frame heap parity failed")
				}
				if (workload.name == "small" || workload.name == "large") && (lc.gcCount < 2 || lc.heapLive == 0 || lc.heapHandles == 0) {
					t.Fatal("allocation workload must retain roots across multiple collections")
				}
			})
		}
	}
}

type sizeAllocHeapRow struct {
	Frame, Allocated, Live, PeakLive, Handles, GC uint32
}

// Replay already-built ROMs because the common capture helper keeps only the
// final heap counters. Preserve every sample's state without changing its API.
func sizeAllocHeapTrace(t *testing.T, helper, rom string, targets []uint32) []sizeAllocHeapRow {
	t.Helper()
	session, err := emulator.Start(helper, rom)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var rows []sizeAllocHeapRow
	for step := 0; step < int(targets[len(targets)-1])*16+240 && len(rows) < len(targets); step++ {
		frame, err := session.Step(0, false)
		if err != nil {
			t.Fatal(err)
		}
		if !frame.Telemetry.DebugValid {
			continue
		}
		s := frame.Telemetry.Stats
		if s.HeapError != emulator.HeapOK {
			t.Fatalf("native heap error at frame %d: %v", s.Frames, s.HeapError)
		}
		if s.Frames < targets[len(rows)] {
			continue
		}
		if s.Frames > targets[len(rows)] {
			t.Fatalf("heap replay skipped frame %d, reached %d", targets[len(rows)], s.Frames)
		}
		rows = append(rows, sizeAllocHeapRow{s.Frames, s.HeapAllocated, s.HeapLive, s.HeapPeakLive, s.HeapHandles, s.GCCount})
	}
	if len(rows) != len(targets) {
		t.Fatalf("heap replay captured %d/%d frames", len(rows), len(targets))
	}
	return rows
}

func writeSizeAllocJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
