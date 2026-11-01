package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/lint"
	"github.com/drpaneas/mtile/internal/sourcebundle"
)

var errLintFindings = errors.New("lint reported advice (strict mode)")

type lintReport struct {
	Files         []string          `json:"files"`
	DisabledRules []string          `json:"disabled_rules"`
	Diagnostics   []lint.Diagnostic `json:"diagnostics"`
	HiddenNotes   int               `json:"hidden_notes"`
}

func runLint(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("lint", flag.ContinueOnError)
	format := flags.String("format", "text", "report format: text or json")
	strict := flags.Bool("strict", false, "exit nonzero when advice is reported (for CI)")
	disable := flags.String("disable", "", "comma-separated rule IDs to disable, e.g. MTL003,MTL004")
	includeNotes := flags.Bool("include-notes", false, "also show conditional allocations and recognized reuse fallbacks")
	docsURL := flags.String("docs-url", "", "rendered guide base URL (default: repository Markdown links)")
	help, err := parseFlags(flags, args, errOut)
	if err != nil || help {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("lint accepts at most one project directory or Lua entry file")
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("unknown lint format %q; use text or json", *format)
	}
	if *docsURL != "" {
		u, err := url.Parse(*docsURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("docs-url must be an http(s) guide base URL without credentials, query or fragment")
		}
	}
	known := map[string]bool{}
	for _, rule := range lint.Rules() {
		known[rule.ID] = true
	}
	disabled := map[string]bool{}
	if *disable != "" {
		for _, name := range strings.Split(*disable, ",") {
			name = strings.TrimSpace(name)
			if !known[name] {
				return fmt.Errorf("unknown lint rule %q", name)
			}
			disabled[name] = true
		}
	}
	input := flags.Arg(0)
	if input == "" {
		input = "."
	}
	path, err := filepath.Abs(input)
	if err != nil {
		return fmt.Errorf("resolve lint input: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read lint input: %w", err)
	}
	root, entry := path, ""
	if info.IsDir() {
		project, err := assetsync.LoadProject(root)
		if err != nil {
			return err
		}
		entry = project.Entry
	} else {
		if !strings.EqualFold(filepath.Ext(path), ".lua") {
			return errors.New("lint entry file must have a .lua extension")
		}
		root, entry = filepath.Dir(path), filepath.Base(path)
	}
	bundle, err := sourcebundle.Resolve(root, entry)
	if err != nil {
		return err
	}
	diagnostics, err := lint.Analyze(bundle)
	if err != nil {
		return err
	}
	report := lintReport{Files: append([]string{}, bundle.Files...), DisabledRules: []string{}, Diagnostics: []lint.Diagnostic{}}
	for name := range disabled {
		report.DisabledRules = append(report.DisabledRules, name)
	}
	sort.Strings(report.DisabledRules)
	for _, d := range diagnostics {
		if !disabled[d.Rule] {
			if d.Severity == "note" && !*includeNotes {
				report.HiddenNotes++
				continue
			}
			d.Documentation = lintDocumentationURL(d.Guide, *docsURL)
			report.Diagnostics = append(report.Diagnostics, d)
		}
	}
	if *format == "json" {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		if err := writeLintText(out, root, report); err != nil {
			return err
		}
	}
	if *strict {
		for _, d := range report.Diagnostics {
			if d.Severity != "note" {
				return errLintFindings
			}
		}
	}
	return nil
}
