package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	editormodel "github.com/drpaneas/mtile/internal/editor"
)

func TestRunNoArgsReturnsUsageError(t *testing.T) {
	var stderr bytes.Buffer
	err := run(nil, &bytes.Buffer{}, &stderr)
	if !errors.Is(err, errMissingCommand) {
		t.Fatalf("run(nil) error = %v, want missing command", err)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := run([]string{"frobnicate"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "frobnicate") {
		t.Fatalf("run([frobnicate]) error = %v, want it to name the unknown command", err)
	}
}

func TestRunHelpPrintsUsageAndSucceeds(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"help"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([help]) error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "usage:") || !strings.Contains(got, "build") {
		t.Fatalf("help output = %q, want usage and the build command", got)
	}
}

func TestFormatIntegerGroupsBuildTelemetry(t *testing.T) {
	for value, want := range map[int]string{0: "0", 999: "999", 16384: "16,384", 1048576: "1,048,576"} {
		if got := formatInteger(value); got != want {
			t.Fatalf("formatInteger(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestRunSubcommandHelpSucceeds(t *testing.T) {
	for _, cmd := range []string{"new", "build", "edit", "render-audio"} {
		t.Run(cmd, func(t *testing.T) {
			var out bytes.Buffer
			if err := run([]string{cmd, "-h"}, &out, &bytes.Buffer{}); err != nil {
				t.Fatalf("run([%s -h]) error = %v", cmd, err)
			}
			if !strings.Contains(out.String(), "Usage of") {
				t.Fatalf("%s -h output = %q, want the flag defaults", cmd, out.String())
			}
		})
	}
}

func TestRunRenderAudioExportsAudiblePSGWAV(t *testing.T) {
	project := filepath.Join("..", "..", "examples", "audio-gb-editor")
	output := filepath.Join(t.TempDir(), "preview.wav")
	var stdout bytes.Buffer
	if err := run([]string{"render-audio", "-o", output, project}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("render-audio: %v", err)
	}
	wav, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) <= 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("render-audio output is not a populated WAV: %d bytes", len(wav))
	}
	if !strings.Contains(stdout.String(), "peak") || !strings.Contains(stdout.String(), "RMS") {
		t.Fatalf("render-audio telemetry = %q", stdout.String())
	}
}

func TestRunRenderAudioRejectsSilentSong(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "silent")
	if err := run([]string{"new", dir}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	model, err := editormodel.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.EnsureNativeClip(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := model.Save(); err != nil {
		t.Fatal(err)
	}
	err = run([]string{"render-audio", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "silent") {
		t.Fatalf("silent render error = %v", err)
	}
}

// TestRunNewCreatesProjectAndReportsPath exercises the CLI entry point (not
// just build.CreateProject directly) so a regression in flag wiring - wrong
// flag name, wrong field, args in the wrong order - shows up here.
func TestRunNewCreatesProjectAndReportsPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "game")
	var out bytes.Buffer
	if err := run([]string{"new", "-width", "64", "-height", "64", "-map-width", "4", "-map-height", "4", dir}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([new]) error = %v", err)
	}
	if !strings.Contains(out.String(), "created") || !strings.Contains(out.String(), dir) {
		t.Fatalf("new output = %q, want it to mention %q", out.String(), dir)
	}
	if _, err := os.Stat(filepath.Join(dir, build.GameFileName)); err != nil {
		t.Fatalf("new did not create %s: %v", build.GameFileName, err)
	}
}

func TestRunNewDefaultProjectHasNativeMap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "game")
	if err := run([]string{"new", dir}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([new]) error = %v", err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.SheetW != 240 || assets.SheetH != 160 || assets.MapW != 30 || assets.MapH != 20 || assets.SpriteN != 600 {
		t.Fatalf("CLI default project = %dx%d map %dx%d sprites %d", assets.SheetW, assets.SheetH, assets.MapW, assets.MapH, assets.SpriteN)
	}
}

func TestRunNewLeftoverEdgeSheetCreatesEmptyMap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "edge")
	if err := run([]string{"new", "-width", "7", "-height", "8", dir}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([new -width 7]) error = %v", err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.SpriteN != 0 || assets.MapW != 0 || assets.MapH != 0 {
		t.Fatalf("leftover-edge CLI project = sprites %d map %dx%d", assets.SpriteN, assets.MapW, assets.MapH)
	}
}

