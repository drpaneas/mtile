package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/drpaneas/mtile/internal/hostdocs"
	"github.com/drpaneas/mtile/internal/lint"
	"github.com/rivo/uniseg"
)

func lintDocumentationURL(guide, base string) string {
	return hostdocs.URL(guide, base)
}

func lintDisplayPath(root, path string) string {
	full := filepath.Join(root, filepath.FromSlash(path))
	if cwd, err := os.Getwd(); err == nil {
		if relative, err := filepath.Rel(cwd, full); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(relative)
		}
	}
	return filepath.ToSlash(full)
}

// Report text is deliberately plain: readable in a terminal and deterministic
// in redirected logs, without ANSI/OSC escape sequences from source contents.
func writeLintText(out io.Writer, root string, report lintReport) error {
	var b strings.Builder
	advice, notes := 0, 0
	for _, d := range report.Diagnostics {
		severity := d.Severity
		if severity == "" {
			severity = "advice"
		}
		if severity == "note" {
			notes++
		} else {
			advice++
		}
		fmt.Fprintf(&b, "%s[%s]: %s\n", severity, d.Rule, terminalText(d.Message))
		width := lintFrame(&b, root, "-->", lint.Location{Path: d.Path, Line: d.Line, Column: d.Column}, d.Label, d.Snippet)
		for _, related := range d.Related {
			lintFrame(&b, root, ":::", related.Location, related.Label, related.Snippet)
		}
		if len(d.CallPath) > 0 {
			lintMessage(&b, width, "note", "named-call witness: "+strings.Join(d.CallPath, " -> "))
		}
		if at := d.ConditionalAt; at != nil {
			fmt.Fprintf(&b, "%*s = note: path includes conditional control flow at %s:%d:%d\n", width, "", terminalText(lintDisplayPath(root, at.Path)), at.Line, at.Column)
		}
		for _, note := range d.Notes {
			lintMessage(&b, width, "note", note)
		}
		lintMessage(&b, width, "help", d.Suggestion)
		fmt.Fprintf(&b, "%*s = docs: %s\n\n", width, "", d.Documentation)
	}
	if len(report.DisabledRules) > 0 {
		fmt.Fprintf(&b, "Disabled rules: %s\n", strings.Join(report.DisabledRules, ", "))
	}
	fmt.Fprintf(&b, "MTile Lint: %d advice item(s), %d note(s) shown, %d source file(s).\n", advice, notes, len(report.Files))
	if report.HiddenNotes > 0 {
		fmt.Fprintf(&b, "%d contextual allocation note(s) hidden; use -include-notes to review them. These are not proven optimization mistakes.\n", report.HiddenNotes)
	}
	b.WriteString("Measure the complete game before accepting a rewrite.\n")
	_, err := io.WriteString(out, b.String())
	return err
}

func lintFrame(b *strings.Builder, root, arrow string, at lint.Location, label string, snippet []lint.SourceLine) int {
	fmt.Fprintf(b, "  %s %s:%d:%d\n", arrow, terminalText(lintDisplayPath(root, at.Path)), at.Line, at.Column)
	width := len(strconv.Itoa(at.Line))
	for _, line := range snippet {
		width = max(width, len(strconv.Itoa(line.Number)))
	}
	fmt.Fprintf(b, "%*s |\n", width, "")
	for _, line := range snippet {
		text := terminalText(line.Text)
		caret := 0
		if line.Number == at.Line {
			byteOffset := max(0, min(at.Column-1, len(line.Text)))
			caret = uniseg.StringWidth(terminalText(line.Text[:byteOffset]))
		}
		shown, pad := lintSourceWindow(text, caret)
		fmt.Fprintf(b, "%*d | %s\n", width, line.Number, shown)
		if line.Number == at.Line {
			fmt.Fprintf(b, "%*s | %s^ %s\n", width, "", strings.Repeat(" ", pad), terminalText(label))
		}
	}
	fmt.Fprintf(b, "%*s |\n", width, "")
	return width
}

func lintMessage(b *strings.Builder, gutter int, kind, text string) {
	prefix := fmt.Sprintf("%*s = %s: ", gutter, "", kind)
	b.WriteString(prefix)
	indent := uniseg.StringWidth(prefix)
	column := indent
	for i, word := range strings.Fields(terminalText(text)) {
		w := uniseg.StringWidth(word)
		if i > 0 {
			if column+1+w > 100 {
				b.WriteByte('\n')
				b.WriteString(strings.Repeat(" ", indent))
				column = indent
			} else {
				b.WriteByte(' ')
				column++
			}
		}
		b.WriteString(word)
		column += w
	}
	b.WriteByte('\n')
}

func terminalText(s string) string {
	var b strings.Builder
	column := 0
	clusters := uniseg.NewGraphemes(s)
	for clusters.Next() {
		cluster := clusters.Str()
		if cluster == "\t" {
			spaces := 4 - column%4
			b.WriteString(strings.Repeat(" ", spaces))
			column += spaces
			continue
		}
		if strings.ContainsFunc(cluster, unicode.IsControl) {
			for _, r := range cluster {
				if unicode.IsControl(r) {
					escaped := fmt.Sprintf("\\u{%x}", r)
					b.WriteString(escaped)
					column += len(escaped)
				} else {
					b.WriteRune(r)
					column += uniseg.StringWidth(string(r))
				}
			}
			continue
		}
		b.WriteString(cluster)
		column += clusters.Width()
	}
	return b.String()
}

// Keep a long source line readable while retaining the actual primary column.
// Cropping happens on grapheme boundaries, including double-width CJK/emoji.
func lintSourceWindow(text string, caret int) (string, int) {
	if uniseg.StringWidth(text) <= 100 {
		return text, caret
	}
	start := max(0, caret-40)
	var b strings.Builder
	column, begin, shown := 0, 0, 0
	clusters := uniseg.NewGraphemes(text)
	for clusters.Next() {
		w := clusters.Width()
		if column+w <= start {
			column += w
			begin = column
			continue
		}
		if shown+w > 92 {
			break
		}
		b.WriteString(clusters.Str())
		shown += w
		column += w
	}
	prefix, suffix := "", ""
	if begin > 0 {
		prefix = "... "
	}
	if column < uniseg.StringWidth(text) {
		suffix = " ..."
	}
	return prefix + b.String() + suffix, caret - begin + len(prefix)
}
