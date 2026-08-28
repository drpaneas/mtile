package editor

import "testing"

func TestNativeMusicLayoutIsOneBoundedNonOverlappingWorkspace(t *testing.T) {
	const width, tabs, statusY = 1600, 36, 976
	layout := NewNativeMusicLayout(width, tabs, statusY)
	inside := func(name string, rect Rect) {
		t.Helper()
		if rect.W <= 0 || rect.H <= 0 || rect.X < 0 || rect.Y < tabs || rect.X+rect.W > width || rect.Y+rect.H > statusY {
			t.Fatalf("%s outside Music Lab: %#v", name, rect)
		}
	}
	for name, rect := range map[string]Rect{
		"header": layout.Header, "channels": layout.Channels, "grid": layout.Grid,
		"inspector": layout.Inspector, "arrangement": layout.Arrangement,
		"arm": layout.Arm, "record": layout.Record, "play": layout.Play, "stop": layout.Stop,
		"song previous": layout.SongPrevious, "song name": layout.SongName, "song next": layout.SongNext, "song add": layout.SongAdd, "song delete": layout.SongDelete,
		"tempo down": layout.TempoDown, "tempo value": layout.TempoValue, "tempo up": layout.TempoUp,
		"bar previous": layout.BarPrevious, "bar value": layout.BarValue, "bar next": layout.BarNext, "grid snap": layout.GridSnap,
		"music workflow": layout.WorkflowMusic, "sfx workflow": layout.WorkflowSFX, "articulation": layout.Articulation,
		"output left": layout.OutputLeft, "output right": layout.OutputRight, "output mix": layout.OutputMix,
		"note length down": layout.NoteLengthDown, "note length": layout.NoteLengthValue, "note length up": layout.NoteLengthUp,
		"instrument previous": layout.InstrumentPrevious, "instrument name": layout.InstrumentName,
		"instrument next": layout.InstrumentNext, "instrument duplicate": layout.InstrumentDuplicate,
		"part bars down": layout.PartBarsDown, "part bars": layout.PartBarsValue, "part bars up": layout.PartBarsUp, "meter": layout.Meter,
		"advanced":      layout.Advanced,
		"take previous": layout.TakePrevious, "take next": layout.TakeNext, "take use": layout.TakeUse,
	} {
		inside(name, rect)
	}
	if layout.Grid.Y+layout.Grid.H > layout.Arrangement.Y || layout.Inspector.Y+layout.Inspector.H > layout.Arrangement.Y {
		t.Fatalf("composer overlaps order: grid=%#v inspector=%#v arrangement=%#v", layout.Grid, layout.Inspector, layout.Arrangement)
	}
	workspaceIdentityRight := layout.Header.X + 456
	if workspaceIdentityRight >= layout.SongPrevious.X {
		t.Fatalf("Mtile/workspace identity ends at %d and overlaps song controls starting at %d", workspaceIdentityRight, layout.SongPrevious.X)
	}
	controlBottom := layout.ClipName.Y + layout.ClipName.H
	beatLabelTop := layout.Grid.Y - 18
	if controlBottom > beatLabelTop {
		t.Fatalf("composer controls end at %d and overlap beat labels starting at %d", controlBottom, beatLabelTop)
	}
	inspectorTitleBottom := layout.Inspector.Y + 24
	instrumentLabelTop := layout.InstrumentName.Y - 14
	if inspectorTitleBottom > instrumentLabelTop {
		t.Fatalf("inspector title ends at %d and overlaps instrument label at %d", inspectorTitleBottom, instrumentLabelTop)
	}
	firstInspectorLabelTop := layout.InspectorControlRect(0, 0).Y - 17
	if layout.InstrumentName.Y+layout.InstrumentName.H > firstInspectorLabelTop {
		t.Fatalf("instrument row ends at %d and overlaps first sound-control label at %d", layout.InstrumentName.Y+layout.InstrumentName.H, firstInspectorLabelTop)
	}
	if layout.OutputLeft.X+layout.OutputLeft.W > layout.OutputRight.X ||
		layout.OutputRight.X+layout.OutputRight.W > layout.OutputMix.X ||
		layout.OutputMix.X+layout.OutputMix.W > layout.Articulation.X {
		t.Fatalf("PSG output controls overlap articulation: left=%#v right=%#v mix=%#v articulation=%#v",
			layout.OutputLeft, layout.OutputRight, layout.OutputMix, layout.Articulation)
	}
	if layout.ClipName.X+layout.ClipName.W > layout.BarPrevious.X ||
		layout.BarPrevious.X+layout.BarPrevious.W > layout.BarValue.X ||
		layout.BarValue.X+layout.BarValue.W > layout.BarNext.X ||
		layout.BarNext.X+layout.BarNext.W > layout.GridSnap.X ||
		layout.GridSnap.X+layout.GridSnap.W > layout.NoteLengthDown.X ||
		layout.NoteLengthDown.X+layout.NoteLengthDown.W > layout.NoteLengthValue.X ||
		layout.NoteLengthValue.X+layout.NoteLengthValue.W > layout.NoteLengthUp.X ||
		layout.NoteLengthUp.X+layout.NoteLengthUp.W > layout.OctaveDown.X {
		t.Fatalf("clip/bar/length/octave controls overlap: clip=%#v bar=%#v down=%#v value=%#v up=%#v octave=%#v",
			layout.ClipName, layout.BarValue, layout.NoteLengthDown, layout.NoteLengthValue, layout.NoteLengthUp, layout.OctaveDown)
	}
	if layout.InstrumentPrevious.X+layout.InstrumentPrevious.W > layout.InstrumentName.X ||
		layout.InstrumentName.X+layout.InstrumentName.W > layout.InstrumentNext.X ||
		layout.InstrumentNext.X+layout.InstrumentNext.W > layout.InstrumentDuplicate.X {
		t.Fatalf("instrument controls overlap: previous=%#v name=%#v next=%#v duplicate=%#v",
			layout.InstrumentPrevious, layout.InstrumentName, layout.InstrumentNext, layout.InstrumentDuplicate)
	}
	for channel := 0; channel < 4; channel++ {
		channelRect := layout.ChannelRect(channel)
		inside("channel", channelRect)
		for name, control := range map[string]Rect{"mute": layout.ChannelMuteRect(channel), "solo": layout.ChannelSoloRect(channel)} {
			inside(name, control)
			if control.X < channelRect.X || control.Y < channelRect.Y || control.X+control.W > channelRect.X+channelRect.W || control.Y+control.H > channelRect.Y+channelRect.H {
				t.Fatalf("CH%d %s outside channel card: control=%#v card=%#v", channel+1, name, control, channelRect)
			}
		}
		if mute, solo := layout.ChannelMuteRect(channel), layout.ChannelSoloRect(channel); mute.X+mute.W > solo.X {
			t.Fatalf("CH%d mute/solo overlap: mute=%#v solo=%#v", channel+1, mute, solo)
		}
		inside("arrangement lane", layout.ArrangementLaneRect(0, channel))
	}
	for mode := 0; mode < 5; mode++ {
		inside("voice mode", layout.VoiceModeRect(mode))
		inside("capture option", layout.CaptureOptionRect(mode))
	}
	for channel, count := range [...]int{9, 6, 5, 8} {
		for index := 0; index < count; index++ {
			control := layout.InspectorControlRect(channel, index)
			inside("inspector control", control)
			if control.Y+control.H > layout.Inspector.Y+layout.Inspector.H {
				t.Fatalf("CH%d control %d outside inspector: %#v", channel+1, index, control)
			}
		}
	}
}

