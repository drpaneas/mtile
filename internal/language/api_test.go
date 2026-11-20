package language

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestReferenceCoversSupportedBuiltins(t *testing.T) {
	for name := range transpiler.Builtins {
		entry, ok := Lookup(name)
		if !ok || entry.Signature == "" || entry.Summary == "" {
			t.Errorf("missing help for builtin %s", name)
		}
	}
	entries := Complete("ss")
	if len(entries) != 2 || entries[0].Name != "sset" || entries[1].Name != "sspr" {
		t.Fatalf("completion = %+v", entries)
	}
	if got := Complete("samp"); len(got) != 1 || got[0].Name != "sample" {
		t.Errorf("completion for samp = %+v, want sample", got)
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
