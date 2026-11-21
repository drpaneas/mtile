package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

type developROMExport struct {
	ready   chan developROMExportResult
	cancel  context.CancelFunc
	label   string
	path    string
	message string
}

type developROMExportResult struct {
	destination string
	build       build.Result
	err         error
}

func developROMExportRects(layout developLayout) (export, reveal editor.Rect) {
	bezel := layout.Play.Bezel
	y := bezel.Y + bezel.H - 36
	export = editor.Rect{X: bezel.X + 16, Y: y, W: 134, H: 26}
	reveal = editor.Rect{X: bezel.X + bezel.W - 162, Y: y, W: 146, H: 26}
	return export, reveal
}

func (g *Game) drawDevelopROMExport(screen *ebiten.Image, layout developLayout) {
	export, reveal := developROMExportRects(layout)
	label := g.romExport.label
	if label == "" {
		label = "EXPORT .GBA"
	}
	g.runMu.Lock()
	busy := g.runBusy
	g.runMu.Unlock()
	if busy || g.romExport.ready != nil {
		drawDisabledTactileButton(screen, export, label, true)
	} else {
		drawDevelopTactileButton(screen, export, label, false, colorSelection, true)
	}
	if g.romExport.path != "" {
		drawDevelopTactileButton(screen, reveal, "SHOW IN FOLDER", false, colorSelection, true)
	}
}

func (g *Game) clickDevelopROMExport(layout developLayout, x, y int) bool {
	export, reveal := developROMExportRects(layout)
	switch {
	case rectContains(export, x, y):
		g.startDevelopROMExport()
	case g.romExport.path != "" && rectContains(reveal, x, y):
		if g.romExport.ready == nil {
			path := g.romExport.path
			g.startROMExportJob("", func(ctx context.Context) developROMExportResult {
				return developROMExportResult{err: revealDevelopROM(ctx, path)}
			})
		}
	default:
		return false
	}
	return true
}

func (g *Game) startDevelopROMExport() {
	g.runMu.Lock()
	busy := g.runBusy
	g.runMu.Unlock()
	if busy || g.romExport.ready != nil {
		return
	}
	projectDir := g.model.Dir
	g.startROMExportJob("CHOOSE FILE…", func(ctx context.Context) developROMExportResult {
		path, err := chooseDevelopExportROMPath(ctx, projectDir)
		return developROMExportResult{destination: path, err: err}
	})
}

func (g *Game) startROMExportJob(label string, work func(context.Context) developROMExportResult) {
	ready := make(chan developROMExportResult, 1)
	ctx, cancel := context.WithCancel(context.Background())
	g.romExport.ready, g.romExport.cancel, g.romExport.label = ready, cancel, label
	go func() { ready <- work(ctx) }()
}

func (g *Game) pumpDevelopROMExport() {
	select {
	case result := <-g.romExport.ready:
		g.romExport.cancel()
		g.romExport.ready, g.romExport.cancel, g.romExport.label = nil, nil, ""
		g.requestRender()
		automatic := errors.Is(result.err, errNoExportPicker)
		if result.err != nil && !automatic {
			g.setROMExportMessage("export: " + result.err.Error())
			return
		}
		if result.build.ROMPath != "" {
			g.romExport.path = result.build.ROMPath
			g.setROMExportMessage(fmt.Sprintf("Exported %s (%s)", result.build.ROMPath, developRendererName(result.build.Renderer.String())))
			return
		}
		if result.destination == "" && !automatic {
			return // Cancelled chooser or successful folder reveal.
		}
		if !g.prepareCodeForBuild() {
			g.setROMExportMessage("export blocked: " + g.statusMsg)
			return
		}
		if err := g.persist(); err != nil {
			g.setROMExportMessage("export blocked: " + g.statusMsg)
			return
		}
		projectDir := g.model.Dir
		g.setROMExportMessage("Building release cartridge…")
		g.startROMExportJob("EXPORTING…", func(context.Context) developROMExportResult {
			rom, err := buildDevelopReleaseROM(projectDir, result.destination)
			return developROMExportResult{build: rom, err: err}
		})
	default:
	}
}

func (g *Game) setROMExportMessage(message string) {
	g.romExport.message, g.statusMsg = message, message
}

func buildDevelopReleaseROM(projectDir, outputPath string) (build.Result, error) {
	service, err := build.NewService("", "")
	if err != nil {
		return build.Result{}, err
	}
	service.UseProjectRenderer = true
	if outputPath != "" {
		return service.Build(projectDir, outputPath)
	}
	path, err := createDevelopExportPath(projectDir)
	if err != nil {
		return build.Result{}, err
	}
	result, err := service.Build(projectDir, path)
	if err != nil {
		_ = os.Remove(filepath.Dir(path))
	}
	return result, err
}

func createDevelopExportPath(projectDir string) (string, error) {
	// A desktop without a save dialog can still export with one click. Each
	// export owns its directory so simultaneous builds never replace a ROM.
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(projectDir, "export-*")
	if err != nil {
		return "", fmt.Errorf("create ROM export directory: %w", err)
	}
	path, err := validateDevelopExportROMPath(developPreviewCacheDir(cache), filepath.Join(dir, "game.gba"))
	if err != nil {
		_ = os.Remove(dir)
	}
	return path, err
}
