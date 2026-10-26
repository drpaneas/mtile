//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestNamedPoseTargetDiagnostic compares named and numeric source in both
// native renderers, including the four flip combinations and an independent
// pixel oracle. The only source change is pspr(0,...) -> pspr("shared",...).
func TestNamedPoseTargetDiagnostic(t *testing.T) {
	out := os.Getenv("MTILE_POSE_NAMES_OUT")
	if out == "" {
		t.Skip("set MTILE_POSE_NAMES_OUT for native named-pose evidence")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(repository, "build", "player", "mtile-player")
	service, err := build.NewService("", filepath.Join(repository, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	const warmup, samples = 2, 120
	var baseline, hardware capture
	var targets []uint32
	rows := map[string]any{}
	var environment environmentReport
	var fixture poseFlipFixture
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0} {
		var control build.Result
		var controlCapture capture
		for _, form := range []string{"numeric", "named"} {
			dir := filepath.Join(out, renderer.String(), form)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			project := filepath.Join(dir, "project")
			fixture = writePoseFlipFixture(t, project, false)
			if form == "named" {
				path := filepath.Join(project, "game.lua")
				source, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				source = []byte(strings.ReplaceAll(string(source), "pspr(0,", `pspr("shared",`))
				if err := os.WriteFile(path, source, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if targets == nil {
				environment, err = benchmarkEnvironment(service, project, helper)
				if err != nil {
					t.Fatal(err)
				}
			}
			result, data, err := buildAndCaptureAtFrames(service, renderer, project, helper, dir, warmup, samples, targets)
			if err != nil {
				t.Fatal(err)
			}
			if targets == nil {
				targets = data.frameNumbers
				baseline = data
			}
			if err := validateSynchronizedFrames(targets, data.frameNumbers); err != nil {
				t.Fatal(err)
			}
			romSHA, err := fileFingerprint(result.ROMPath)
			if err != nil {
				t.Fatal(err)
			}
			hashes := make([]string, len(data.frames))
			for i, rgba := range data.frames {
				hashes[i] = fmt.Sprintf("%x", sha256.Sum256(rgba))
			}
			rows[renderer.String()+"-"+form] = map[string]any{
				"romSha256": romSHA, "romBytes": result.ROMSize, "generatedGoSha256": fmt.Sprintf("%x", sha256.Sum256([]byte(result.GeneratedGo))),
				"ewramBytes": result.EWRAMUsed, "iwramBytes": result.IWRAMUsed, "heapLinked": result.HeapLinked,
				"frame": objOptimizationCycleSummary(data.cycles, twoFrameCycleBudget), "draw": objOptimizationCycleSummary(data.draw, twoFrameCycleBudget), "commit": objOptimizationCycleSummary(data.commit, 67*1232),
				"frameNumbers": data.frameNumbers, "cycles": data.cycles, "frameSha256": hashes, "missedFrames": data.missed - data.warmupMissed,
			}
			if form == "numeric" {
				control, controlCapture = result, data
			} else {
				controlSHA, err := fileFingerprint(control.ROMPath)
				if err != nil {
					t.Fatal(err)
				}
				if result.GeneratedGo != control.GeneratedGo || romSHA != controlSHA || !reflect.DeepEqual(data.cycles, controlCapture.cycles) || compareFrames(data.frames, controlCapture.frames) != 0 {
					t.Fatalf("%s named/numeric source changed generated code, native ROM, cycles, or pixels", renderer)
				}
			}
			if renderer == transpiler.RendererMode0 && form == "named" {
				hardware = data
			}
		}
	}
	differences := compareFrames(baseline.frames, hardware.frames)
	oracleDifferences := 0
	for i, frame := range targets {
		expected, _ := poseFlipOracle(fixture, frame)
		oracleDifferences += poseFlipRGB5Differences(expected, baseline.frames[i]) + poseFlipRGB5Differences(expected, hardware.frames[i])
	}
	lcdDifferences, lcdScans := 0, 0
	s4, err := capturePresentWindow(helper, filepath.Join(out, "mode4", "named", "mode4.gba"), warmup+samples)
	if err != nil {
		t.Fatal(err)
	}
	s0, err := capturePresentWindow(helper, filepath.Join(out, "mode0", "named", "mode0.gba"), warmup+samples)
	if err != nil {
		t.Fatal(err)
	}
	for tick := warmup; tick < len(s0); tick++ {
		for _, scan := range s0[tick].Scans {
			lcdScans++
			lcdDifferences += countDifferingPixels(s4[tick].lastRGBA, scan.rgba)
		}
	}
	writePSprJSON(t, filepath.Join(out, "report.json"), map[string]any{
		"schemaVersion": 1, "environment": environment, "warmupFrames": warmup, "samples": samples,
		"differentPixels": differences, "oracleDifferentPixels": oracleDifferences, "lcdDifferentPixels": lcdDifferences, "lcdScans": lcdScans,
		"identicalGeneratedGo": true, "identicalNativeROMsWithinEachRenderer": true, "identicalCycleSamplesWithinEachRenderer": true, "renderers": rows,
		"fixture": "writePoseFlipFixture: negative offsets, authored flips, all call flips, shared pose and unequal animation bounds",
	})
	if differences != 0 || oracleDifferences != 0 || lcdDifferences != 0 || lcdScans < samples {
		t.Fatalf("pixels=%d oracle=%d LCD=%d scans=%d", differences, oracleDifferences, lcdDifferences, lcdScans)
	}
	t.Logf("%d frames and %d LCD scans match; named/numeric Go, ROMs and all cycle samples are identical in both renderers", samples, lcdScans)
}
