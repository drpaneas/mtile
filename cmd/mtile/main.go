// Command mtile is the user-facing native GBA toolchain.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/lsp"
	"github.com/drpaneas/mtile/internal/transpiler"
)

var errMissingCommand = errors.New("missing command")

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, errMissingCommand) {
			fmt.Fprintln(os.Stderr, "mtile:", err)
		}
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	return runWithInput(args, os.Stdin, stdout, stderr)
}

func runWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, usageText)
		return errMissingCommand
	}
	switch args[0] {
	case "doctor":
		return runDoctor(args[1:], stdout)
	case "new":
		return runNew(args[1:], stdout)
	case "build":
		return runBuild(args[1:], stdout)
	case "edit":
		return runEdit(args[1:], stdout, stderr)
	case "lsp":
		return runLSP(args[1:], stdin, stdout, stderr)
	case "lint":
		return runLint(args[1:], stdout, stderr)
	case "import-audio":
		return runImportAudio(args[1:], stdout, stderr)
	case "render-audio":
		return runRenderAudio(args[1:], stdout)
	case "help", "-h", "--help":
		_, _ = fmt.Fprintln(stdout, usageText)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

const usageText = `usage: mtile <command> [options] [project-dir]

Commands:
  doctor check the ROM build toolchain and show setup fixes (no project needed)
  new    create a native GBA project
  build  transpile Lua and assets, link a .gba ROM
  edit   open the Sprites, Map, Audio, Code, and Play IDE
  lsp    serve Lua completion, hover, and live diagnostics over stdio
  lint   review Lua for Effective mTile performance opportunities
  import-audio  import WAV effects and MOD/XM/S3M/IT music with Maxmod
  render-audio  render the PSG editor preview to a WAV file

Run mtile <command> -h for flags.`

func runLSP(args []string, in io.Reader, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("lsp", flag.ContinueOnError)
	help, err := parseFlags(flags, args, errOut)
	if err != nil || help {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("lsp accepts at most one project directory")
	}
	return lsp.Serve(context.Background(), in, out, flags.Arg(0))
}

func runRenderAudio(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("render-audio", flag.ContinueOnError)
	output := flags.String("o", "", "output WAV path (default: project-dir/music-preview.wav)")
	songID := flags.Int("song", 0, "song ID to render")
	help, err := parseFlags(flags, args, out)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if flags.NArg() != 1 {
		return errors.New("render-audio requires exactly one project directory")
	}
	dir, err := filepath.Abs(flags.Arg(0))
	if err != nil {
		return err
	}
	assets, err := assetsync.LoadNativeAudio(dir)
	if err != nil {
		return fmt.Errorf("load four-channel audio v2: %w", err)
	}
	preview, err := editor.RenderNativeMusicPreview(assets.Library, *songID)
	if err != nil {
		return err
	}
	stats := preview.AudioStats()
	if stats.Peak == 0 {
		return errors.New("song preview is silent; place at least one note or drum hit")
	}
	wav, err := editor.RenderNativeMusicPreviewWAV(preview)
	if err != nil {
		return err
	}
	path := *output
	if path == "" {
		path = filepath.Join(dir, "music-preview.wav")
	} else if path, err = filepath.Abs(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, wav, 0o644); err != nil {
		return fmt.Errorf("write preview WAV: %w", err)
	}
	_, _ = fmt.Fprintf(out, "rendered %s (%.2fs, %d Hz mono, peak %d/127, RMS %.2f)\n",
		path, stats.Duration, editor.NativePreviewSampleRate, stats.Peak, stats.RMS)
	return nil
}

func parseFlags(flags *flag.FlagSet, args []string, out io.Writer) (help bool, err error) {
	flags.SetOutput(out)
	err = flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return true, nil
	}
	return false, err
}

