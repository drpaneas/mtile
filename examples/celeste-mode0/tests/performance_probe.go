//go:build ignore

// Diagnostic only: isolates draw passes in a temporary project; never edits the authored game.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
)

func main() {
	out, err := os.MkdirTemp("", "celeste-performance-probe-")
	checkProbe(err)
	project := os.Getenv("CELESTE_PERFORMANCE_PROJECT")
	if project == "" {
		project = "examples/celeste"
	}
	variants := []string{"full", "no-background", "no-ambient", "no-map", "no-player", "bare"}
	if len(os.Args) > 1 {
		variants = os.Args[1:]
	}
	for _, variant := range variants {
		dest := filepath.Join(out, variant)
		checkProbe(filepath.WalkDir(project, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(project, path)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if rel == "tests" {
					return filepath.SkipDir
				}
				return os.MkdirAll(filepath.Join(dest, rel), 0755)
			}
			if strings.HasSuffix(path, ".gba") || strings.HasSuffix(path, ".sav") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dest, rel), b, 0644)
		}))
		b, err := os.ReadFile(filepath.Join(dest, "game.lua"))
		checkProbe(err)
		code := string(b)
		for name, call := range map[string]string{"no-background": "update_parallax()", "no-ambient": "draw_ambient_particles()", "no-map": "map(1)", "no-player": "draw_player()"} {
			if variant == name || variant == "bare" {
				code = strings.ReplaceAll(code, "  "+call, "  -- probe omitted "+call)
			}
		}
		checkProbe(os.WriteFile(filepath.Join(dest, "game.lua"), []byte(code), 0644))
		if variant == "silent" {
			code = "function sfx() end\nfunction music() end\n" + code
		}

		if strings.HasPrefix(variant, "iwram") || variant == "rom" {
			names := []string{"update_ambient_particles", "draw_ambient_particles", "update_hair", "draw_hair"}
			if variant == "iwram-draw" {
				names = []string{"draw_ambient_particles", "draw_hair"}
			}
			if variant == "rom" {
				names = nil
			}
			if single, ok := map[string]string{"iwram-ambient": "draw_ambient_particles", "iwram-background": "update_parallax", "iwram-hair": "draw_hair", "iwram-smoke": "update_smoke"}[variant]; ok {
				names = []string{single}
			}
			path := filepath.Join(dest, "project.json")
			data, err := os.ReadFile(path)
			checkProbe(err)
			var config map[string]any
			checkProbe(json.Unmarshal(data, &config))
			config["iwramFunctions"] = names
			data, err = json.MarshalIndent(config, "", "  ")
			checkProbe(err)
			checkProbe(os.WriteFile(path, append(data, '\n'), 0644))
		}
		service, err := build.NewService("", "")
		checkProbe(err)
		service.Debug = true
		service.KeepWork = true
		service.UseProjectRenderer = true
		service.DisableConditionSpecialization = variant == "conditions-off"
		service.DisableStableWriteSpecialization = variant == "stores-off"
		result, err := service.Build(dest, filepath.Join(dest, "game.gba"))
		checkProbe(err)
		fmt.Printf("IWRAM %d/%d bytes\n", result.IWRAMUsed, build.IWRAMCapacity)

		session, err := emulator.Start("build/player/mtile-player", filepath.Join(dest, "game.gba"))
		checkProbe(err)
		var draw, work, update []int
		var last uint32
		var final emulator.DebugStats
		var firstError *emulator.DebugStats
		for i := 0; i < 600; i++ {
			frame, err := session.Step(0, false)
			checkProbe(err)
			if frame.Telemetry.DebugValid {
				st := frame.Telemetry.Stats
				final = st
				if st.HeapError != emulator.HeapOK && firstError == nil {
					copy := st
					firstError = &copy
				}
				if st.Frames != last && i > 100 {
					draw = append(draw, int(st.LastDraw))
					work = append(work, int(st.LastFrame))
					update = append(update, int(st.LastUpdate))
				}
				last = st.Frames
			}
		}
		session.Close()
		sort.Ints(draw)
		sort.Ints(work)
		sort.Ints(update)
		if len(draw) > 0 {
			fmt.Printf("%s drawP50=%d drawMax=%d workP50=%d workMax=%d updateP50=%d ticks=%d final=%+v firstError=%+v work=%s\n", variant, draw[len(draw)/2], draw[len(draw)-1], work[len(work)/2], work[len(work)-1], update[len(update)/2], len(draw), final, firstError, result.WorkDir)
		}
	}
	fmt.Println("Evidence:", out)
}
func checkProbe(err error) {
	if err != nil {
		panic(err)
	}
}
