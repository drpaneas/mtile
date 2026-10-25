//go:build ignore

// Native timing of the production map call with identical cached/fallback assets.
package main

import (
	"crypto/sha256"
	"fmt"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	out, err := os.MkdirTemp("", "celeste-map-raster-")
	checkMapProbe(err)
	type probeCase struct {
		camera     [2]int
		clip, draw string
	}
	cases := []probeCase{{camera: [2]int{0, 0}}, {camera: [2]int{0, 1}}, {camera: [2]int{1, 1}}, {camera: [2]int{2, 1}}, {camera: [2]int{3, 1}}}
	parity := len(os.Args) > 1 && os.Args[1] == "parity"
	if parity {
		cases = append(cases,
			probeCase{camera: [2]int{-3, -2}}, probeCase{camera: [2]int{-2, -1}}, probeCase{camera: [2]int{-1, 3}},
			probeCase{camera: [2]int{237, 159}}, probeCase{camera: [2]int{-239, -159}},
			probeCase{camera: [2]int{1, 1}, clip: "clip(1,3,232,153)"},
			probeCase{camera: [2]int{2, 1}, clip: "clip(2,5,13,41)"},
			probeCase{camera: [2]int{3, 1}, clip: "clip(239,159,1,1)"},
			probeCase{camera: [2]int{2, 3}, draw: "map(1,5,3,2,1,7,5,0)"},
			probeCase{camera: [2]int{-1, 2}, draw: "map(1,-1,-1,2,2,3,4,0)"})
	}
	for caseID, fixture := range cases {
		camera := fixture.camera
		cam, camY := camera[0], camera[1]
		if len(os.Args) > 1 && os.Args[1] == "full" && camY != 0 {
			continue
		}
		var baselineHash [32]byte
		var haveBaseline bool
		for _, cached := range []bool{false, true} {
			if len(os.Args) > 1 && os.Args[1] == "cached" && !cached {
				continue
			}
			dest := filepath.Join(out, fmt.Sprintf("case%02d-camera%d-%d-cache%v", caseID, cam, camY, cached))
			checkMapProbe(filepath.WalkDir("examples/celeste/assets", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, err := filepath.Rel("examples/celeste", path)
				if err != nil {
					return err
				}
				if d.IsDir() {
					return os.MkdirAll(filepath.Join(dest, rel), 0755)
				}
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dest, rel), b, 0644)
			}))
			background := "cls(0)"
			if parity {
				background = "camera()\nclip()\ncls(9)\nline(0,0,239,159,6)\nrectfill(60,30,100,60,3)"
			}
			drawMap := fixture.draw
			if drawMap == "" {
				drawMap = "map(1)"
			}
			source := fmt.Sprintf("function _init() end\nfunction _update() end\nfunction _draw()\n%s\ncamera(%d,%d)\n%s\n%s\nend\n", background, cam, camY, fixture.clip, drawMap)
			checkMapProbe(os.WriteFile(filepath.Join(dest, "game.lua"), []byte(source), 0644))
			service, err := build.NewService("", "")
			checkMapProbe(err)
			service.Debug = true
			service.KeepWork = true
			service.DisableMapRasterCache = !cached
			result, err := service.Build(dest, filepath.Join(dest, "game.gba"))
			checkMapProbe(err)
			session, err := emulator.Start("build/player/mtile-player", filepath.Join(dest, "game.gba"))
			checkMapProbe(err)
			var draw, work []int
			var last uint32
			var pixels []byte
			for i := 0; i < 360; i++ {
				f, e := session.Step(0, false)
				checkMapProbe(e)
				pixels = f.RGBA
				if f.Telemetry.DebugValid {
					st := f.Telemetry.Stats
					if st.Frames != last && i > 120 {
						draw = append(draw, int(st.LastDraw))
						work = append(work, int(st.LastFrame))
					}
					last = st.Frames
				}
			}
			session.Close()
			hash := sha256.Sum256(pixels)
			if !cached {
				baselineHash, haveBaseline = hash, true
			} else if haveBaseline && hash != baselineHash {
				panic(fmt.Sprintf("camera %v native RGBA mismatch: baseline %x, cached %x", camera, baselineHash, hash))
			}
			sort.Ints(draw)
			sort.Ints(work)
			fmt.Printf("case%d camera=%v cached%v drawP50=%d workP50=%d EWRAMfree=%d RGBA=%x work=%s\n", caseID, camera, cached, draw[len(draw)/2], work[len(work)/2], result.EWRAMFree, hash, result.WorkDir)
		}
	}
	fmt.Println("Evidence:", out)
}
func checkMapProbe(err error) {
	if err != nil {
		panic(err)
	}
}
