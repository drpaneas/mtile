package transpiler

import "testing"

func TestRendererReportCountsUniqueStaticStrings(t *testing.T) {
	_, report, err := TranspileWithReport("static-count.lua", `
function pair()
  return "alpha", "alpha", "beta"
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.StaticStrings != 2 {
		t.Fatalf("StaticStrings = %d, want 2 unique entries", report.StaticStrings)
	}
}
