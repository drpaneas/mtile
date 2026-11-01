package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lintFile(t *testing.T, root, name, source string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunLintSourceOnlyProjectAndJSON(t *testing.T) {
	root := t.TempDir()
	lintFile(t, root, "project.json", `{"entry":"main.lua"}`)
	lintFile(t, root, "main.lua", "require(\"step\")\nfunction _update() step() end\n")
	path := lintFile(t, root, "src/step.lua", "function step()\n  local scratch = {}\nend\n")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// No assets or target compiler exist in this project; lint remains source-only.
	t.Setenv("TINYGO", "/not/a/compiler")
	var out, stderr bytes.Buffer
	if err := run([]string{"lint", "-format", "json", root}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	var report lintReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, &out)
	}
	if len(report.Files) != 2 || len(report.Diagnostics) != 1 || report.Diagnostics[0].Path != "src/step.lua" || report.Diagnostics[0].Rule != "MTL001" {
		t.Fatalf("report = %+v", report)
	}
	if report.DisabledRules == nil || stderr.Len() != 0 {
		t.Fatalf("nil list or stderr: %+v %s", report, &stderr)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("lint modified source")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 3 {
		t.Fatalf("lint wrote project files: %v", entries)
	}
}

func TestRunLintStrictAndDisable(t *testing.T) {
	file := lintFile(t, t.TempDir(), "game.lua", "function _draw() pal() end")
	for _, tt := range []struct {
		name     string
		flags    []string
		strict   bool
		contains string
	}{
		{"advisory", nil, false, "MTL003"},
		{"strict", []string{"-strict"}, true, "MTL003"},
		{"disabled", []string{"-strict", "-disable", "MTL003"}, false, "0 advice"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			args := append([]string{"lint"}, tt.flags...)
			args = append(args, file)
			err := run(args, &out, &bytes.Buffer{})
			if tt.strict {
				if !errors.Is(err, errLintFindings) {
					t.Fatalf("strict error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tt.contains) {
				t.Fatalf("output: %s", &out)
			}
		})
	}
	var out bytes.Buffer
	if err := run([]string{"lint", "-strict", "-format", "json", file}, &out, &bytes.Buffer{}); !errors.Is(err, errLintFindings) {
		t.Fatalf("strict JSON: %v", err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("strict contaminated JSON: %s", &out)
	}
}

func TestRunLintCleanJSON(t *testing.T) {
	file := lintFile(t, t.TempDir(), "game.lua", "function _draw() print(\"SCORE\",8,8,7) end")
	var out bytes.Buffer
	if err := run([]string{"lint", "-strict", "-format", "json", file}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"diagnostics": []`) {
		t.Fatalf("empty results should be [], got %s", &out)
	}
}

func TestRunLintInvalidInputsAndHelp(t *testing.T) {
	root := t.TempDir()
	bad := lintFile(t, root, "broken.lua", "function _update(")
	nonlua := lintFile(t, root, "data.txt", "hello")
	lintFile(t, root, "game.lua", "require(\"missing\")")
	for _, args := range [][]string{
		{"lint", "-format", "yaml", root},
		{"lint", "-disable", "MTL999", root},
		{"lint", "-disable", "MTL003,", root},
		{"lint", root, root},
		{"lint", bad},
		{"lint", nonlua},
		{"lint", root},
		{"lint", filepath.Join(root, "missing.lua")},
	} {
		if err := run(args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("expected failure: %v", args)
		}
	}
	var stderr bytes.Buffer
	if err := run([]string{"lint", "-h"}, &bytes.Buffer{}, &stderr); err != nil || !strings.Contains(stderr.String(), "strict") {
		t.Fatalf("help: %v %s", err, &stderr)
	}
	lintFile(t, root, "project.json", `{"entry":"../outside.lua"}`)
	if err := run([]string{"lint", root}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("unsafe project entry accepted")
	}
}
