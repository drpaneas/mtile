package transpiler

import (
	"fmt"
	"strings"
	"testing"
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
	calls := mode0BudgetRepeatedCall(127, "spr(1,0,0)")
	calls = append(calls, "spr(32,0,0,2,2)")
	report := requireMode0BudgetReport(t, "packed-oam-128.lua", mode0BudgetTestSource(calls...))
	if !report.Mode0ObjectPacking || report.Mode0OAMEntries != 128 || report.Mode0OBJLineCycles != 1032 {
		t.Fatalf("budget = %+v, want packed OAM=128 and line cycles=1032", report)
	}
}

func TestMode0ObjectBudgetRejectsPackedOAMBoundaryPlusOne(t *testing.T) {
	calls := mode0BudgetRepeatedCall(128, "spr(1,0,0)")
	calls = append(calls, "spr(32,0,0,2,2)")
	_, _, err := TranspileWithReport("packed-oam-129.lua", mode0BudgetTestSource(calls...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_OBJ_BUDGET]") || !strings.Contains(err.Error(), "129 OAM entries") {
		t.Fatalf("error = %v, want stable 129-entry diagnostic", err)
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
	calls = append(calls, mode0BudgetRepeatedCall(38, "spr(32,0,64,4,4)")...)
	report := requireMode0BudgetReport(t, "dynamic-camera.lua", mode0BudgetTestSource(calls...))
	if report.Mode0OAMEntries != 38 || report.Mode0OBJLineCycles != 1216 {
		t.Fatalf("budget = %+v, want camera-invariant OAM=38 and line cycles=1216", report)
	}
}

func TestMode0ObjectBudgetUsesRuntimeFixedPointForFractionalY(t *testing.T) {
	calls := mode0BudgetRepeatedCall(38, "spr(32,0,-0.000001,4,4)")
	calls = append(calls, "spr(1,0,31)", "spr(1,8,31)")
	_, _, err := TranspileWithReport("fractional-y.lua", mode0BudgetTestSource(calls...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1232 rendering cycles") {
		t.Fatalf("error = %v, want fixed-point-rounded Y rejected at 1232 cycles", err)
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
	accepted := mode0BudgetRepeatedCall(38, "spr(32,0,0,4,4)")
	accepted = append(accepted, "spr(1,0,py)")
	report := requireMode0BudgetReport(t, "static-dynamic-accepted.lua", mode0BudgetTestSource(accepted...))
	if report.Mode0OAMEntries != 39 || report.Mode0OBJLineCycles != 1224 {
		t.Fatalf("accepted budget = %+v, want OAM=39 and line cycles=1224", report)
	}

	rejected := append(append([]string(nil), accepted...), "spr(1,0,other_y)")
	_, _, err := TranspileWithReport("static-dynamic-rejected.lua", mode0BudgetTestSource(rejected...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1232 rendering cycles") {
		t.Fatalf("error = %v, want conservative 1232-cycle rejection", err)
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
	calls = append(calls, mode0BudgetRepeatedCall(96, "spr(1,0,0)")...)
	report := requireMode0BudgetReport(t, "text-sprite-128.lua", mode0BudgetTestSource(calls...))
	if !report.Mode0Text || report.Mode0OAMEntries != 128 || report.Mode0OBJLineCycles != 1024 {
		t.Fatalf("budget = %+v, want text+sprites OAM=128 and line cycles=1024", report)
	}
}

func TestMode0ObjectBudgetAccountsForRuntimeTextWrapping(t *testing.T) {
	calls := mode0BudgetRepeatedCall(38, "spr(32,0,32,4,4)")
	calls = append(calls, fmt.Sprintf("print(%q,228,0,7)", strings.Repeat("A", 32)))
	_, _, err := TranspileWithReport("wrapped-text.lua", mode0BudgetTestSource(calls...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1232 rendering cycles") {
		t.Fatalf("error = %v, want wrapped glyphs included in the 1232-cycle scanline", err)
	}
}

func TestMode0ObjectBudgetRejectsCameraRebaseBetweenObjectBands(t *testing.T) {
	calls := mode0BudgetRepeatedCall(38, "spr(32,0,0,4,4)")
	calls = append(calls, "camera(0,32)", "spr(1,0,32)", "spr(1,8,32)")
	_, _, err := TranspileWithReport("camera-rebase.lua", mode0BudgetTestSource(calls...), Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1232 rendering cycles") {
		t.Fatalf("error = %v, want camera-rebased bands rejected at 1232 cycles", err)
	}
}

func TestMode0ObjectBudgetRejectsCameraRebaseThroughHelper(t *testing.T) {
	calls := mode0BudgetRepeatedCall(38, "spr(32,0,0,4,4)")
	calls = append(calls, "rebase()", "spr(1,0,32)", "spr(1,8,32)")
	source := "function rebase() camera(0,32) end\n" + mode0BudgetTestSource(calls...)
	_, _, err := TranspileWithReport("camera-helper-rebase.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1232 rendering cycles") {
		t.Fatalf("error = %v, want helper camera-rebased bands rejected at 1232 cycles", err)
	}
}

func TestMode0ObjectBudgetAutoFallsBackOnScanlineOverflow(t *testing.T) {
	source := mode0BudgetTestSource(mode0BudgetRepeatedCall(39, "spr(32,0,64,4,4)")...)
	_, report, err := TranspileWithReport("auto-scanline-overflow.lua", source, Options{
		Renderer: RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0Ineligible ||
		!strings.Contains(report.Diagnostic, "[M0_OBJ_BUDGET]") || !strings.Contains(report.Diagnostic, "1248 rendering cycles") {
		t.Fatalf("AUTO report = %+v, want stable scanline-overflow fallback", report)
	}
}

func TestMode0ObjectBudgetDoesNotDiscountHorizontallyOffscreenObjects(t *testing.T) {
	source := mode0BudgetTestSource(mode0BudgetRepeatedCall(39, "spr(32,10000,64,4,4)")...)
	_, _, err := TranspileWithReport("offscreen-x.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "1248 rendering cycles") {
		t.Fatalf("error = %v, want offscreen objects included in scanline budget", err)
	}
}

func TestMode0ObjectBudgetReportsZeroForObjectFreeDraw(t *testing.T) {
	report := requireMode0BudgetReport(t, "no-objects.lua", mode0BudgetTestSource())
	if report.Mode0Objects || report.Mode0ObjectPacking || report.Mode0OAMEntries != 0 || report.Mode0OBJLineCycles != 0 {
		t.Fatalf("object-free report = %+v, want zero budget", report)
	}
}
