package language

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestReferenceCoversSupportedBuiltins(t *testing.T) {
	for name := range transpiler.Builtins {
		if name == "sample" { // Removed Audio v1 API, kept for its diagnostic.
			continue
		}
		entry, ok := Lookup(name)
		if !ok || entry.Signature == "" || entry.Summary == "" {
			t.Errorf("missing help for builtin %s", name)
		}
	}
	entries := Complete("ss")
	if len(entries) != 2 || entries[0].Name != "sset" || entries[1].Name != "sspr" {
		t.Fatalf("completion = %+v", entries)
	}
	if _, ok := Lookup("sample"); ok {
		t.Fatal("removed PCM API is suggested")
	}
}

func TestHoverUsesRendererContract(t *testing.T) {
	for name, want := range map[string]string{"spr": "Mode 0 conditional", "circfill": "Mode 4 only", "btn": "Renderer independent", "mget": "Available in Mode 0 and Mode 4"} {
		entry, _ := Lookup(name)
		if text := entry.Markdown(); !strings.Contains(text, want) || !strings.Contains(text, entry.Signature) {
			t.Fatalf("%s hover = %q", name, text)
		}
	}
}
