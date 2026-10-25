// Command verify builds and plays the real Lumen cartridge through mGBA,
// comparing forced Mode 0 and Mode 4 at every matching game frame.
//
// Run from the repository root:
//
//	go run ./examples/lumen/tests/verify.go -player build/player/mtile-player -out /tmp/lumen-verification
//
// It changes no project source or emulator memory: all actions use the same
// GBA buttons as a player. Separate copied-source fixtures start near clock
// rollover or with saved progress for an out-of-order dawn. The debug mailbox
// is read only, supplying position and state for steering and assertions.
// ROMs, screenshots and measurements stay in -out.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

const (
	flightCycleBudget     = 2 * 1232 * 228 // Two GBA LCD frames per 30 Hz update.
	mode0EWRAMFreeMinimum = 140 * 1024
	roomWidth             = 240
	roomCount             = 12
	worldWidth            = roomWidth * roomCount
)

type snapshot struct {
	Frame      uint32  `json:"frame"`
	State      int     `json:"state"`
	Age        float64 `json:"age"`
	PlayTicks  float64 `json:"playTicks"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	VX         float64 `json:"vx"`
	VY         float64 `json:"vy"`
	CameraX    float64 `json:"cameraX"`
	Facing     int     `json:"facing"`
	Carried    int     `json:"carried"`
	Lit        int     `json:"lit"`
	Health     int     `json:"health"`
	Hits       int     `json:"hits"`
	Checkpoint int     `json:"checkpoint"`
	DashTicks  int     `json:"dashTicks"`
}

type sample struct {
	Frame  uint32 `json:"frame"`
	State  int    `json:"state"`
	Cycles uint32 `json:"cycles"`
	Update uint32 `json:"updateCycles"`
	Draw   uint32 `json:"drawCycles"`
	Commit uint32 `json:"commitCycles"`
	Missed uint32 `json:"missedFrames"`
}

type checkpoint struct {
	Name  string   `json:"name"`
	State snapshot `json:"snapshot"`
}

type cycleSummary struct {
	Samples int    `json:"samples"`
	Median  uint32 `json:"median"`
	P95     uint32 `json:"p95"`
	Maximum uint32 `json:"maximum"`
}

type rendererEvidence struct {
	Renderer        string                 `json:"renderer"`
	Reason          string                 `json:"reason"`
	ROM             string                 `json:"rom"`
	ROMSHA256       string                 `json:"romSha256"`
	ROMBytes        int                    `json:"romBytes"`
	EWRAMFree       uint64                 `json:"ewramFreeBytes"`
	HeapLinked      bool                   `json:"heapLinked"`
	Backgrounds     int                    `json:"backgrounds"`
	OAMEntries      int                    `json:"oamEntries"`
	WorstLineCycles int                    `json:"worstLineCycles"`
	PoseMemory      build.PoseMemoryReport `json:"poseMemory"`
	FlightCycles    cycleSummary           `json:"flightCycles"`
	MissedFrames    uint32                 `json:"missedFrames"`
}

type parityReport struct {
	MatchedGameFrames int     `json:"matchedGameFrames"`
	ComparedPixels    uint64  `json:"comparedPixels"`
	DifferingPixels   uint64  `json:"differingPixels"`
	MinCameraX        float64 `json:"minCameraX"`
	MaxCameraX        float64 `json:"maxCameraX"`
	CameraPixelPhases uint8   `json:"cameraPixelPhases"`
	LeftFacingFrames  int     `json:"leftFacingFrames"`
	RightFacingFrames int     `json:"rightFacingFrames"`
	DashFrames        int     `json:"dashFrames"`
	DawnFrames        int     `json:"dawnFrames"`
	WorldWidth        int     `json:"worldWidth"`
	RoomWidth         int     `json:"roomWidth"`
	VisitedRooms      []int   `json:"visitedRooms"`
	VisitedRoomMask   uint16  `json:"visitedRoomMask"`
	RoomCrossings     int     `json:"roomCrossings"`
	MaximumMothX      float64 `json:"maximumMothX"`
}

type backgroundEvidence struct {
	Background int    `json:"background"`
	CameraCall string `json:"cameraCall"`
	MapCall    string `json:"mapCall"`
}

type rolloverReport struct {
	FixtureROM      string `json:"fixtureRom"`
	ObservedAges    []int  `json:"observedAges"`
	MatchedFrames   int    `json:"matchedFrames"`
	ComparedPixels  uint64 `json:"comparedPixels"`
	DifferingPixels uint64 `json:"differingPixels"`
}

type dawnOrderReport struct {
	FixtureROM      string       `json:"fixtureRom"`
	ReferenceROM    string       `json:"referenceRom"`
	Final           snapshot     `json:"final"`
	MatchedFrames   int          `json:"matchedFrames"`
	ComparedPixels  uint64       `json:"comparedPixels"`
	DifferingPixels uint64       `json:"differingPixels"`
	MissedFrames    uint32       `json:"missedFrames"`
	Cycles          cycleSummary `json:"cycles"`
}

type report struct {
	Success             bool                 `json:"success"`
	Failure             string               `json:"failure,omitempty"`
	Environment         string               `json:"environment"`
	ROM                 string               `json:"rom"`
	ROMSHA256           string               `json:"romSha256"`
	ROMBytes            int                  `json:"romBytes"`
	Renderer            string               `json:"renderer"`
	EWRAMFree           uint64               `json:"ewramFreeBytes"`
	EWRAMFreeMinimum    uint64               `json:"ewramFreeMinimumBytes"`
	HeapLinked          bool                 `json:"heapLinked"`
	LCDFrames           int                  `json:"lcdFrames"`
	StartupMissedFrames uint32               `json:"startupMissedFrames"`
	SteadyMissedFrames  uint32               `json:"steadyMissedFrames"`
	FlightMissedFrames  uint32               `json:"flightMissedFrames"`
	AudioNonSilent      bool                 `json:"audioNonSilent"`
	Checks              []string             `json:"checks"`
	Route               []checkpoint         `json:"route"`
	FlightCycles        cycleSummary         `json:"flightCycles"`
	FlightCycleBudget   uint32               `json:"flightCycleBudget"`
	LastStats           emulator.DebugStats  `json:"lastStats"`
	Samples             []sample             `json:"samples"`
	Mode0               rendererEvidence     `json:"mode0"`
	Mode4               rendererEvidence     `json:"mode4"`
	Parity              parityReport         `json:"parity"`
	Parallax            []backgroundEvidence `json:"parallax"`
	ParallaxGIF         string               `json:"parallaxGif,omitempty"`
	Rollover            *rolloverReport      `json:"rollover,omitempty"`
	OutOfOrderDawn      *dawnOrderReport     `json:"outOfOrderDawn,omitempty"`
	SourceSHA256        map[string]string    `json:"sourceSha256"`
}

type player struct {
	session         *emulator.Session
	frame           emulator.Frame
	current         snapshot
	report          *report
	out             string
	reference       *player
	recording       bool
	clipTicks       int
	clip            [][]byte
	clipLimit       int
	strictDeadlines bool
	previousRoom    int
}

func main() {
	_, source, _, _ := runtime.Caller(0)
	defaultProject := filepath.Clean(filepath.Join(filepath.Dir(source), ".."))
	helper := flag.String("player", "build/player/mtile-player", "headless mTile mGBA helper")
	out := flag.String("out", "/tmp/lumen-verification", "ROM, screenshots and JSON report directory")
	project := flag.String("project", defaultProject, "Lumen project directory")
	flag.Parse()
	if err := run(*project, *helper, *out); err != nil {
		fmt.Fprintln(os.Stderr, "lumen verification:", err)
		os.Exit(1)
	}
}

func run(project, helper, output string) (runErr error) {
	out, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	helper, err = filepath.Abs(helper)
	if err != nil {
		return err
	}
	service, err := build.NewService("", "")
	if err != nil {
		return err
	}
	digests, err := sourceDigests(project)
	if err != nil {
		return err
	}
	service.Debug, service.Renderer = true, transpiler.RendererMode0
	fmt.Println("Building forced Mode 0 and Mode 4 cartridges with read-only debug telemetry...")
	result, err := service.Build(project, filepath.Join(out, "lumen.gba"))
	if err != nil {
		return fmt.Errorf("forced Mode 0 build: %w", err)
	}
	service.Renderer = transpiler.RendererMode4
	referenceResult, err := service.Build(project, filepath.Join(out, "lumen-mode4.gba"))
	if err != nil {
		return fmt.Errorf("forced Mode 4 reference build: %w", err)
	}
	referenceROM, err := os.ReadFile(referenceResult.ROMPath)
	if err != nil {
		return err
	}
	rom, err := os.ReadFile(result.ROMPath)
	if err != nil {
		return err
	}
	r := &report{
		Environment: "native mGBA helper; forced Mode 0 and Mode 4 ARM ROMs; identical D-pad/A/B/Start input per matching 30 Hz game frame; no memory writes or gameplay hooks",
		ROM:         result.ROMPath, ROMSHA256: fmt.Sprintf("%x", sha256.Sum256(rom)),
		ROMBytes: result.ROMSize, Renderer: result.Renderer.String(), EWRAMFree: result.EWRAMFree, HeapLinked: result.HeapLinked,
		EWRAMFreeMinimum:  mode0EWRAMFreeMinimum,
		SourceSHA256:      digests,
		FlightCycleBudget: flightCycleBudget,
		Mode0:             evidence(result, rom), Mode4: evidence(referenceResult, referenceROM),
	}
	referenceReport := &report{}
	defer func() {
		r.Success = runErr == nil
		if runErr != nil {
			r.Failure = runErr.Error()
		}
		var cycles []uint32
		for _, value := range r.Samples {
			if value.State == 1 {
				cycles = append(cycles, value.Cycles)
			}
		}
		r.FlightCycles = summarize(cycles)
		r.Mode0.FlightCycles = r.FlightCycles
		cycles = cycles[:0]
		for _, value := range referenceReport.Samples {
			if value.State == 1 {
				cycles = append(cycles, value.Cycles)
			}
		}
		r.Mode4.FlightCycles = summarize(cycles)
		r.Mode0.MissedFrames = r.LastStats.MissedFrames - r.StartupMissedFrames
		r.Mode4.MissedFrames = referenceReport.LastStats.MissedFrames - referenceReport.StartupMissedFrames
		r.SteadyMissedFrames = r.LastStats.MissedFrames - r.StartupMissedFrames
		data, err := json.MarshalIndent(r, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(out, "report.json"), append(data, '\n'), 0o644)
		}
		if runErr == nil && err != nil {
			runErr = err
		}
	}()
	r.Parallax, err = parallaxEvidence(project)
	if err != nil {
		return err
	}
	session, err := emulator.Start(helper, result.ROMPath)
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	referenceSession, err := emulator.Start(helper, referenceResult.ROMPath)
	if err != nil {
		return err
	}
	defer func() { _ = referenceSession.Close() }()
	p := &player{session: session, report: r, out: out, clipLimit: 75,
		reference: &player{session: referenceSession, report: referenceReport, out: out}}
	if err := p.expect(result.Renderer == transpiler.RendererMode0 && result.RendererReason == "M0_FORCED_PROVEN" &&
		result.RendererLayers == 4 && result.RendererObjects,
		"forced Mode 0 proves four hardware backgrounds and objects"); err != nil {
		return err
	}
	if err := p.expect(result.PoseMemory.PoseCount > 0 && result.PoseMemory.ClipCount > 0,
		"authored poses and named animation clips are linked into the cartridge"); err != nil {
		return err
	}
	if err := p.expect(result.PoseMemory.ResidentVRAMBytes+result.PoseMemory.StageRAMBytes > 0,
		"the linked cartridge allocates hardware pose graphics"); err != nil {
		return err
	}
	if err := p.expect(len(r.Parallax) == 4,
		"four ordered map passes use distinct authored camera formulas recorded in the report"); err != nil {
		return err
	}
	var initialTitle [][]byte
	for range 45 {
		if err := p.advance(0); err != nil {
			return err
		}
		if len(initialTitle) < 8 {
			if p.current.Age != float64(len(initialTitle)+1) {
				return fmt.Errorf("initial title age is %v, want %d", p.current.Age, len(initialTitle)+1)
			}
			initialTitle = append(initialTitle, append([]byte(nil), p.frame.RGBA...))
		}
	}
	if err := p.expect(p.current.State == 0, "title screen is available"); err != nil {
		return err
	}
	r.StartupMissedFrames = r.LastStats.MissedFrames
	referenceReport.StartupMissedFrames = referenceReport.LastStats.MissedFrames
	p.strictDeadlines = true
	if err := p.expect(!r.HeapLinked, "allocation analysis marks the cartridge heap-free"); err != nil {
		return err
	}
	if err := p.expect(result.EWRAMFree >= mode0EWRAMFreeMinimum,
		"the linked Mode 0 cartridge leaves at least 140 KiB of EWRAM free"); err != nil {
		return err
	}
	if err := p.screenshot("title.png"); err != nil {
		return err
	}
	p.mark("title")
	if err := p.tap(emulator.KeyA); err != nil {
		return err
	}
	if err := p.expect(p.current.State == 1 && p.current.Health == 3, "A begins a healthy journey"); err != nil {
		return err
	}

	// Pausing must freeze gameplay even while directional and action keys are held.
	if err := p.tap(emulator.KeyStart); err != nil {
		return err
	}
	if err := p.expect(p.current.State == 2, "Start pauses"); err != nil {
		return err
	}
	frozen := p.current
	for range 30 {
		if err := p.advance(emulator.KeyRight | emulator.KeyDown | emulator.KeyB); err != nil {
			return err
		}
		if p.current.State != 2 || p.current.PlayTicks != frozen.PlayTicks || p.current.X != frozen.X || p.current.Y != frozen.Y {
			return fmt.Errorf("pause changed game time or position: before %+v; after %+v", frozen, p.current)
		}
	}
	r.Checks = append(r.Checks, "pause freezes play_ticks, px and py through 30 game updates with held input")
	if err := p.tap(emulator.KeyStart); err != nil {
		return err
	}
	if err := p.expect(p.current.State == 1, "Start resumes"); err != nil {
		return err
	}
	p.mark("pause verified")

	embers := [6][3][2]float64{
		{{42, 72}, {210, 40}, {346, 94}},
		{{538, 46}, {681, 98}, {836, 32}},
		{{1010, 100}, {1144, 48}, {1303, 83}},
		{{1508, 36}, {1656, 90}, {1805, 52}},
		{{1964, 85}, {2113, 30}, {2270, 104}},
		{{2462, 53}, {2605, 99}, {2774, 41}},
	}
	lanterns := [6][2]float64{{416, 87}, {896, 72}, {1376, 91}, {1856, 64}, {2336, 88}, {2816, 80}}
	for ember, position := range embers[0] {
		if ember == 1 {
			if err := p.cruiseTo(120); err != nil {
				return err
			}
			if err := p.advance(emulator.KeyRight | emulator.KeyA); err != nil {
				return err
			}
			if err := p.advance(emulator.KeyRight); err != nil {
				return err
			}
			r.Checks = append(r.Checks, "A dash is exercised during the first flight")
		}
		if err := p.collect(position[0], position[1], ember+1, ember+1); err != nil {
			return err
		}
	}
	if err := p.light(lanterns[0][0], lanterns[0][1], 1); err != nil {
		return err
	}
	if err := p.screenshot("gameplay.png"); err != nil {
		return err
	}
	if err := p.screenshot("rooms-01-02.png"); err != nil {
		return err
	}

	// Chase the first moving shadow, using its documented orbit and the public
	// age watch. Three real collisions test the resting and checkpoint path.
	if err := p.cruiseTo(130); err != nil {
		return err
	}
	hitsBefore := p.current.Hits
	for count := 0; p.current.State == 1 && count < 700; count++ {
		age := p.current.Age + 1
		x := 130 - math.Sin(age/180*2*math.Pi)*22
		y := 65 + math.Cos(age/150*2*math.Pi)*12
		if err := p.advance(p.steer(x, y)); err != nil {
			return err
		}
	}
	if err := p.expect(p.current.State == 3 && p.current.Health == 0 && p.current.Hits >= hitsBefore+3,
		"three shadow collisions enter the resting screen"); err != nil {
		return err
	}
	p.mark("three collisions")
	if err := p.tap(emulator.KeyA); err != nil {
		return err
	}
	if err := p.expect(p.current.State == 1 && p.current.Health == 3 && p.current.Lit == 1 &&
		p.current.Checkpoint == 1 && math.Abs(p.current.X-416) < 1 && math.Abs(p.current.Y-69) < 1,
		"A recovers at the restored lantern and preserves progress"); err != nil {
		return err
	}
	p.mark("checkpoint recovery")
	// Retain existing captures during an ordinary flight. Recording introduces
	// no emulator steps, input changes or game hooks.
	p.recording = true

	for sector := 1; sector < len(embers); sector++ {
		if sector == 3 || sector == 5 {
			// Retain later gardens during the same ordinary route, without
			// adding emulator steps or changing any player input.
			p.recording = true
			p.clipTicks = 0
			p.clipLimit += 75
		}
		for ember, position := range embers[sector] {
			if err := p.collect(position[0], position[1], ember+1, sector*3+ember+1); err != nil {
				return err
			}
		}
		if sector == len(embers)-1 {
			// Visit the entire last screen before kindling the final lantern.
			// Ordinary flight must render the far edge, rather than dawn moving
			// the camera there for the first time.
			if err := p.cruiseTo(worldWidth - 12); err != nil {
				return err
			}
			for range 30 {
				if err := p.advance(0); err != nil {
					return err
				}
			}
			if err := p.screenshot("right-edge.png"); err != nil {
				return err
			}
			p.mark("full twelve-room camera extent")
		}
		if err := p.light(lanterns[sector][0], lanterns[sector][1], sector+1); err != nil {
			return err
		}
		if err := p.screenshot(fmt.Sprintf("rooms-%02d-%02d.png", sector*2+1, sector*2+2)); err != nil {
			return err
		}
		if sector == 1 {
			for range 20 {
				if err := p.advance(0); err != nil {
					return err
				}
			}
			if err := p.screenshot("scenic-gameplay.png"); err != nil {
				return err
			}
			p.recording = false
		}
		if sector == 3 || sector == 5 {
			p.recording = false
		}
		if sector == 4 {
			// Backtrack through already restored gardens, then lose three
			// wings at the first room's real moving shadow. Recovering at
			// lantern five exercises a two-thousand-pixel camera return.
			for _, x := range []float64{2000, 1600, 1200, 800, 400, 130} {
				if err := p.cruiseTo(x); err != nil {
					return err
				}
			}
			hitsBefore := p.current.Hits
			for count := 0; p.current.State == 1 && count < 700; count++ {
				age := p.current.Age + 1
				x := 130 - math.Sin(age/180*2*math.Pi)*22
				y := 65 + math.Cos(age/150*2*math.Pi)*12
				if err := p.advance(p.steer(x, y)); err != nil {
					return err
				}
			}
			if err := p.expect(p.current.State == 3 && p.current.Health == 0 && p.current.Hits >= hitsBefore+3,
				"three further shadow collisions enter rest after traversing the garden in reverse"); err != nil {
				return err
			}
			if err := p.tap(emulator.KeyA); err != nil {
				return err
			}
			if err := p.expect(p.current.State == 1 && p.current.Health == 3 && p.current.Lit == 5 &&
				p.current.Checkpoint == 5 && math.Abs(p.current.X-2336) < 1 && math.Abs(p.current.Y-70) < 1,
				"A recovers at lantern five and preserves all five restored lanterns"); err != nil {
				return err
			}
			p.mark("long checkpoint return begins")
			for ticks := 0; math.Abs(p.current.CameraX-2224) >= 1 && ticks < 500; ticks++ {
				if err := p.advance(0); err != nil {
					return err
				}
				if math.Abs(p.current.X-2336) >= 1 || math.Abs(p.current.Y-70) >= 1 {
					return fmt.Errorf("long checkpoint return moved the moth before its camera arrived: %+v", p.current)
				}
			}
			if err := p.expect(math.Abs(p.current.CameraX-2224) < 1 && p.current.State == 1,
				"the long checkpoint return reaches its camera target without moving the moth or missing deadlines"); err != nil {
				return err
			}
			p.mark("long checkpoint return complete")
		}
	}
	if err := p.writeGIF("parallax.gif"); err != nil {
		return err
	}
	if err := p.expect(p.current.State == 4 && p.current.Lit == 6 && p.current.Carried == 0,
		"eighteen gathered embers restore all six lanterns and trigger dawn"); err != nil {
		return err
	}
	for range 145 {
		if err := p.advance(0); err != nil {
			return err
		}
	}
	if err := p.expect(p.current.State == 4, "dawn remains on screen through its palette transition"); err != nil {
		return err
	}
	if err := p.screenshot("dawn.png"); err != nil {
		return err
	}
	p.mark("full dawn")
	if err := p.tap(emulator.KeyStart); err != nil {
		return err
	}
	// The authored restart first presents the night sky, then selects the
	// prepared terrain bank on the following update. Let that final update
	// publish with released input, while retaining all deadline/parity checks.
	if err := p.advance(0); err != nil {
		return err
	}
	if err := p.expect(p.current.State == 1 && p.current.Lit == 0 && p.current.Carried == 0 &&
		p.current.Health == 3 && p.current.Hits == 0 && p.current.Checkpoint == 0 &&
		p.current.PlayTicks <= 2 && math.Abs(p.current.X-32) < 1 && math.Abs(p.current.Y-88) < 1,
		"Start after dawn resets position, time, health, embers and lanterns"); err != nil {
		return err
	}
	// The same first ember being collectible proves its taken flag was reset.
	if err := p.collect(42, 72, 1, 1); err != nil {
		return err
	}
	r.Checks = append(r.Checks, "restart makes previously collected embers collectible again")
	if err := p.expect(r.AudioNonSilent, "the actual ROM produces audible PSG samples"); err != nil {
		return err
	}
	if err := p.expect(r.LastStats.GCCount == 0, "the complete journey performs no garbage collections"); err != nil {
		return err
	}
	var flightCycles []uint32
	for _, value := range r.Samples {
		if value.State == 1 {
			flightCycles = append(flightCycles, value.Cycles)
		}
	}
	flight := summarize(flightCycles)
	if err := p.expect(flight.Maximum <= flightCycleBudget,
		fmt.Sprintf("all %d flight updates fit the 561792-cycle 30 Hz budget (maximum %d)", flight.Samples, flight.Maximum)); err != nil {
		return err
	}
	if err := p.expect(r.FlightMissedFrames == 0, "flight misses no frame deadlines"); err != nil {
		return err
	}
	if err := p.expect(r.LastStats.MissedFrames == r.StartupMissedFrames,
		"the complete journey, including dawn and restart, misses no frame deadlines after startup"); err != nil {
		return err
	}
	if err := p.expect(r.Parity.DifferingPixels == 0 && r.Parity.MatchedGameFrames > 500,
		fmt.Sprintf("Mode 0 and Mode 4 match every pixel through %d game frames", r.Parity.MatchedGameFrames)); err != nil {
		return err
	}
	if err := p.expect(r.Parity.MaxCameraX-r.Parity.MinCameraX > worldWidth-roomWidth-1 &&
		r.Parity.VisitedRoomMask == (1<<roomCount)-1 && r.Parity.MaximumMothX > worldWidth-20 &&
		r.Parity.CameraPixelPhases == 255 &&
		r.Parity.LeftFacingFrames > 0 && r.Parity.RightFacingFrames > 0 && r.Parity.DashFrames > 0 && r.Parity.DawnFrames > 120,
		"parity covers all twelve rooms, the full 2640-pixel camera range, all eight sub-tile camera phases, both moth facings, dash and dawn"); err != nil {
		return err
	}
	p.mark("restart verified")
	r.Rollover, err = verifyRollover(service, project, helper, out, initialTitle)
	if err != nil {
		return err
	}
	if err := p.expect(r.Rollover.MatchedFrames == len(initialTitle) && r.Rollover.DifferingPixels == 0,
		"a copied cartridge wraps age from 28799 to 0, then matches the original title at ages 1 through 8"); err != nil {
		return err
	}
	r.OutOfOrderDawn, err = verifyOutOfOrderDawn(service, project, helper, out)
	if err != nil {
		return err
	}
	if err := p.expect(r.OutOfOrderDawn.Final.State == 4 && r.OutOfOrderDawn.DifferingPixels == 0 &&
		r.OutOfOrderDawn.MissedFrames == 0 && r.OutOfOrderDawn.Cycles.Maximum <= flightCycleBudget,
		"a copied out-of-order cartridge restores lantern one last and celebrates its local dawn with exact pixels and no missed deadlines"); err != nil {
		return err
	}
	fmt.Printf("PASS: twelve rooms, eighteen embers, six lanterns, pause, six hits, long checkpoint recovery, both dawn orders and restart; artifacts %s\n", out)
	return nil
}

func (p *player) advance(keys uint16) error {
	limit := 40
	if p.current.Frame == 0 {
		// Packed map preparation happens once before the generated game loop;
		// it has no publications yet and is outside steady-state frame timing.
		limit = 2400
	}
	for range limit {
		frame, err := p.session.Step(keys, false)
		if err != nil {
			return err
		}
		p.report.LCDFrames++
		if frame.Overflow {
			return fmt.Errorf("audio overflow at LCD frame %d", frame.Number)
		}
		for _, value := range frame.PCM {
			if value != 0 {
				p.report.AudioNonSilent = true
				break
			}
		}
		if !frame.Telemetry.DebugValid || frame.Telemetry.Stats.Frames <= p.current.Frame {
			continue
		}
		stats := frame.Telemetry.Stats
		if stats.HeapError != emulator.HeapOK {
			return fmt.Errorf("cartridge heap error %d", stats.HeapError)
		}
		values := make(map[string]float64, len(frame.Telemetry.Watches))
		for _, watch := range frame.Telemetry.Watches {
			values[watch.Name] = float64(watch.Raw) / 65536
		}
		for _, name := range []string{"state", "age", "play_ticks", "px", "py", "vx", "vy", "view_x", "facing", "carried", "lit_count", "health", "hits", "checkpoint", "dash_ticks"} {
			if _, ok := values[name]; !ok {
				return fmt.Errorf("required debug watch %q is unavailable; watches: %+v", name, frame.Telemetry.Watches)
			}
		}
		p.current = snapshot{Frame: stats.Frames, State: int(values["state"]), Age: values["age"], PlayTicks: values["play_ticks"],
			X: values["px"], Y: values["py"], VX: values["vx"], VY: values["vy"], CameraX: values["view_x"], Facing: int(values["facing"]), Carried: int(values["carried"]),
			Lit: int(values["lit_count"]), Health: int(values["health"]), Hits: int(values["hits"]), Checkpoint: int(values["checkpoint"]), DashTicks: int(values["dash_ticks"])}
		if p.current.State == 1 {
			p.report.FlightMissedFrames += stats.MissedFrames - p.report.LastStats.MissedFrames
		}
		p.frame, p.report.LastStats = frame, stats
		if p.recording {
			if p.clipTicks%2 == 0 && len(p.clip) < p.clipLimit {
				p.clip = append(p.clip, append([]byte(nil), frame.RGBA...))
			}
			p.clipTicks++
		}
		p.report.Samples = append(p.report.Samples, sample{stats.Frames, p.current.State, stats.LastFrame, stats.LastUpdate, stats.LastDraw, stats.LastBlit, stats.MissedFrames})
		if p.reference != nil {
			if err := p.reference.advance(keys); err != nil {
				return fmt.Errorf("Mode 4 reference: %w", err)
			}
			if err := p.compare(); err != nil {
				return err
			}
		}
		if p.strictDeadlines && stats.MissedFrames != p.report.StartupMissedFrames {
			return fmt.Errorf("Mode 0 missed a poststartup deadline at game frame %d: total %d, startup %d; state %+v, cycles %d", stats.Frames, stats.MissedFrames, p.report.StartupMissedFrames, p.current, stats.LastFrame)
		}
		return nil
	}
	return fmt.Errorf("no new debug publication after %d LCD frames; last state %+v", limit, p.current)
}

func (p *player) tap(key uint16) error {
	for _, keys := range []uint16{0, key, 0} {
		if err := p.advance(keys); err != nil {
			return err
		}
	}
	return nil
}

func (p *player) steer(x, y float64) uint16 {
	// Brake before the target to account for the game's gentle flight inertia.
	dx, dy := x-p.current.X-p.current.VX*4, y-p.current.Y-p.current.VY*4
	var keys uint16
	if dx < -1.5 {
		keys |= emulator.KeyLeft
	}
	if dx > 1.5 {
		keys |= emulator.KeyRight
	}
	if dy < -1.5 {
		keys |= emulator.KeyUp
	}
	if dy > 1.5 {
		keys |= emulator.KeyDown
	}
	return keys
}

func (p *player) moveTo(x, y float64) error {
	for range 420 {
		if p.current.State != 1 {
			return fmt.Errorf("flight interrupted while steering to (%.0f, %.0f): %+v", x, y, p.current)
		}
		if math.Abs(p.current.X-x) < 4 && math.Abs(p.current.Y-y) < 4 && math.Abs(p.current.VX) < .8 && math.Abs(p.current.VY) < .8 {
			return nil
		}
		if err := p.advance(p.steer(x, y)); err != nil {
			return err
		}
	}
	return fmt.Errorf("could not reach (%.0f, %.0f): %+v", x, y, p.current)
}

func (p *player) cruiseTo(x float64) error {
	// All authored shadow orbits stay at y >= 33. Holding Up reaches the
	// actual y=24 world clamp, keeping the long horizontal route above their
	// nine-pixel collision height without weakening ordinary game physics.
	for range 420 {
		if p.current.State != 1 {
			return fmt.Errorf("flight interrupted while cruising to x=%.0f: %+v", x, p.current)
		}
		if math.Abs(p.current.X-x) < 4 && p.current.Y == 24 && math.Abs(p.current.VX) < .8 {
			return nil
		}
		travelX := x
		if p.current.Y != 24 {
			// Climb before travelling, so leaving a lantern does not cut
			// diagonally through the neighboring room's shadow orbit.
			travelX = p.current.X
		}
		if err := p.advance(p.steer(travelX, 24) | emulator.KeyUp); err != nil {
			return err
		}
	}
	return fmt.Errorf("could not cruise to x=%.0f: %+v", x, p.current)
}

func (p *player) collect(x, y float64, carried, number int) error {
	if err := p.cruiseTo(x); err != nil {
		return err
	}
	if err := p.moveTo(x, y); err != nil {
		return err
	}
	if err := p.expect(p.current.Carried == carried, fmt.Sprintf("ember %d gathered (carried %d)", number, carried)); err != nil {
		return err
	}
	p.mark(fmt.Sprintf("ember %d", number))
	return nil
}

func (p *player) light(x, y float64, number int) error {
	if err := p.cruiseTo(x); err != nil {
		return err
	}
	if err := p.moveTo(x, y); err != nil {
		return err
	}
	if err := p.tap(emulator.KeyB); err != nil {
		return err
	}
	if err := p.expect(p.current.Lit == number && p.current.Carried == 0 && p.current.Health == 3,
		fmt.Sprintf("B restores lantern %d, spends three embers and heals", number)); err != nil {
		return err
	}
	p.mark(fmt.Sprintf("lantern %d", number))
	return nil
}

func (p *player) expect(ok bool, label string) error {
	if !ok {
		return fmt.Errorf("%s: assertion failed at %+v", label, p.current)
	}
	p.report.Checks = append(p.report.Checks, label)
	fmt.Println("  verified:", label)
	return nil
}

func (p *player) mark(name string) {
	p.report.Route = append(p.report.Route, checkpoint{name, p.current})
}

func (p *player) screenshot(name string) error {
	if p.reference != nil {
		if err := p.reference.screenshot("mode4-" + name); err != nil {
			return err
		}
	}
	// Nearest-neighbour 3x: every captured LCD pixel becomes one 3x3 square.
	picture := image.NewRGBA(image.Rect(0, 0, emulator.Width*3, emulator.Height*3))
	for y := range emulator.Height * 3 {
		for x := range emulator.Width * 3 {
			from := (y/3*emulator.Width + x/3) * 4
			to := y*picture.Stride + x*4
			copy(picture.Pix[to:to+4], p.frame.RGBA[from:from+4])
		}
	}
	file, err := os.Create(filepath.Join(p.out, name))
	if err != nil {
		return err
	}
	err = png.Encode(file, picture)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (p *player) writeGIF(name string) error {
	if len(p.clip) == 0 {
		return fmt.Errorf("no native flight frames retained for parallax GIF")
	}
	// Every pixel is from the cartridge's native framebuffer. Build its exact
	// RGB palette, then enlarge with nearest-neighbour sampling for display.
	var palette color.Palette
	indices := map[uint32]uint8{}
	for _, frame := range p.clip {
		for i := 0; i < len(frame); i += 4 {
			key := uint32(frame[i])<<16 | uint32(frame[i+1])<<8 | uint32(frame[i+2])
			if _, exists := indices[key]; exists {
				continue
			}
			if len(palette) == 256 {
				return fmt.Errorf("native parallax GIF exceeds the 256-color palette")
			}
			indices[key] = uint8(len(palette))
			palette = append(palette, color.NRGBA{frame[i], frame[i+1], frame[i+2], 255})
		}
	}
	animation := &gif.GIF{LoopCount: 0}
	for tick, frame := range p.clip {
		picture := image.NewPaletted(image.Rect(0, 0, emulator.Width*3, emulator.Height*3), palette)
		for y := range emulator.Height * 3 {
			for x := range emulator.Width * 3 {
				from := (y/3*emulator.Width + x/3) * 4
				key := uint32(frame[from])<<16 | uint32(frame[from+1])<<8 | uint32(frame[from+2])
				picture.Pix[y*picture.Stride+x] = indices[key]
			}
		}
		animation.Image = append(animation.Image, picture)
		// One capture per two 30 Hz updates: 7+7+6 centiseconds per three
		// images preserves the actual average 15 fps in GIF's time units.
		delay := 7
		if tick%3 == 2 {
			delay = 6
		}
		animation.Delay = append(animation.Delay, delay)
	}
	path := filepath.Join(p.out, name)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	err = gif.EncodeAll(file, animation)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	p.report.ParallaxGIF = path
	return nil
}

func evidence(result build.Result, rom []byte) rendererEvidence {
	return rendererEvidence{
		Renderer: result.Renderer.String(), Reason: result.RendererReason,
		ROM: result.ROMPath, ROMSHA256: fmt.Sprintf("%x", sha256.Sum256(rom)),
		ROMBytes: result.ROMSize, EWRAMFree: result.EWRAMFree, HeapLinked: result.HeapLinked,
		Backgrounds: result.RendererLayers, OAMEntries: result.RendererOAMEntries,
		WorstLineCycles: result.RendererLineCycles, PoseMemory: result.PoseMemory,
	}
}

func sourceDigests(project string) (map[string]string, error) {
	paths, err := filepath.Glob(filepath.Join(project, "src", "*.lua"))
	if err != nil {
		return nil, err
	}
	paths = append(paths, filepath.Join(project, "game.lua"), filepath.Join(project, "project.json"))
	result := make(map[string]string, len(paths))
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(project, path)
		if err != nil {
			return nil, err
		}
		result[filepath.ToSlash(relative)] = fmt.Sprintf("%x", sha256.Sum256(contents))
	}
	return result, nil
}

func verifyOutOfOrderDawn(service build.Service, project, helper, out string) (*dawnOrderReport, error) {
	fixture, err := os.MkdirTemp("", "lumen-out-of-order-project-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(fixture) }()
	if err := os.CopyFS(fixture, os.DirFS(project)); err != nil {
		return nil, err
	}
	statePath := filepath.Join(fixture, "src", "state.lua")
	state, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}
	// This saved-state fixture leaves the first lantern dark, with its three
	// embers carried. Starting the camera farther along the garden also
	// stresses simultaneous terrain motion and the authored sunrise stages.
	replacements := map[string]string{
		"state = 0 --":         "state = 1 --",
		"\npx = 32\n":          "\npx = 416\n",
		"\npy = 88\n":          "\npy = 87\n",
		"\nview_x = 0\n":       "\nview_x = 1000\n",
		"\ncarried = 0\n":      "\ncarried = 3\n",
		"\nlit_count = 0\n":    "\nlit_count = 5\n",
		"\ncheckpoint = 0\n":   "\ncheckpoint = 6\n",
		"\ninvulnerable = 0\n": "\ninvulnerable = 90\n",
		"\nember_mask = 0\n":   "\nember_mask = 511\n",
		"\nember_mask2 = 0\n":  "\nember_mask2 = 511\n",
		"\nlantern_mask = 0\n": "\nlantern_mask = 62\n",
	}
	text := string(state)
	for from, to := range replacements {
		if strings.Count(text, from) != 1 {
			return nil, fmt.Errorf("out-of-order fixture needs exactly one initial assignment %q", from)
		}
		text = strings.Replace(text, from, to, 1)
	}
	if err := os.WriteFile(statePath, []byte(text), 0o644); err != nil {
		return nil, err
	}
	service.Renderer = transpiler.RendererMode0
	result, err := service.Build(fixture, filepath.Join(out, "lumen-out-of-order.gba"))
	if err != nil {
		return nil, fmt.Errorf("out-of-order Mode 0 fixture build: %w", err)
	}
	service.Renderer = transpiler.RendererMode4
	referenceResult, err := service.Build(fixture, filepath.Join(out, "lumen-out-of-order-mode4.gba"))
	if err != nil {
		return nil, fmt.Errorf("out-of-order Mode 4 fixture build: %w", err)
	}
	session, err := emulator.Start(helper, result.ROMPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = session.Close() }()
	reference, err := emulator.Start(helper, referenceResult.ROMPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reference.Close() }()
	r := &report{}
	p := &player{session: session, report: r, out: out,
		reference: &player{session: reference, report: &report{}, out: out}}
	for range 45 {
		if err := p.advance(0); err != nil {
			return nil, err
		}
	}
	r.StartupMissedFrames = r.LastStats.MissedFrames
	p.strictDeadlines = true
	if p.current.State != 1 || p.current.Carried != 3 || p.current.Lit != 5 {
		return nil, fmt.Errorf("out-of-order fixture did not start with five lit lanterns and three carried embers: %+v", p.current)
	}
	if err := p.tap(emulator.KeyB); err != nil {
		return nil, err
	}
	if p.current.State != 4 || p.current.Lit != 6 || p.current.Carried != 0 || p.current.Checkpoint != 1 {
		return nil, fmt.Errorf("out-of-order fixture did not restore lantern one last: %+v", p.current)
	}
	for range 145 {
		if err := p.advance(0); err != nil {
			return nil, err
		}
	}
	if p.current.State != 4 || math.Abs(p.current.X-416) >= 1 || math.Abs(p.current.CameraX-296) >= 1 || r.LastStats.GCCount != 0 {
		return nil, fmt.Errorf("out-of-order dawn did not settle around its actual final lantern: %+v", p.current)
	}
	if err := p.screenshot("out-of-order-dawn.png"); err != nil {
		return nil, err
	}
	var cycles []uint32
	for _, sample := range r.Samples[45:] {
		cycles = append(cycles, sample.Cycles)
	}
	return &dawnOrderReport{FixtureROM: result.ROMPath, ReferenceROM: referenceResult.ROMPath,
		Final: p.current, MatchedFrames: r.Parity.MatchedGameFrames, ComparedPixels: r.Parity.ComparedPixels,
		DifferingPixels: r.Parity.DifferingPixels, MissedFrames: r.LastStats.MissedFrames - r.StartupMissedFrames,
		Cycles: summarize(cycles)}, nil
}

func verifyRollover(service build.Service, project, helper, out string, initialTitle [][]byte) (*rolloverReport, error) {
	fixture, err := os.MkdirTemp("", "lumen-rollover-project-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(fixture) }()
	if err := os.CopyFS(fixture, os.DirFS(project)); err != nil {
		return nil, err
	}
	statePath := filepath.Join(fixture, "src", "state.lua")
	state, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}
	if strings.Count(string(state), "\nage = 0\n") != 1 {
		return nil, fmt.Errorf("rollover fixture needs exactly one initial age assignment")
	}
	state = []byte(strings.Replace(string(state), "\nage = 0\n", "\nage = 28798\n", 1))
	if err := os.WriteFile(statePath, state, 0o644); err != nil {
		return nil, err
	}
	service.Renderer = transpiler.RendererMode0
	result, err := service.Build(fixture, filepath.Join(out, "lumen-rollover.gba"))
	if err != nil {
		return nil, fmt.Errorf("rollover fixture build: %w", err)
	}
	session, err := emulator.Start(helper, result.ROMPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = session.Close() }()
	p := &player{session: session, report: &report{}, out: out}
	r := &rolloverReport{FixtureROM: result.ROMPath}
	for tick := range len(initialTitle) + 2 {
		if err := p.advance(0); err != nil {
			return nil, err
		}
		age := tick - 1
		if tick == 0 {
			age = 28799
		}
		if p.current.State != 0 || p.current.Age != float64(age) {
			return nil, fmt.Errorf("rollover fixture at tick %d has state %d, age %v; want title age %d", tick, p.current.State, p.current.Age, age)
		}
		r.ObservedAges = append(r.ObservedAges, age)
		if tick < 2 {
			continue
		}
		r.MatchedFrames++
		r.ComparedPixels += emulator.Width * emulator.Height
		for i := 0; i < len(p.frame.RGBA); i += 4 {
			for channel := range 3 {
				if p.frame.RGBA[i+channel] != initialTitle[tick-2][i+channel] {
					r.DifferingPixels++
					break
				}
			}
		}
	}
	return r, nil
}

// The renderer proof checks that these map passes are direct, ordered and
// unconditional. Record their authored camera formulas beside the native
// camera-phase/pixel evidence so the different parallax rates are reviewable.
func parallaxEvidence(project string) ([]backgroundEvidence, error) {
	source, err := os.ReadFile(filepath.Join(project, "game.lua"))
	if err != nil {
		return nil, err
	}
	var result []backgroundEvidence
	camera := ""
	drawing := false
	for _, line := range strings.Split(string(source), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "--", 2)[0])
		if line == "function _draw()" {
			drawing = true
			continue
		}
		if !drawing {
			continue
		}
		if strings.HasPrefix(line, "camera(") {
			camera = line
		}
		if !strings.HasPrefix(line, "map(") {
			continue
		}
		bgText := strings.FieldsFunc(strings.TrimPrefix(line, "map("), func(r rune) bool { return r == ',' || r == ')' })
		if len(bgText) == 0 {
			return nil, fmt.Errorf("parallax pass has no explicit background: %s", line)
		}
		bg, err := strconv.Atoi(strings.TrimSpace(bgText[0]))
		if err != nil || bg != len(result) || camera == "" {
			return nil, fmt.Errorf("expected ordered BG%d with an explicit camera, got %s after %s", len(result), line, camera)
		}
		for _, previous := range result {
			if strings.ReplaceAll(previous.CameraCall, " ", "") == strings.ReplaceAll(camera, " ", "") {
				return nil, fmt.Errorf("BG%d and BG%d share a camera formula: %s", previous.Background, bg, camera)
			}
		}
		result = append(result, backgroundEvidence{bg, camera, line})
		if len(result) == 4 {
			return result, nil
		}
	}
	return nil, fmt.Errorf("found %d ordered parallax backgrounds, want four", len(result))
}

func (p *player) compare() error {
	if p.current != p.reference.current {
		return fmt.Errorf("paired renderer gameplay diverged: Mode 0 %+v; Mode 4 %+v", p.current, p.reference.current)
	}
	for i, watch := range p.frame.Telemetry.Watches {
		if i >= len(p.reference.frame.Telemetry.Watches) || watch != p.reference.frame.Telemetry.Watches[i] {
			return fmt.Errorf("paired renderer debug watch %d diverged at game frame %d", i, p.current.Frame)
		}
	}
	parity := &p.report.Parity
	parity.MatchedGameFrames++
	parity.ComparedPixels += emulator.Width * emulator.Height
	if parity.MatchedGameFrames == 1 || p.current.CameraX < parity.MinCameraX {
		parity.MinCameraX = p.current.CameraX
	}
	if p.current.CameraX > parity.MaxCameraX {
		parity.MaxCameraX = p.current.CameraX
	}
	parity.CameraPixelPhases |= 1 << uint(int(math.Floor(p.current.CameraX))&7)
	if p.current.Facing < 0 {
		parity.LeftFacingFrames++
	} else {
		parity.RightFacingFrames++
	}
	if p.current.DashTicks > 0 {
		parity.DashFrames++
	}
	if p.current.State == 4 {
		parity.DawnFrames++
	}
	parity.WorldWidth, parity.RoomWidth = worldWidth, roomWidth
	if p.current.X > parity.MaximumMothX {
		parity.MaximumMothX = p.current.X
	}
	if p.current.State == 1 {
		room := int(p.current.X) / roomWidth
		if room < 0 || room >= roomCount {
			return fmt.Errorf("moth left the %d-room world at x=%v", roomCount, p.current.X)
		}
		mask := uint16(1) << uint(room)
		if parity.VisitedRoomMask&mask == 0 {
			parity.VisitedRooms = append(parity.VisitedRooms, room+1)
			parity.VisitedRoomMask |= mask
		}
		if room != p.previousRoom {
			parity.RoomCrossings++
			p.previousRoom = room
		}
	}
	var different uint64
	for i := 0; i < len(p.frame.RGBA); i += 4 {
		for channel := range 3 {
			if p.frame.RGBA[i+channel] != p.reference.frame.RGBA[i+channel] {
				different++
				break
			}
		}
	}
	parity.DifferingPixels += different
	if different != 0 {
		name := fmt.Sprintf("mismatch-frame%06d.png", p.current.Frame)
		if err := p.screenshot(name); err != nil {
			return err
		}
		return fmt.Errorf("Mode 0/4 pixel parity failed at game frame %d: %d differing pixels; see %s and mode4-%s", p.current.Frame, different, name, name)
	}
	return nil
}

func summarize(values []uint32) cycleSummary {
	if len(values) == 0 {
		return cycleSummary{}
	}
	ordered := append([]uint32(nil), values...)
	sort.Slice(ordered, func(a, b int) bool { return ordered[a] < ordered[b] })
	return cycleSummary{len(values), ordered[(len(ordered)*50+99)/100-1], ordered[(len(ordered)*95+99)/100-1], ordered[len(ordered)-1]}
}
