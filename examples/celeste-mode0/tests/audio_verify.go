//go:build ignore

// Run with go run examples/celeste/tests/audio_verify.go.
package main

import (
	"fmt"
	"os"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

func must(ok bool, format string, args ...any) {
	if !ok {
		panic(fmt.Sprintf(format, args...))
	}
}

func main() {
	project := "examples/celeste"
	if len(os.Args) > 1 {
		project = os.Args[1]
	}
	assets, err := assetsync.LoadNativeAudio(project)
	must(err == nil, "asset load: %v", err)
	library := assets.Library
	clipByID := map[uint8]audio.Clip{}
	partByID := map[uint8]audio.Part{}
	for _, clip := range library.Clips {
		clipByID[clip.ID] = clip
	}
	for _, part := range library.Parts {
		partByID[part.ID] = part
	}
	newController := func() *audio.Controller {
		hw.ResetForTest()
		c := new(audio.Controller)
		must(c.Init(audio.ProfilePSG) && c.Bind(library), "controller init/bind failed")
		return c
	}
	checks := 0
	assertChannel := func(c *audio.Controller, clip audio.Clip, tick uint32, owner audio.NativeOwner) {
		state := c.Stats().Channels[clip.Kind]
		var expected *audio.NoteEvent
		for i := range clip.NoteEvents {
			e := &clip.NoteEvents[i]
			if e.Tick <= tick && tick < e.Tick+uint32(e.DurationTicks) {
				expected = e
				break
			}
		}
		if expected == nil {
			must(!state.Active, "clip %d tick %d unexpected active %+v", clip.ID, tick, state)
			return
		}
		must(state.Active && state.Owner == owner && state.Note == expected.Note && state.SoundID == expected.SoundID,
			"clip %d tick %d expected %+v, state %+v", clip.ID, tick, *expected, state)
		checks++
	}
	count := 0
	for _, authored := range assets.Music.Clips {
		if !authored.SFX {
			continue
		}
		count++
		clip := clipByID[uint8(authored.ID)]
		c := newController()
		must(c.PlaySFX(clip.ID, 160), "SFX%d rejected", clip.ID)
		c.Service()
		for i := 0; i < int(clip.LengthTicks)*2+10; i++ {
			s := c.Stats()
			playing := s.SFX[clip.Kind]
			if !playing.Active {
				break
			}
			assertChannel(c, clip, playing.Tick, audio.OwnerSFX)
			c.Service()
		}
		stats := c.Stats()
		must(!stats.SFX[clip.Kind].Active, "SFX%d did not finish", clip.ID)
		must(stats.CommandsRejected == 0 && stats.QueueOverflows == 0 && stats.InvalidAssets == 0, "SFX%d diagnostics %+v", clip.ID, stats)
	}
	for _, song := range library.Songs {
		c := newController()
		must(c.PlaySong(song.ID), "song%d rejected", song.ID)
		c.Service()
		total := 0
		for _, pid := range song.Sequence {
			total += int(partByID[pid].LengthTicks)
		}
		// Two complete cycles verify part transitions and the loop end/start.
		seen := map[uint8]bool{}
		for i := 0; i < total*8/3+16; i++ {
			s := c.Stats()
			must(s.RunningSong, "song%d unexpectedly stopped", song.ID)
			part := partByID[s.PartID]
			seen[part.ID] = true
			for _, cid := range part.PSGClips {
				if cid >= 0 {
					assertChannel(c, clipByID[uint8(cid)], s.PartTick, audio.OwnerSong)
				}
			}
			c.Service()
		}
		s := c.Stats()
		must(len(seen) == len(song.Sequence), "song%d visited%d/%d parts", song.ID, len(seen), len(song.Sequence))
		must(s.CommandsRejected == 0 && s.QueueOverflows == 0 && s.InvalidAssets == 0, "song%d diagnostics %+v", song.ID, s)
	}
	// Gameplay effect ownership must interrupt and finish while music continues.
	c := newController()
	must(c.PlaySong(0), "music0 rejected")
	c.Service()
	must(c.PlaySFX(1, 160), "jump rejected")
	c.Service()
	must(c.Stats().RunningSong && c.Stats().SFX[audio.SoundCH3].Active, "SFX/music coexistence failed")
	must(c.StopSFX(), "stopSFX rejected")
	c.Service()
	must(c.Stats().RunningSong && !c.Stats().SFX[audio.SoundCH3].Active, "stopSFX stopped song")
	fmt.Printf("PASS: %d native SFX clips, %d song graphs (two loops each), %d audible channel-state checks; no invalid assets/commands/overflows.\n", count, len(library.Songs), checks)
}
