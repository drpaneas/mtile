package transpiler

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func mode0BudgetTestSource(calls ...string) string {
	return "function _draw()\n  cls()\n  " + strings.Join(calls, "\n  ") + "\nend\n"
}

func mode0BudgetRepeatedCall(count int, call string) []string {
	calls := make([]string, count)
	for index := range calls {
		calls[index] = call
	}
	return calls
}

func requireMode0BudgetReport(t *testing.T, name, source string) RendererReport {
	t.Helper()
	_, report, err := TranspileWithReport(name, source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestMode0ObjectBudgetAcceptsPackedExactOAMBoundary(t *testing.T) {
	calls := mode0BudgetRepeatedCall(126, "spr(1,0,0)")
	calls = append(calls, "spr(32,0,0,2,2)")
	report := requireMode0BudgetReport(t, "packed-oam-128.lua", mode0BudgetTestSource(calls...))
	if !report.Mode0ObjectPacking || report.Mode0OAMEntries != 127 || report.Mode0OBJLineCycles != 1024 {
		t.Fatalf("budget = %+v, want packed OAM=127 and line cycles=1024", report)
	}
}

func TestMode0ObjectBudgetRejectsPackedOAMBoundaryPlusOne(t *testing.T) {
	calls := mode0BudgetRepeatedCall(127, "spr(1,0,0)")
	calls = append(calls, "spr(32,0,0,2,2)")
	_, _, err := TranspileWithReport("packed-oam-129.lua", mode0BudgetTestSource(calls...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_OBJ_BUDGET]") || !strings.Contains(err.Error(), "128 OAM entries") {
		t.Fatalf("error = %v, want stable 128-entry diagnostic", err)
	}
}

func TestMode0ObjectBudgetSeparatesStaticSpriteBands(t *testing.T) {
	calls := make([]string, 0, 100)
	calls = append(calls, mode0BudgetRepeatedCall(50, "spr(32,0,0,2,2)")...)
	calls = append(calls, mode0BudgetRepeatedCall(50, "spr(32,0,16,2,2)")...)
	report := requireMode0BudgetReport(t, "static-bands.lua", mode0BudgetTestSource(calls...))
	if report.Mode0OAMEntries != 100 || report.Mode0OBJLineCycles != 800 {
		t.Fatalf("budget = %+v, want OAM=100 and independently composed 800-cycle bands", report)
	}
}

func TestMode0ObjectBudgetKeepsDynamicCameraAsSharedTranslation(t *testing.T) {
	calls := []string{"camera(0,t())"}
	calls = append(calls, mode0BudgetRepeatedCall(36, "spr(32,0,64,4,4)")...)
	report := requireMode0BudgetReport(t, "dynamic-camera.lua", mode0BudgetTestSource(calls...))
	if report.Mode0OAMEntries != 36 || report.Mode0OBJLineCycles != 1152 {
		t.Fatalf("budget = %+v, want camera-invariant OAM=36 and line cycles=1152", report)
	}
}

func TestMode0ObjectBudgetUsesRuntimeFixedPointForFractionalY(t *testing.T) {
	calls := mode0BudgetRepeatedCall(36, "spr(32,0,-0.000001,4,4)")
	calls = append(calls, "spr(1,0,31)", "spr(1,8,31)")
	_, _, err := TranspileWithReport("fractional-y.lua", mode0BudgetTestSource(calls...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1168 rendering cycles") {
		t.Fatalf("error = %v, want fixed-point-rounded Y rejected at 1168 cycles", err)
	}
}

func TestMode0ObjectBudgetPreservesRepresentableNegativeFractionalY(t *testing.T) {
	report := requireMode0BudgetReport(t, "representable-fractional-y.lua", mode0BudgetTestSource(
		"spr(1,0,-0.5)",
		"spr(1,0,7)",
	))
	if report.Mode0OAMEntries != 2 || report.Mode0OBJLineCycles != 8 {
		t.Fatalf("budget = %+v, want runtime -0.5 -> -1 with two disjoint 8-cycle bands", report)
	}
}

func TestMode0ObjectBudgetCombinesStaticAndDynamicProfiles(t *testing.T) {
	accepted := mode0BudgetRepeatedCall(36, "spr(32,0,0,4,4)")
	accepted = append(accepted, "spr(1,0,py)")
	report := requireMode0BudgetReport(t, "static-dynamic-accepted.lua", mode0BudgetTestSource(accepted...))
	if report.Mode0OAMEntries != 37 || report.Mode0OBJLineCycles != 1160 {
		t.Fatalf("accepted budget = %+v, want OAM=37 and line cycles=1160", report)
	}

	rejected := append(append([]string(nil), accepted...), "spr(1,0,other_y)")
	_, _, err := TranspileWithReport("static-dynamic-rejected.lua", mode0BudgetTestSource(rejected...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1168 rendering cycles") {
		t.Fatalf("error = %v, want conservative 1168-cycle rejection", err)
	}
}

func TestMode0ObjectBudgetAccountsForPackedCacheExhaustion(t *testing.T) {
	calls := make([]string, 0, 9)
	for index := 0; index < 9; index++ {
		calls = append(calls, fmt.Sprintf("spr(%d,0,0,2,2)", 32+index*2))
	}
	report := requireMode0BudgetReport(t, "packed-cache-full.lua", mode0BudgetTestSource(calls...))
	if report.Mode0OAMEntries != 12 || report.Mode0OBJLineCycles != 144 {
		t.Fatalf("budget = %+v, want eight packed entries plus four-entry fallback", report)
	}
}

func TestMode0ObjectBudgetFallsBackWhenSheetIsMutable(t *testing.T) {
	source := `
function _update()
  sset(0,0,1)
end
function _draw()
  cls()
  spr(32,0,0,4,4)
end
`
	report := requireMode0BudgetReport(t, "mutable-sheet.lua", source)
	if report.Mode0ObjectPacking || report.Mode0OAMEntries != 16 || report.Mode0OBJLineCycles != 32 {
		t.Fatalf("budget = %+v, want unpacked 16-entry mutable-sheet fallback", report)
	}
}

func TestMode0ObjectBudgetCombinesTextAndSpriteEntries(t *testing.T) {
	calls := []string{fmt.Sprintf("print(%q,0,0,7)", strings.Repeat("A", 32))}
	calls = append(calls, mode0BudgetRepeatedCall(95, "spr(1,0,0)")...)
	report := requireMode0BudgetReport(t, "text-sprite-128.lua", mode0BudgetTestSource(calls...))
	if !report.Mode0Text || report.Mode0OAMEntries != 127 || report.Mode0OBJLineCycles != 1016 {
		t.Fatalf("budget = %+v, want text+sprites OAM=127 and line cycles=1016", report)
	}
}

func TestMode0ObjectBudgetAccountsForRuntimeTextWrapping(t *testing.T) {
	calls := mode0BudgetRepeatedCall(36, "spr(32,0,32,4,4)")
	calls = append(calls, fmt.Sprintf("print(%q,228,0,7)", strings.Repeat("A", 32)))
	_, _, err := TranspileWithReport("wrapped-text.lua", mode0BudgetTestSource(calls...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1168 rendering cycles") {
		t.Fatalf("error = %v, want wrapped glyphs included in the 1168-cycle scanline", err)
	}
}

func TestMode0ObjectBudgetRejectsCameraRebaseBetweenObjectBands(t *testing.T) {
	calls := mode0BudgetRepeatedCall(36, "spr(32,0,0,4,4)")
	calls = append(calls, "camera(0,32)", "spr(1,0,32)", "spr(1,8,32)")
	_, _, err := TranspileWithReport("camera-rebase.lua", mode0BudgetTestSource(calls...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1168 rendering cycles") {
		t.Fatalf("error = %v, want camera-rebased bands rejected at 1168 cycles", err)
	}
}

func TestMode0ObjectBudgetRejectsCameraRebaseThroughHelper(t *testing.T) {
	calls := mode0BudgetRepeatedCall(36, "spr(32,0,0,4,4)")
	calls = append(calls, "rebase()", "spr(1,0,32)", "spr(1,8,32)")
	source := "function rebase() camera(0,32) end\n" + mode0BudgetTestSource(calls...)
	_, _, err := TranspileWithReport("camera-helper-rebase.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1168 rendering cycles") {
		t.Fatalf("error = %v, want helper camera-rebased bands rejected at 1168 cycles", err)
	}
}

func TestMode0ObjectBudgetIncludesCameraRebasesInLoops(t *testing.T) {
	const helper = "function rebase() camera(0,32) end\n"
	const firstBand = "for i=1,36 do spr(32,0,0,4,4) end"
	const secondBand = "for i=1,36 do spr(32,0,32,4,4) end"
	for _, tc := range []struct{ name, loop string }{
		{"while", "while btn(0) do rebase() break end"},
		{"repeat", "repeat camera(0,32) until true"},
		{"numeric-for", "for i=1,flr(t()) do camera(0,32) end"},
		{"generic-for", "for v in all({1}) do camera(0,32) end"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := helper + mode0BudgetTestSource(firstBand, tc.loop, secondBand)
			requireMode0ObjectBudgetFallback(t, tc.name+".lua", source, "2304 rendering cycles")
		})
	}

	for _, tc := range []struct{ name, source string }{
		{"loop-before-objects", helper + mode0BudgetTestSource("while btn(0) do rebase() break end", firstBand, secondBand)},
		{"empty-loop", mode0BudgetTestSource(firstBand, "for i=1,0 do camera(0,32) end", secondBand)},
	} {
		report := requireMode0BudgetReport(t, tc.name+".lua", tc.source)
		if report.Mode0OAMEntries != 72 || report.Mode0OBJLineCycles != 1152 {
			t.Fatalf("%s: budget = %+v, want shared camera translation of both bands", tc.name, report)
		}
	}
}

func TestMode0LoopCameraTraceOnlyRecordsPossibleRebase(t *testing.T) {
	const helper = "function rebase() camera(8,32) return true end\n"
	for _, loop := range []string{
		"while btn(0) do rebase() break end",
		"repeat if btn(0) then break end until rebase()",
	} {
		chunk, err := luaparser.ParseString("loop-camera-text.lua", helper+mode0BudgetTestSource("camera()", loop, `print("A",8,32,7)`))
		if err != nil {
			t.Fatal(err)
		}
		draw := chunk.Body[1].(*luaparser.FunctionDeclStmt)
		ops, unbounded, err := mode0WalkDraw(chunk, draw, newMode0SpriteIDAnalysis(chunk), Options{})
		if err != nil || unbounded {
			t.Fatalf("%s: unbounded object count %t, error %v", loop, unbounded, err)
		}
		if len(ops) < 3 || ops[len(ops)-1].name != "print" {
			t.Fatalf("%s: missing loop and print trace", loop)
		}
		change := ops[len(ops)-2]
		want := mode0DrawOp{name: "camera", call: change.call, copies: change.copies}
		if change.call == nil || len(change.call.Args) != 2 || !reflect.DeepEqual(change, want) {
			t.Fatalf("%s: want a possible camera rebase before print, got %+v", loop, change)
		}
	}
}

func TestMode0CallGraphUsesGeneratorBuiltinPrecedence(t *testing.T) {
	calls := mode0BudgetRepeatedCall(36, "spr(32,0,0,4,4)")
	calls = append(calls, "abs(1)", "spr(1,0,32)", "spr(1,8,32)")
	source := "function abs(n) camera(0,32) end\n" + mode0BudgetTestSource(calls...)
	_, report, err := TranspileWithReport("builtin-precedence-budget.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatalf("builtin call followed the unreachable user body: %v", err)
	}
	if report.Selected != RendererMode0 || report.Mode0OBJLineCycles != 1152 {
		t.Fatalf("Mode 0 report = %+v, want builtin abs() resolution and a 1152-cycle static band", report)
	}
}

func TestMode0ObjectBudgetAutoFallsBackOnScanlineOverflow(t *testing.T) {
	source := mode0BudgetTestSource(mode0BudgetRepeatedCall(37, "spr(32,0,64,4,4)")...)
	_, report, err := TranspileWithReport("auto-scanline-overflow.lua", source, Options{
		Renderer: RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0Ineligible ||
		!strings.Contains(report.Diagnostic, "[M0_OBJ_BUDGET]") || !strings.Contains(report.Diagnostic, "1184 rendering cycles") {
		t.Fatalf("AUTO report = %+v, want stable scanline-overflow fallback", report)
	}
}

func TestMode0ObjectBudgetDoesNotDiscountHorizontallyOffscreenObjects(t *testing.T) {
	source := mode0BudgetTestSource(mode0BudgetRepeatedCall(37, "spr(32,10000,64,4,4)")...)
	_, _, err := TranspileWithReport("offscreen-x.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1184 rendering cycles") {
		t.Fatalf("error = %v, want offscreen objects included in scanline budget", err)
	}
}

func TestMode0ObjectBudgetReportsZeroForObjectFreeDraw(t *testing.T) {
	report := requireMode0BudgetReport(t, "no-objects.lua", mode0BudgetTestSource())
	if report.Mode0Objects || report.Mode0ObjectPacking || report.Mode0OAMEntries != 0 || report.Mode0OBJLineCycles != 0 {
		t.Fatalf("object-free report = %+v, want zero budget", report)
	}
}

func TestMode0ObjectBudgetUsesWorstFiniteSpriteSize(t *testing.T) {
	source := `
id=1
size=1
function _update()
  if btn(0) then size=2 else size=1 end
end
function _draw()
  cls()
  spr(id,0,0,size,size)
end
`
	_, report, err := TranspileWithReport("finite-spr-size.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 4 || report.Mode0OBJLineCycles != 16 {
		t.Fatalf("report = %+v, want 2x2 worst-case OAM=4 and 16 line cycles", report)
	}
}

func TestMode0RejectsUnprovenObjectLoop(t *testing.T) {
	source := `
sprites={{id=1,x=0,y=0}}
function _draw()
  cls()
  for s in all(sprites) do
    spr(s.id,s.x,s.y)
  end
end
`
	_, _, err := TranspileWithReport("unproven-loop.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_OBJ_BUDGET]") || !strings.Contains(err.Error(), "unproven object count") {
		t.Fatalf("error = %v, want unproven loop rejected", err)
	}
	_, report, err := TranspileWithReport("unproven-loop-auto.lua", source, Options{
		Renderer: RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "[M0_OBJ_BUDGET]") {
		t.Fatalf("AUTO report = %+v, want Mode 4 fallback", report)
	}
}

func TestMode0ObjectBudgetProvesEveryNumericLoopBound(t *testing.T) {
	for _, tc := range []struct{ name, setup, bounds, diagnostic string }{
		{"mixed-count", "local n=1 if btn(0) then n=129 end", "1,n", "unproven object count"},
		{"mixed-wrap", "local n=32766 if btn(0) then n=32767 end", "32766,n", "unproven object count"},
		{"positive-wrap", "", "32767,32767", "unproven object count"},
		{"negative-wrap", "", "-32768,-32768,-1", "unproven object count"},
		{"literal-count", "", "1,129", "129 OAM entries"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := mode0BudgetTestSource(tc.setup, "for i="+tc.bounds+" do spr(1,0,0) end")
			requireMode0ObjectBudgetFallback(t, tc.name+".lua", source, tc.diagnostic)
		})
	}

	source := mode0BudgetTestSource("local n=0 if btn(0) then n=1 end", "for i=1,n do spr(1,0,0) end")
	report := requireMode0BudgetReport(t, "empty-loop-alternative.lua", source)
	if report.Mode0OAMEntries != 1 {
		t.Fatalf("budget = %+v, want one entry with a valid zero-trip alternative", report)
	}
}

func requireMode0ObjectBudgetFallback(t *testing.T, name, source, detail string) {
	t.Helper()
	opts := Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160}
	_, _, err := TranspileWithReport(name, source, opts)
	if err == nil || !strings.Contains(err.Error(), name+":") || !strings.Contains(err.Error(), "[M0_OBJ_BUDGET]") || !strings.Contains(err.Error(), detail) {
		t.Fatalf("forced Mode 0 error = %v, want source-positioned M0_OBJ_BUDGET with %q", err, detail)
	}

	opts.Renderer = RendererAuto
	_, report, err := TranspileWithReport(name, source, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0Ineligible || !strings.Contains(report.Diagnostic, name+":") ||
		!strings.Contains(report.Diagnostic, "[M0_OBJ_BUDGET]") || !strings.Contains(report.Diagnostic, detail) {
		t.Fatalf("AUTO report = %+v, want Mode 4 fallback with source-positioned M0_OBJ_BUDGET and %q", report, detail)
	}
}
