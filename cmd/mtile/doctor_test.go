package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestDoctorGoVersion(t *testing.T) {
	for output, want := range map[string]bool{
		"go version go1.27.1 linux/amd64":    true,
		"go version go1.27.2 darwin/arm64":   true,
		"go version go1.28.0 linux/amd64":    true,
		"go version go1.27.0 linux/amd64":    false,
		"go version go1.26.4 linux/amd64":    false,
		"go version go1.28rc1 linux/amd64":   false,
		"go version go1.28.0rc1 linux/amd64": false,
		"go version go1.27 linux/amd64":      false,
		"devel go1.28-abcdef linux/amd64":    false,
		"not a Go version":                   false,
	} {
		if got := supportedGoVersion(output); got != want {
			t.Errorf("supportedGoVersion(%q) = %v, want %v", output, got, want)
		}
	}
}

func TestDoctorReportsAllMissingToolsWithFixes(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("MTILE_TINYGO", filepath.Join(t.TempDir(), "missing-tinygo"))
	var out bytes.Buffer
	if err := run([]string{"doctor"}, &out, &out); err == nil {
		t.Fatal("doctor succeeded with missing tools")
	}
	for _, want := range []string{"FAIL Go", "FAIL Build setup", "Fix:", "MTILE_TINYGO"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor output missing %q: %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Next:") {
		t.Fatal("failed doctor advertised successful setup")
	}
}

func TestDoctorHelpAndArguments(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"doctor", "-h"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "TinyGo") {
		t.Fatal("missing toolchain override help")
	}
	for _, args := range [][]string{{"doctor", "project"}, {"doctor", "-unknown"}} {
		if err := run(args, &out, &out); err == nil {
			t.Fatalf("accepted invalid args %v", args)
		}
	}
}

func TestNewCoinQuestIncludesPlayableAssetsAndWalkthrough(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "first game")
	var out bytes.Buffer
	if err := run([]string{"new", "-template", "coin-quest", dir}, &out, &out); err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.NativeAudio == nil || len(assets.NativeAudio.Library.Clips) != 1 {
		t.Fatal("starter has no compiled pickup sound")
	}
	for _, file := range []string{"game.lua", "README.md", "lessons/01-movement.lua", "lessons/02-coins.lua", "lessons/03-sound.lua", "lessons/04-complete.lua"} {
		if data, err := os.ReadFile(filepath.Join(dir, file)); err != nil || len(data) == 0 {
			t.Fatalf("missing template file %s: %v", file, err)
		}
	}
	game, _ := os.ReadFile(filepath.Join(dir, "game.lua"))
	if err := run([]string{"new", "-template", "coin-quest", dir}, &out, &out); err == nil {
		t.Fatal("overwrote an existing project")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "game.lua"))
	if !bytes.Equal(after, game) {
		t.Fatal("refused creation changed existing source")
	}
}

func TestNewRejectsInvalidTemplateBeforeCreatingDirectory(t *testing.T) {
	for _, options := range [][]string{{"-template", "missing"}, {"-template", "coin-quest", "-width", "8"}} {
		dir := filepath.Join(t.TempDir(), "game")
		args := append(append([]string{"new"}, options...), dir)
		if err := run(args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if _, err := os.Lstat(dir); !os.IsNotExist(err) {
			t.Fatalf("invalid template left a directory: %v", err)
		}
	}
}
