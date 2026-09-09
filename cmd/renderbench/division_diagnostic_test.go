//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
	rt "github.com/drpaneas/mtile/runtime"
)

// TestDivisionTargetDiagnostic checks actual ARM arithmetic against the original
// widening/floor-remainder formulas, independently of runtime fast paths. Run
// the same test in a baseline checkout to compare its JSON cycle measurements.
//
//	MTILE_DIVISION_OUT=/tmp/division go test ./cmd/renderbench \
//	  -tags=runtimeperfdiagnostic -run '^TestDivisionTargetDiagnostic$' -v
func TestDivisionTargetDiagnostic(t *testing.T) {
	helper := os.Getenv("MTILE_DIVISION_PLAYER")
	if helper == "" {
		helper = filepath.Join("..", "..", "build", "player", "mtile-player")
	}
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build the headless mtile-player or set MTILE_DIVISION_PLAYER")
	}
	out := os.Getenv("MTILE_DIVISION_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	type measurement struct {
		Workload     string `json:"workload"`
		Renderer     string `json:"renderer"`
		Median       uint32 `json:"median"`
		P95          uint32 `json:"p95"`
		Maximum      uint32 `json:"maximum"`
		SteadyMissed uint32 `json:"steadyMissed"`
		ROMBytes     int    `json:"romBytes"`
		IWRAMBytes   uint64 `json:"iwramBytes"`
	}
	var report []measurement
	for _, workload := range []struct {
		name, divisor string
		raw           int32
	}{
		{"dynamic", "i + 0.625", 0},
		{"literal_large", "1.7", 111411},
		{"literal_small", "0.6", 39322},
	} {
		t.Run(workload.name, func(t *testing.T) {
			project := filepath.Join(t.TempDir(), "project")
			if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
				t.Fatal(err)
			}
			source := fmt.Sprintf(`numerator = 1
quotient = 0
remainder = 0
function _update()
 quotient = 0
 remainder = 0
 for i = 1,16 do
  numerator = (numerator + 73.25) %% 4096 - 2048
  quotient = quotient + numerator / (%s)
  remainder = remainder + numerator %% (%s)
 end
end
function _draw() cls(0) end
`, workload.divisor, workload.divisor)
			if err := os.WriteFile(filepath.Join(project, build.GameFileName), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			var reference capture
			for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
				t.Run(renderer.String(), func(t *testing.T) {
					service, err := build.NewService("", "")
					if err != nil {
						t.Fatal(err)
					}
					service.Debug = true
					dir := filepath.Join(out, workload.name, renderer.String())
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					result, captured, err := buildAndCaptureAtFrames(service, renderer, project, helper, dir, 10, 120, reference.frameNumbers)
					if err != nil {
						t.Fatal(err)
					}
					numerator := rt.FromInt(1)
					frame := uint32(0)
					for sample, target := range captured.frameNumbers {
						var quotient, remainder rt.Number
						for frame < target {
							quotient, remainder = 0, 0
							for i := 1; i <= 16; i++ {
								numerator = divisionReferenceMod(numerator+rt.Number(4800512), rt.FromInt(4096)) - rt.FromInt(2048)
								divisor := rt.Number(workload.raw)
								if divisor == 0 {
									divisor = rt.FromInt(i) + rt.Number(40960)
								}
								quotient += rt.Number((int64(numerator) << 16) / int64(divisor))
								remainder += divisionReferenceMod(numerator, divisor)
							}
							frame++
						}
						for name, want := range map[string]rt.Number{"numerator": numerator, "quotient": quotient, "remainder": remainder} {
							found := false
							for _, watch := range captured.watches[sample] {
								if watch.Name != name {
									continue
								}
								found = true
								if watch.Kind != byte(rt.KindNumber) || watch.Raw != int32(want) {
									t.Fatalf("frame %d %s kind=%d raw=%d want=%d", target, name, watch.Kind, watch.Raw, want)
								}
							}
							if !found {
								t.Fatalf("missing %s watch", name)
							}
						}
					}
					if len(reference.frames) == 0 {
						reference = captured
					} else if differing := compareFrames(reference.frames, captured.frames); differing != 0 {
						t.Fatalf("renderer parity: %d differing pixels", differing)
					}
					row := measurement{workload.name, renderer.String(), percentile(captured.cycles, 50), percentile(captured.cycles, 95), percentile(captured.cycles, 100), captured.missed - captured.warmupMissed, result.ROMSize, result.IWRAMUsed}
					report = append(report, row)
					t.Logf("%+v", row)
					if row.SteadyMissed != 0 {
						t.Error("division workload missed a steady-state frame")
					}
				})
			}
		})
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "division.json"), append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log("evidence:", out)
}

func divisionReferenceMod(a, b rt.Number) rt.Number {
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		r += b
	}
	return r
}
