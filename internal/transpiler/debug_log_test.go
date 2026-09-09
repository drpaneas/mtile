package transpiler

import (
	"strings"
	"testing"
)

func TestPreviewDiagnosticsOnlyInDebugBuild(t *testing.T) {
	for _, debug := range []bool{false, true} {
		generated, err := Transpile("preview.lua", "function _draw() cls(0) end", Options{Debug: debug})
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range []string{"rt.DebugLog", "Cartridge preview started", "Cartridge assets could not be initialized"} {
			if strings.Contains(generated, message) != debug {
				t.Fatalf("debug=%v: diagnostic %q has wrong presence", debug, message)
			}
		}
	}
}