func TestNativeMusicLayoutGivesLargeDisplaysMoreComposerSpace(t *testing.T) {
	standard := NewNativeMusicLayout(1920, 36, 1176)
	large := NewNativeMusicLayout(2560, 36, 1416)
	if large.Grid.W <= standard.Grid.W || large.Grid.H <= standard.Grid.H {
		t.Fatalf("large grid = %dx%d, standard = %dx%d", large.Grid.W, large.Grid.H, standard.Grid.W, standard.Grid.H)
	}
	if large.Arrangement.W <= standard.Arrangement.W {
		t.Fatalf("large arrangement width = %d, standard = %d", large.Arrangement.W, standard.Arrangement.W)
	}
	if standard.Arrangement.H > 280 || large.Arrangement.H > 280 {
		t.Fatalf("song order grew too tall: standard=%d large=%d", standard.Arrangement.H, large.Arrangement.H)
	}
	if standard.Grid.H <= standard.Arrangement.H*2 || large.Grid.H <= large.Arrangement.H*2 {
		t.Fatalf("piano roll should dominate vertical space: standard grid/order=%d/%d large=%d/%d", standard.Grid.H, standard.Arrangement.H, large.Grid.H, large.Arrangement.H)
	}
	if large.Grid.Y+large.Grid.H > large.Arrangement.Y || large.Inspector.Y+large.Inspector.H > large.Arrangement.Y {
		t.Fatalf("large composer overlaps arrangement: grid=%#v inspector=%#v arrangement=%#v", large.Grid, large.Inspector, large.Arrangement)
	}
}