func runNew(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	template := flags.String("template", "blank", "starting project: blank or coin-quest")
	width := flags.Int("width", 240, "sprite-sheet width in pixels")
	height := flags.Int("height", 160, "sprite-sheet height in pixels")
	mapWidth := flags.Int("map-width", 0, "map width in 8x8 cells (default 30, or none when the sheet has no complete 8x8 sprite)")
	mapHeight := flags.Int("map-height", 0, "map height in 8x8 cells (default 20, or none when the sheet has no complete 8x8 sprite)")
	help, err := parseFlags(flags, args, out)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if flags.NArg() != 1 {
		return errors.New("new requires exactly one project directory")
	}
	dir := flags.Arg(0)
	if err := build.CreateProject(dir, build.NewProjectOptions{Template: *template, SheetWidth: *width, SheetHeight: *height, MapWidth: *mapWidth, MapHeight: *mapHeight}); err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "created %s\n", abs)
	if *template == "coin-quest" {
		_, _ = fmt.Fprintf(out, "next: mtile edit %q\nwalkthrough: %s\n", abs, filepath.Join(abs, "README.md"))
	}
	return nil
}

func runBuild(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	output := flags.String("o", "", "output ROM path (default: project-dir/game.gba)")
	tinygo := flags.String("tinygo", "", "TinyGo executable (default: PATH)")
	runtimeDir := flags.String("runtime", "", "mtile runtime directory")
	keepWork := flags.Bool("keep-work", false, "keep the temporary generated build workspace")
	rendererName := flags.String("renderer", "", "override project renderer: mode4, mode0, or auto")
	tileCanvas := flags.Bool("experimental-tilecanvas", false, "build this project with generic tiled backgrounds and a sparse OBJ canvas")
	tileObjects := flags.Bool("experimental-tilecanvas-objects", false, "compatibility alias for -experimental-tilecanvas")
	help, err := parseFlags(flags, args, out)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if flags.NArg() != 1 {
		return errors.New("build requires exactly one project directory")
	}
	service, err := build.NewService(*tinygo, *runtimeDir)
	if err != nil {
		return err
	}
	service.KeepWork = *keepWork
	service.ExperimentalTileCanvas = *tileCanvas || *tileObjects
	if strings.TrimSpace(*rendererName) == "" {
		service.UseProjectRenderer = true
	} else {
		renderer, parseErr := transpiler.ParseRenderer(*rendererName)
		if parseErr != nil {
			return parseErr
		}
		service.Renderer = renderer
	}
	result, err := service.Build(flags.Arg(0), *output)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "built %s (%d bytes, %d bytes EWRAM free)\n", result.ROMPath, result.ROMSize, result.EWRAMFree)
	for _, warning := range result.Warnings {
		_, _ = fmt.Fprintln(out, "warning:", warning)
	}
	if result.Renderer == transpiler.RendererMode0 {
		_, _ = fmt.Fprintf(out, "renderer requested %s -> selected %s (%s), %d BG layer(s), OBJ %t, text %t, OAM %d/128, OBJ line %d/1226 cycles\n",
			result.RendererRequested, result.Renderer, result.RendererReason, result.RendererLayers, result.RendererObjects, result.RendererText,
			result.RendererOAMEntries, result.RendererLineCycles)
	} else {
		_, _ = fmt.Fprintf(out, "renderer requested %s -> selected %s (%s)\n", result.RendererRequested, result.Renderer, result.RendererReason)
	}
	if result.RendererDiagnostic != "" {
		_, _ = fmt.Fprintln(out, "renderer fallback:", result.RendererDiagnostic)
	}
	if result.RendererProfile != "" {
		_, _ = fmt.Fprintln(out, "renderer auto profile:", result.RendererProfile)
	}
	if result.AudioVoices != 0 {
		if result.AudioProfile == assetsync.NativeAudioProfile {
			_, _ = fmt.Fprintf(out, "audio: Game Boy PSG CH1-CH4, hardware mix %d%%, %s-byte assets\n",
				result.AudioPSGRatio, formatInteger(result.AudioROMBytes))

		}
	}
	if result.WorkDir != "" {
		_, _ = fmt.Fprintf(out, "workdir %s\n", result.WorkDir)
	}
	if result.MapPath != "" {
		_, _ = fmt.Fprintf(out, "map %s\n", result.MapPath)
	}
	return nil
}