func TestRunNewRejectsBadFlag(t *testing.T) {
	if err := run([]string{"new", "-not-a-flag", "x"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run([new -not-a-flag]) should surface the flag parse error")
	}
}

// TestRunNewPropagatesCreateProjectError guards runNew's error path from
// build.CreateProject: this is the only test that would catch a regression
// where runNew silently swallowed the error instead of returning it.
func TestRunNewPropagatesCreateProjectError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "game")
	err := run([]string{"new", "-width", "-1", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("run([new -width -1]) error = %v, want CreateProject's dimension error", err)
	}
}

func TestRunNewRejectsWrongArgCount(t *testing.T) {
	if err := run([]string{"new"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run([new]) with no project directory should fail")
	}
	if err := run([]string{"new", "a", "b"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run([new]) with two project directories should fail")
	}
}

func TestRunBuildRejectsWrongArgCount(t *testing.T) {
	if err := run([]string{"build"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run([build]) with no project directory should fail")
	}
}

func TestRunBuildRejectsBadFlag(t *testing.T) {
	if err := run([]string{"build", "-not-a-flag", "x"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run([build -not-a-flag]) should surface the flag parse error")
	}
}

// TestRunBuildPropagatesNewServiceError guards runBuild's error path from
// build.NewService: this is the only test that would catch a regression
// where a bad -runtime flag was silently ignored instead of failing fast.
func TestRunBuildPropagatesNewServiceError(t *testing.T) {
	err := run([]string{"build", "-runtime", t.TempDir(), t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run([build -runtime <empty dir>]) should fail NewService's runtime check")
	}
}

// TestRunBuildPropagatesServiceBuildError guards runBuild's error path from
// Service.Build itself (as opposed to NewService): this is the only test
// that would catch a regression where a missing project directory was
// silently ignored instead of failing.
func TestRunBuildPropagatesServiceBuildError(t *testing.T) {
	err := run([]string{"build", filepath.Join(t.TempDir(), "does-not-exist")}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "project directory") {
		t.Fatalf("run([build <missing dir>]) error = %v, want Service.Build's project directory error", err)
	}
}

// TestRunBuildEndToEndWithRealTinyGo exercises the "new" then "build"
// subcommands back to back through run(), the same entry point main() uses,
// rather than calling build.CreateProject/Service.Build directly: it is the
// only test that would catch a regression in how runBuild wires its flags
// (-o, -tinygo, -runtime, -keep-work) into internal/build's API.
func TestRunBuildEndToEndWithRealTinyGo(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "e2e")
	if err := run([]string{"new", "-width", "16", "-height", "16", "-map-width", "2", "-map-height", "2", dir}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([new]) error = %v", err)
	}
	romPath := filepath.Join(t.TempDir(), "out.gba")
	var out bytes.Buffer
	if err := run([]string{"build", "-o", romPath, dir}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([build]) error = %v", err)
	}
	if !strings.Contains(out.String(), romPath) || !strings.Contains(out.String(), "EWRAM free") {
		t.Fatalf("build output = %q, want it to mention %q and EWRAM free", out.String(), romPath)
	}
	if _, err := os.Stat(romPath); err != nil {
		t.Fatalf("build did not write a ROM at -o path: %v", err)
	}
}

func TestRunBuildHelloExampleWithRealTinyGo(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	exampleDir, err := filepath.Abs(filepath.Join("..", "..", "examples", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(exampleDir)
	if err != nil {
		t.Fatalf("load hello assets: %v", err)
	}
	// Sprite 0 is reserved blank (the "empty map cell" convention: map()
	// skips cell id 0 outright), so the player graphic lives at sprite 1,
	// sheet-pixel (10,0) - local (2,0) within that 8x8 cell, which
	// spritesheet.json authors as 31 - giving flat index 10. Index 2 falls
	// inside sprite 0 and is 0 by design; it doesn't prove authored pixel
	// data made it through.
	const playerPixelIndex = 10
	if assets.SheetW != 240 || assets.SheetH != 160 || assets.SpriteN != 600 || assets.SheetROM[playerPixelIndex] != 31 || assets.Palette.Name != assetsync.CanonicalPaletteName {
		t.Fatalf("hello assets = %dx%d sprites=%d pixel[%d]=%d palette=%q", assets.SheetW, assets.SheetH, assets.SpriteN, playerPixelIndex, assets.SheetROM[playerPixelIndex], assets.Palette.Name)
	}
	romPath := filepath.Join(t.TempDir(), "hello.gba")
	var out bytes.Buffer
	if err := run([]string{"build", "-o", romPath, exampleDir}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([build hello]) error: %v", err)
	}
	rom, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := build.ValidateROM(rom); err != nil {
		t.Fatalf("hello ROM failed validation: %v", err)
	}
	if !strings.Contains(out.String(), romPath) || !strings.Contains(out.String(), "EWRAM free") {
		t.Fatalf("hello build output = %q", out.String())
	}
}

// TestRunBuildKeepWorkReportsWorkdirAndMap guards the -keep-work flag's
// output: Service.Build only populates Result.WorkDir/MapPath when
// KeepWork is set, so this is the only test that exercises runBuild's
// "map %s" line at all.
func TestRunBuildKeepWorkReportsWorkdirAndMap(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH")
	}
	if testing.Short() {
		t.Skip("skipping a real TinyGo link in -short mode")
	}
	dir := filepath.Join(t.TempDir(), "e2e")
	if err := run([]string{"new", "-width", "16", "-height", "16", "-map-width", "2", "-map-height", "2", dir}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([new]) error = %v", err)
	}
	var out bytes.Buffer
	if err := run([]string{"build", "-keep-work", dir}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([build -keep-work]) error = %v", err)
	}
	if !strings.Contains(out.String(), "workdir ") {
		t.Fatalf("build -keep-work output = %q, want a workdir line", out.String())
	}
	if !strings.Contains(out.String(), "map ") {
		t.Fatalf("build -keep-work output = %q, want a map line", out.String())
	}
}

func TestRunEditRejectsWrongArgCount(t *testing.T) {
	if err := run([]string{"edit"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run([edit]) with no project directory should fail")
	}
}

func TestRunEditRejectsBadFlag(t *testing.T) {
	if err := run([]string{"edit", "-not-a-flag", "x"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run([edit -not-a-flag]) should surface the flag parse error")
	}
}

// TestRunEditPropagatesEditorCommandError guards runEdit's error path from
// editorCommand: this is the only test that would catch a regression where
// "no editor found" was silently ignored instead of returned from run().
func TestRunEditPropagatesEditorCommandError(t *testing.T) {
	outside := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MTILE_EDITOR", "")
	t.Setenv("PATH", "")
	if err := run([]string{"edit", outside}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run([edit]) with no editor available anywhere should fail")
	}
}

// TestRunEditRunsExplicitEditorAndPropagatesFailure exercises runEdit's
// exec.Command/cmd.Run wiring end to end with a real subprocess (a script
// standing in for a compiled editor) instead of only unit-testing
// editorCommand's argument construction: it is the only test that would
// catch a broken working directory, a swallowed exit status, or an
// unwrapped error from cmd.Run.
func TestRunEditRunsExplicitEditorAndPropagatesFailure(t *testing.T) {
	dir := t.TempDir()
	okScript := filepath.Join(dir, "ok-editor.sh")
	if err := os.WriteFile(okScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"edit", "-editor", okScript, "."}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run([edit]) with a successful editor returned an error: %v", err)
	}

	failScript := filepath.Join(dir, "fail-editor.sh")
	if err := os.WriteFile(failScript, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"edit", "-editor", failScript, "."}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "run editor") {
		t.Fatalf("run([edit]) with a failing editor error = %v, want it wrapped as \"run editor: ...\"", err)
	}
}

func TestEditorCommandPrefersExplicitFlagOverEnv(t *testing.T) {
	t.Setenv("MTILE_EDITOR", "/env/editor")
	command, args, workDir, err := editorCommand("/flag/editor", "/proj")
	if err != nil {
		t.Fatal(err)
	}
	if command != "/flag/editor" {
		t.Fatalf("command = %q, want the explicit flag value", command)
	}
	if len(args) != 2 || args[0] != "-project" || args[1] != "/proj" {
		t.Fatalf("args = %v, want [-project /proj]", args)
	}
	if workDir != "" {
		t.Fatalf("workDir = %q, want empty (inherit cwd) for an explicit editor", workDir)
	}
}

func TestEditorCommandFallsBackToEnvVar(t *testing.T) {
	t.Setenv("MTILE_EDITOR", "/env/editor")
	command, args, workDir, err := editorCommand("", "/proj")
	if err != nil {
		t.Fatal(err)
	}
	if command != "/env/editor" {
		t.Fatalf("command = %q, want MTILE_EDITOR value", command)
	}
	if len(args) != 2 || args[1] != "/proj" {
		t.Fatalf("args = %v", args)
	}
	if workDir != "" {
		t.Fatalf("workDir = %q, want empty for the env-var editor", workDir)
	}
}

func TestFindSiblingEditorPrefersNamedBinaryAndAcceptsGoBuildName(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "editor"), []byte("editor"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := findSiblingEditor(filepath.Join(directory, "mtile"))
	if !ok || got != filepath.Join(directory, "editor") {
		t.Fatalf("findSiblingEditor without named binary = %q, %v", got, ok)
	}
	preferred := filepath.Join(directory, "mtile-editor")
	if err := os.WriteFile(preferred, []byte("named editor"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok = findSiblingEditor(filepath.Join(directory, "mtile"))
	if !ok || got != preferred {
		t.Fatalf("findSiblingEditor with named binary = %q, %v; want %q", got, ok, preferred)
	}
}

// TestFindSourceCheckoutLocatesAncestorRoot guards the bug where the
// source-checkout fallback located the repository root by walking up from
// the current directory, but then executed "go run ./cmd/editor" without
// changing into that root: the package path is relative to the discovered
// root, not to whatever subdirectory mtile happened to be invoked from.
func TestFindSourceCheckoutLocatesAncestorRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "editor"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "examples", "hello")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	// Resolve symlinks (e.g. macOS /tmp -> /private/tmp) so the comparison
	// below matches what os.Getwd() actually returns from inside the loop.
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gotRoot, ok := findSourceCheckout()
	if !ok {
		t.Fatal("findSourceCheckout did not locate the ancestor repository root")
	}
	gotRoot, err = filepath.EvalSymlinks(gotRoot)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("findSourceCheckout root = %q, want %q", gotRoot, wantRoot)
	}
}

func TestFindSourceCheckoutReturnsFalseOutsideAnyCheckout(t *testing.T) {
	outside := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}
	if _, ok := findCheckoutFrom(outside); ok {
		t.Fatal("findSourceCheckout found a checkout under an isolated temp directory")
	}
}

func TestFindSourceCheckoutUsesSourceLocationOutsideCheckout(t *testing.T) {
	outside := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}
	root, ok := findSourceCheckout()
	if !ok {
		t.Fatal("findSourceCheckout did not use the source-file fallback")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("source fallback root %q has no go.mod: %v", root, err)
	}
}

// TestEditorCommandSourceCheckoutSetsWorkDir guards the same bug as
// TestFindSourceCheckoutLocatesAncestorRoot from editorCommand's side: the
// returned workDir must be the discovered repository root so runEdit can
// set cmd.Dir before running "go run ./cmd/editor", regardless of what
// directory mtile was actually invoked from.
func TestEditorCommandSourceCheckoutSetsWorkDir(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found on PATH")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "editor"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "examples", "hello")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MTILE_EDITOR", "")
	command, args, workDir, err := editorCommand("", "/proj")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(command, "go") {
		t.Fatalf("command = %q, want the go tool", command)
	}
	if len(args) != 4 || args[0] != "run" || args[1] != "./cmd/editor" {
		t.Fatalf("args = %v, want [run ./cmd/editor -project /proj]", args)
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gotRoot, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("workDir = %q, want the repository root %q so ./cmd/editor resolves correctly", workDir, wantRoot)
	}
}

func TestEditorCommandErrorsWhenNothingIsFound(t *testing.T) {
	t.Setenv("MTILE_EDITOR", "")
	outside := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	if _, _, _, err := editorCommand("", "/proj"); err == nil {
		t.Fatal("editorCommand should fail when no editor, env var, sibling binary, or source checkout is available")
	}
}
