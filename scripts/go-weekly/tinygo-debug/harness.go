// Command harness builds and captures the isolated TinyGo mGBA experiment.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	compiler := flag.String("compiler", "", "isolated TinyGo wrapper")
	runtimeDir := flag.String("runtime", "", "mTile runtime")
	project := flag.String("project", "", "project to build")
	output := flag.String("out", "", "output ROM")
	target := flag.String("target", "", "only generate this target JSON")
	renderer := flag.String("renderer", "mode4", "renderer")
	debug := flag.Bool("debug", false, "enable normal mTile telemetry")
	player := flag.String("player", "", "native player")
	samples := flag.Int("samples", 0, "debug logical frames after twenty warmup updates")
	flag.Parse()
	service, err := build.NewService(*compiler, *runtimeDir)
	if err != nil {
		return err
	}
	if *target != "" {
		root, err := exec.Command(*compiler, "env", "TINYGOROOT").Output()
		if err != nil {
			return err
		}
		return build.GenerateTarget(strings.TrimSpace(string(root)), service.RuntimeDir, *target)
	}
	service.Debug, service.KeepWork = *debug, true
	service.Renderer, err = transpiler.ParseRenderer(*renderer)
	if err != nil {
		return err
	}
	start := time.Now()
	result, err := service.Build(*project, *output)
	if err != nil {
		return err
	}
	buildElapsed := time.Since(start)
	rom, err := os.ReadFile(result.ROMPath)
	if err != nil {
		return err
	}
	report := map[string]any{
		"romSHA256": fmt.Sprintf("%x", sha256.Sum256(rom)), "romBytes": result.ROMSize,
		"iwramBytes": result.IWRAMUsed, "ewramBytes": result.EWRAMUsed, "ewramFreeBytes": result.EWRAMFree,
		"generatedGoSHA256": fmt.Sprintf("%x", sha256.Sum256([]byte(result.GeneratedGo))),
		"buildSeconds":      buildElapsed.Seconds(), "workDir": result.WorkDir,
		"romPath": result.ROMPath, "mapPath": result.MapPath,
	}
	if *samples > 0 {
		session, err := emulator.Start(*player, result.ROMPath)
		if err != nil {
			return err
		}
		defer session.Close()
		type row struct {
			Tick     uint32              `json:"tick"`
			RGBA     string              `json:"rgbaSHA256"`
			Stats    emulator.DebugStats `json:"stats"`
			Watches  []emulator.Watch    `json:"watches"`
			Overflow bool                `json:"audioOverflow"`
		}
		rows := make([]row, 0, *samples)
		var lastTick uint32
		for step := 0; step < (*samples+20)*16+240 && len(rows) < *samples; step++ {
			frame, err := session.Step(0, false)
			if err != nil {
				return err
			}
			if !frame.Telemetry.DebugValid {
				continue
			}
			tick := frame.Telemetry.Stats.Frames
			if tick <= 20 || tick == lastTick {
				continue
			}
			lastTick = tick
			rows = append(rows, row{tick, fmt.Sprintf("%x", sha256.Sum256(frame.RGBA)), frame.Telemetry.Stats, frame.Telemetry.Watches, frame.Overflow})
		}
		if err := session.Close(); err != nil {
			return err
		}
		if len(rows) != *samples {
			return fmt.Errorf("captured %d/%d logical frames", len(rows), *samples)
		}
		report["rows"], report["logs"] = rows, session.DrainLogs()
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
