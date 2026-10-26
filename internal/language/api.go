// Package language provides compiler-backed builtin help for editor clients.
package language

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/drpaneas/mtile/internal/transpiler"
	"github.com/drpaneas/mtile/website"
)

type Entry struct {
	Name, Category, Signature, Summary, Example, Note string
}

var reference = loadReference()

func loadReference() map[string]Entry {
	var entries []Entry
	if err := json.Unmarshal([]byte(website.APIJSON), &entries); err != nil {
		panic(fmt.Sprintf("invalid embedded API documentation: %v", err))
	}
	result := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		if _, exists := transpiler.Builtins[entry.Name]; exists {
			result[entry.Name] = entry
		}
	}
	return result
}

// Complete intentionally offers supported builtins, without depending on a
// successful parse of the buffer being edited. sample is retained by the
// compiler only to diagnose removed PCM audio and must not be suggested.
func Complete(prefix string) []Entry {
	result := []Entry{}
	for name := range transpiler.Builtins {
		if entry, ok := reference[name]; ok && strings.HasPrefix(name, prefix) {
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func Lookup(name string) (Entry, bool) {
	entry, ok := reference[name]
	return entry, ok
}

func (e Entry) Markdown() string {
	var renderer string
	switch transpiler.BuiltinRendererCompatibility(e.Name) {
	case transpiler.RendererIndependent:
		renderer = "Renderer independent."
	case transpiler.RendererBoth:
		renderer = "Available in Mode 0 and Mode 4; project-wide renderer checks still apply."
	case transpiler.RendererMode0Conditional:
		renderer = "Mode 0 conditional: exact call shapes and project assets must pass the compiler's compatibility checks. AUTO also requires a proven cost profile."
	case transpiler.RendererMode4Only:
		renderer = "Mode 4 only. A reachable call prevents Mode 0 selection; AUTO falls back to Mode 4."
	default:
		renderer = "Renderer compatibility is unclassified."
	}
	text := "```lua\n" + e.Signature + "\n```\n\n" + e.Summary
	if e.Note != "" {
		text += "\n\n" + e.Note
	}
	text += "\n\n" + renderer
	if e.Example != "" {
		text += "\n\n```lua\n" + e.Example + "\n```"
	}
	return text
}
