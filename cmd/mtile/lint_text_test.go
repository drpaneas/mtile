package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/lint"
)

func TestLintTextShowsSourceReasonHelpAndDocumentation(t *testing.T) {
	root, _ := os.Getwd()
	report := lintReport{Files: []string{"game.lua"}, Diagnostics: []lint.Diagnostic{{
		Rule: "MTL001", Path: "game.lua", Line: 3, Column: 14, Severity: "advice",
		Message:  "A new table can be allocated on a frame-reachable path.",
		Label:    "fresh table constructor",
		Snippet:  []lint.SourceLine{{Number: 2, Text: "function _update()"}, {Number: 3, Text: "  readings = {1, 2, 3, 4}"}, {Number: 4, Text: "end"}},
		CallPath: []string{"_update"}, Suggestion: "Refill slots only if old identities are not needed.",
		Documentation: "https://example.test/guide#reuse",
	}}}
	var out bytes.Buffer
	if err := writeLintText(&out, root, report); err != nil {
		t.Fatal(err)
	}
	const want = `advice[MTL001]: A new table can be allocated on a frame-reachable path.
  --> game.lua:3:14
  |
2 | function _update()
3 |   readings = {1, 2, 3, 4}
  |              ^ fresh table constructor
4 | end
  |
  = note: named-call witness: _update
  = help: Refill slots only if old identities are not needed.
  = docs: https://example.test/guide#reuse

MTile Lint: 1 advice item(s), 0 note(s) shown, 1 source file(s).
Measure the complete game before accepting a rewrite.
`
	if out.String() != want {
		t.Fatalf("diagnostic changed:\n%s\nwant:\n%s", &out, want)
	}
}

func TestRunLintRendersByteColumnsAsTerminalColumns(t *testing.T) {
	line := "\tlocal tag = \"π界\"; local scratch = {}"
	file := lintFile(t, t.TempDir(), "game.lua", "function _update()\n"+line+"\nend\n")
	var out bytes.Buffer
	if err := run([]string{"lint", file}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// Four cells for the tab, one for π and two for 界: the '{' starts at
	// display offset 39, while the parser's byte column intentionally differs.
	if !strings.Contains(out.String(), "  | "+strings.Repeat(" ", 39)+"^ fresh table constructor") {
		t.Fatalf("misaligned Unicode/tab caret:\n%s", &out)
	}
	if !strings.Contains(out.String(), "2 |     local tag = \"π界\"; local scratch = {}") {
		t.Fatalf("missing original source: %s", &out)
	}
}

func TestLintSourceWindowAndControlCharacters(t *testing.T) {
	text := strings.Repeat("界", 80) + "{}"
	shown, pad := lintSourceWindow(text, 160)
	if pad != 44 || !strings.HasPrefix(shown, "... ") || !strings.HasSuffix(shown, "{}") {
		t.Fatalf("lost primary point: %q %d", shown, pad)
	}
	if got := terminalText("a\x1b[31m\tb"); got != "a\\u{1b}[31m b" {
		t.Fatalf("unsafe control formatting: %q", got)
	}
}

func TestConditionalNotesDoNotFailCI(t *testing.T) {
	file := lintFile(t, t.TempDir(), "game.lua", `function reset() player={} end
function _update() if dead then reset() end end`)
	for _, show := range []bool{false, true} {
		args := []string{"lint", "-strict", "-format", "json"}
		if show {
			args = append(args, "-include-notes")
		}
		args = append(args, file)
		var out bytes.Buffer
		if err := run(args, &out, &bytes.Buffer{}); err != nil {
			t.Fatalf("notes should not fail CI: %v", err)
		}
		var report lintReport
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if show {
			if len(report.Diagnostics) != 1 || report.Diagnostics[0].Severity != "note" || report.HiddenNotes != 0 {
				t.Fatalf("missing audit note: %+v", report)
			}
		} else if len(report.Diagnostics) != 0 || report.HiddenNotes != 1 {
			t.Fatalf("hidden notes not disclosed: %+v", report)
		}
	}
	// Real unconditional work still fails, regardless of a helper's name.
	file = lintFile(t, t.TempDir(), "game.lua", `function initialize_cache() cache={} end
function _update() initialize_cache() end`)
	if err := run([]string{"lint", "-strict", file}, &bytes.Buffer{}, &bytes.Buffer{}); !errors.Is(err, errLintFindings) {
		t.Fatalf("lost unconditional advice: %v", err)
	}
}

func TestRunLintDocumentationURLsAndSourcePaths(t *testing.T) {
	file := lintFile(t, t.TempDir(), "game.lua", "function _draw() pal(1) end")
	var out bytes.Buffer
	if err := run([]string{"lint", "-format", "json", "-docs-url", "http://127.0.0.1:4191/mtile/", file}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var report lintReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if got := report.Diagnostics[0].Documentation; got != "http://127.0.0.1:4191/mtile/guides/performance.html#change-the-colours-that-actually-change" {
		t.Fatalf("wrong URL: %q", got)
	}
	if report.Diagnostics[0].Path != "game.lua" || !strings.HasPrefix(report.Diagnostics[0].Guide, "docs/performance.md#") {
		t.Fatal("changed existing project-relative JSON fields")
	}
	if got := lintDocumentationURL("docs/performance.md#reuse", ""); got != "https://github.com/drpaneas/mtile/blob/main/docs/performance.md#reuse" {
		t.Fatalf("repository URL: %q", got)
	}
	if got := lintDocumentationURL("docs/lint.md#mtl008-discarded-construction", "http://127.0.0.1:4191"); got != "http://127.0.0.1:4191/guides/lint.html#mtl008-discarded-construction" {
		t.Fatalf("discarded-construction guide URL: %q", got)
	}
	for _, base := range []string{"file:///tmp/docs", "https://user:secret@example.test", "https://example.test/?token=1", "https://example.test/#part", "nonsense"} {
		if err := run([]string{"lint", "-docs-url", base, file}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid base accepted: %q", base)
		}
	}
}

func TestRunLintDiscardedConstructionIsVisibleAndFailsStrict(t *testing.T) {
	file := lintFile(t, t.TempDir(), "game.lua", `function assemble() cache={} end
function _update()
 if ready then
  assemble()
  assemble()
 end
end`)
	var out bytes.Buffer
	err := run([]string{"lint", "-strict", "-format", "json", file}, &out, &bytes.Buffer{})
	if !errors.Is(err, errLintFindings) {
		t.Fatalf("discarded construction must be actionable without include-notes: %v", err)
	}
	var report lintReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Diagnostics) != 1 || report.Diagnostics[0].Rule != "MTL008" || report.Diagnostics[0].Severity != "advice" || report.HiddenNotes != 1 {
		t.Fatalf("wrong distinction between evidence and context: %+v", report)
	}
	seen := map[int]bool{}
	for _, related := range report.Diagnostics[0].Related {
		seen[related.Line] = true
	}
	if !seen[4] || !seen[5] {
		t.Fatalf("missing first/replacement call: %+v", report.Diagnostics[0])
	}
	out.Reset()
	if err := run([]string{"lint", file}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"advice[MTL008]", ":4:3", ":5:3", "first construction reached through assemble()", "replacement reached through assemble()"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("missing related frame %q:\n%s", text, &out)
		}
	}
	if err := run([]string{"lint", "-strict", "-disable", "MTL008", file}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("explicit disable should keep only hidden allocation notes: %v", err)
	}
}