func formatInteger(value int) string {
	digits := fmt.Sprintf("%d", value)
	for index := len(digits) - 3; index > 0; index -= 3 {
		digits = digits[:index] + "," + digits[index:]
	}
	return digits
}

func runEdit(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("edit", flag.ContinueOnError)
	editor := flags.String("editor", "", "editor executable (default: MTILE_EDITOR or sibling mtile-editor/editor)")
	tileCanvas := flags.Bool("experimental-tilecanvas", false, "preview this project with generic tiled backgrounds and a sparse OBJ canvas")
	tileObjects := flags.Bool("experimental-tilecanvas-objects", false, "compatibility alias for -experimental-tilecanvas")
	help, err := parseFlags(flags, args, out)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if flags.NArg() != 1 {
		return errors.New("edit requires exactly one project directory")
	}
	dir, err := filepath.Abs(flags.Arg(0))
	if err != nil {
		return err
	}
	command, commandArgs, workDir, err := editorCommand(*editor, dir)
	if err != nil {
		return err
	}
	cmd := exec.Command(command, commandArgs...)
	cmd.Dir = workDir
	if *tileCanvas || *tileObjects {
		const key = "MTILE_EXPERIMENTAL_TILECANVAS_PROJECT="
		const objectsKey = "MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS="
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, key) && !strings.HasPrefix(value, objectsKey) {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, key+dir)
		if *tileObjects {
			cmd.Env = append(cmd.Env, objectsKey+"1")
		}
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}

// editorCommand resolves how to launch the editor. The returned workDir is
// the directory cmd.Run should use; it is empty (meaning "inherit the
// current directory") for every case except the source-checkout fallback,
// whose "go run ./cmd/editor" package path is relative to the repository
// root rather than to whatever directory mtile itself was invoked from.
func editorCommand(explicit, projectDir string) (command string, args []string, workDir string, err error) {
	if explicit == "" {
		explicit = os.Getenv("MTILE_EDITOR")
	}
	if explicit != "" {
		return explicit, []string{"-project", projectDir}, "", nil
	}
	if self, statErr := os.Executable(); statErr == nil {
		if candidate, ok := findSiblingEditor(self); ok {
			return candidate, []string{"-project", projectDir}, "", nil
		}
	}
	if goBin, lookErr := exec.LookPath("go"); lookErr == nil {
		if root, ok := findSourceCheckout(); ok {
			return goBin, []string{"run", "./cmd/editor", "-project", projectDir}, root, nil
		}
	}
	return "", nil, "", errors.New("editor executable not found; build mtile-editor or editor, or set MTILE_EDITOR")
}

func findSiblingEditor(executable string) (string, bool) {
	directory := filepath.Dir(executable)
	for _, name := range []string{"mtile-editor", "editor"} {
		candidate := filepath.Join(directory, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// findSourceCheckout first walks up from the current directory looking for
// the repository root (a go.mod next to cmd/editor). For a source-built CLI
// invoked outside the checkout it also uses this source file's location as a
// fallback. Either result lets the caller run "go run ./cmd/editor" from the
// directory that path is actually relative to.
func findSourceCheckout() (string, bool) {
	if dir, err := os.Getwd(); err == nil {
		if root, ok := findCheckoutFrom(dir); ok {
			return root, true
		}
	}
	// When the CLI is invoked from a project directory outside this checkout,
	// the current-directory walk cannot find the source tree. In a source
	// build, runtime.Caller still points at this file, so use that location as
	// a second anchor. Installed binaries should use a sibling editor or the
	// MTILE_EDITOR override instead.
	if _, source, _, ok := runtime.Caller(0); ok && filepath.IsAbs(source) {
		if root, ok := findCheckoutFrom(filepath.Dir(source)); ok {
			return root, true
		}
	}
	return "", false
}

func findCheckoutFrom(start string) (string, bool) {
	if start == "" {
		return "", false
	}
	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	start = absStart
	for dir := start; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "cmd", "editor")); err == nil {
				return dir, true
			}
		}
	}
	return "", false
}