func TestNativeMusicLayoutReflowsAtNativeMacWindowSize(t *testing.T) {
	const width, tabs, statusY = 1031, 36, 1047
	layout := NewNativeMusicLayout(width, tabs, statusY)
	inside := func(name string, rect Rect) {
		t.Helper()
		if rect.W <= 0 || rect.H <= 0 || rect.X < 0 || rect.Y < tabs || rect.X+rect.W > width || rect.Y+rect.H > statusY {
			t.Fatalf("%s outside compact Music Lab: %#v", name, rect)
		}
	}
	for name, rect := range map[string]Rect{
		"header": layout.Header, "channels": layout.Channels, "grid": layout.Grid,
		"inspector": layout.Inspector, "arrangement": layout.Arrangement,
		"song": layout.SongName, "tempo": layout.TempoValue,
		"clip": layout.ClipName, "bar": layout.BarValue, "snap": layout.GridSnap,
		"note length": layout.NoteLengthValue, "octave": layout.OctaveDown,
		"add": layout.AddSection, "reuse": layout.ReuseSection, "loop": layout.Loop,
		"bars": layout.PartBarsValue, "meter": layout.Meter, "take": layout.TakeUse,
	} {
		inside(name, rect)
	}
	if layout.Grid.Y <= layout.ClipName.Y+layout.ClipName.H ||
		layout.Grid.Y <= layout.NoteLengthValue.Y+layout.NoteLengthValue.H {
		t.Fatalf("compact toolbar intrudes into piano: grid=%#v clip=%#v length=%#v", layout.Grid, layout.ClipName, layout.NoteLengthValue)
	}
	if layout.Grid.Y+layout.Grid.H > layout.Arrangement.Y || layout.Inspector.Y+layout.Inspector.H > layout.Arrangement.Y {
		t.Fatalf("compact composer overlaps arrangement: grid=%#v inspector=%#v arrangement=%#v", layout.Grid, layout.Inspector, layout.Arrangement)
	}
	for _, pair := range [][2]Rect{
		{layout.ClipName, layout.BarPrevious},
		{layout.BarNext, layout.GridSnap},
		{layout.TakeUse, layout.Loop},
		{layout.Loop, layout.PartBarsDown},
		{layout.PartBarsUp, layout.Meter},
		{layout.ReuseSection, layout.AddSection},
	} {
		if pair[0].X+pair[0].W > pair[1].X {
			t.Fatalf("compact controls overlap: %#v and %#v", pair[0], pair[1])
		}
	}
}
