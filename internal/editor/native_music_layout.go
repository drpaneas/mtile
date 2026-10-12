package editor

// NativeMusicLayout is the headless geometry contract for the single-screen
// Music Lab. Keeping it outside the Ebitengine command lets CI verify every
// visible and clickable region without opening a desktop window.
type NativeMusicLayout struct {
	Header, SongName, SongPrevious, SongNext                  Rect
	SongAdd, SongDelete, TempoDown, TempoValue, TempoUp, Play Rect
	Stop                                                      Rect
	Arm, Record                                               Rect
	Body, Channels, Main, Grid, Inspector                     Rect
	WorkflowMusic, WorkflowSFX                                Rect
	Allocation, Articulation, OutputLeft                      Rect
	OutputRight, OutputMix, Arrangement                       Rect
	ClipName, NoteLengthDown, NoteLengthValue                 Rect
	NoteLengthUp, OctaveDown, OctaveUp                        Rect
	BarPrevious, BarValue, BarNext, GridSnap                  Rect
	InstrumentPrevious, InstrumentName                        Rect
	InstrumentNext, InstrumentDuplicate                       Rect
	InstrumentPreset                                          Rect
	Advanced                                                  Rect
	AddSection, ReuseSection, Loop                            Rect
	PartBarsDown, PartBarsValue, PartBarsUp, Meter            Rect
	TakePrevious, TakeNext, TakeUse                           Rect
}

func NewNativeMusicLayout(screenWidth, tabsHeight, statusBarY int) NativeMusicLayout {
	compact := screenWidth < 1400
	headerHeight := 72
	if compact {
		headerHeight = 112
	}
	header := Rect{X: 0, Y: tabsHeight, W: screenWidth, H: headerHeight}
	body := Rect{X: 8, Y: header.Y + header.H + 8, W: screenWidth - 16, H: statusBarY - header.Y - header.H - 16}
	extraWidth := max(0, screenWidth-1600)
	channelWidth := 218 + min(42, extraWidth/10)
	inspectorWidth := 278 + min(82, extraWidth/8)
	if compact {
		channelWidth = 210
		inspectorWidth = 248
	}
	channels := Rect{X: body.X, Y: body.Y, W: channelWidth, H: body.H}
	main := Rect{X: channels.X + channels.W + 8, Y: body.Y, W: body.W - channels.W - 8, H: body.H}
	// Song order is deliberately a compact strip. The piano roll is the primary
	// composing surface, so larger displays give their extra height to notes and
	// the channel inspector instead of an increasingly empty arrangement panel.
	arrangementHeight := min(280, max(220, body.H/4))
	composerOffset := body.H - arrangementHeight - 8
	inspector := Rect{X: main.X + main.W - inspectorWidth - 8, Y: main.Y + 40, W: inspectorWidth, H: composerOffset - 48}
	// Reserve a real toolbar band above the grid. Beat labels live between the
	// controls and the piano rather than being painted over the buttons.
	gridY := main.Y + 112
	gridBottomInset := 120
	if compact {
		gridY = main.Y + 158
		gridBottomInset = 166
	}
	grid := Rect{X: main.X + 56, Y: gridY, W: inspector.X - main.X - 64, H: composerOffset - gridBottomInset}
	arrangement := Rect{X: main.X + 8, Y: main.Y + composerOffset, W: main.W - 16, H: main.H - composerOffset - 8}
	songX := header.X + 472
	songY := header.Y + 20
	songNameWidth := 250
	if compact {
		songX = header.X + 8
		songY = header.Y + 72
		songNameWidth = 230
	}
	clipX := main.X + 88
	clipWidth := 220
	if compact {
		clipX = main.X + 60
		clipWidth = 170
	}
	barX := clipX + clipWidth + 10
	secondaryControlY := main.Y + 54
	gridSnapWidth := 104
	if compact {
		secondaryControlY = main.Y + 102
		gridSnapWidth = 90
	}
	instrumentNameWidth := inspector.W - 132
	instrumentNameX := inspector.X + 42
	addSection := Rect{X: arrangement.X + arrangement.W - 102, Y: arrangement.Y + 8, W: 92, H: 28}
	reuseSection := Rect{X: arrangement.X + arrangement.W - 202, Y: arrangement.Y + 8, W: 92, H: 28}
	loop := Rect{X: arrangement.X + arrangement.W - 330, Y: arrangement.Y + 8, W: 120, H: 28}
	partBarsDown := Rect{X: arrangement.X + 516, Y: arrangement.Y + 8, W: 28, H: 28}
	partBarsValue := Rect{X: arrangement.X + 548, Y: arrangement.Y + 8, W: 70, H: 28}
	partBarsUp := Rect{X: arrangement.X + 622, Y: arrangement.Y + 8, W: 28, H: 28}
	meter := Rect{X: arrangement.X + 658, Y: arrangement.Y + 8, W: 72, H: 28}
	takePrevious := Rect{X: arrangement.X + 272, Y: arrangement.Y + 8, W: 28, H: 28}
	takeNext := Rect{X: arrangement.X + 402, Y: arrangement.Y + 8, W: 28, H: 28}
	takeUse := Rect{X: arrangement.X + 436, Y: arrangement.Y + 8, W: 66, H: 28}
	if compact {
		addSection = Rect{X: arrangement.X + arrangement.W - 90, Y: arrangement.Y + 8, W: 80, H: 28}
		reuseSection = Rect{X: addSection.X - 88, Y: addSection.Y, W: 80, H: 28}
		takePrevious = Rect{X: arrangement.X + 10, Y: arrangement.Y + 42, W: 28, H: 28}
		takeNext = Rect{X: arrangement.X + 150, Y: takePrevious.Y, W: 28, H: 28}
		takeUse = Rect{X: arrangement.X + 186, Y: takePrevious.Y, W: 56, H: 28}
		loop = Rect{X: arrangement.X + 250, Y: takePrevious.Y, W: 102, H: 28}
		partBarsDown = Rect{X: arrangement.X + 360, Y: takePrevious.Y, W: 28, H: 28}
		partBarsValue = Rect{X: arrangement.X + 392, Y: takePrevious.Y, W: 62, H: 28}
		partBarsUp = Rect{X: arrangement.X + 458, Y: takePrevious.Y, W: 28, H: 28}
		meter = Rect{X: arrangement.X + 494, Y: takePrevious.Y, W: 64, H: 28}
	}
	return NativeMusicLayout{
		Header:       header,
		SongPrevious: Rect{X: songX, Y: songY, W: 28, H: 30},
		SongName:     Rect{X: songX + 32, Y: songY, W: songNameWidth, H: 30},
		SongNext:     Rect{X: songX + 36 + songNameWidth, Y: songY, W: 28, H: 30},
		SongAdd:      Rect{X: songX + 68 + songNameWidth, Y: songY, W: 32, H: 30},
		SongDelete:   Rect{X: songX + 104 + songNameWidth, Y: songY, W: 32, H: 30},
		TempoDown:    Rect{X: songX + 164 + songNameWidth, Y: songY, W: 30, H: 30},
		TempoValue:   Rect{X: songX + 198 + songNameWidth, Y: songY, W: 82, H: 30},
		TempoUp:      Rect{X: songX + 284 + songNameWidth, Y: songY, W: 30, H: 30},
		Arm:          Rect{X: header.X + header.W - 384, Y: header.Y + 14, W: 74, H: 38},
		Record:       Rect{X: header.X + header.W - 302, Y: header.Y + 14, W: 88, H: 38},
		Play:         Rect{X: header.X + header.W - 206, Y: header.Y + 14, W: 92, H: 38},
		Stop:         Rect{X: header.X + header.W - 106, Y: header.Y + 14, W: 92, H: 38},
		Body:         body, Channels: channels, Main: main, Grid: grid, Inspector: inspector,
		WorkflowMusic:       Rect{X: channels.X + 10, Y: channels.Y + 6, W: 91, H: 28},
		WorkflowSFX:         Rect{X: channels.X + 109, Y: channels.Y + 6, W: 91, H: 28},
		Allocation:          Rect{X: main.X + 8, Y: main.Y + 6, W: main.W - 302, H: 26},
		Articulation:        Rect{X: main.X + main.W - 410, Y: main.Y + 6, W: 108, H: 26},
		OutputLeft:          Rect{X: main.X + main.W - 674, Y: main.Y + 6, W: 76, H: 26},
		OutputRight:         Rect{X: main.X + main.W - 594, Y: main.Y + 6, W: 76, H: 26},
		OutputMix:           Rect{X: main.X + main.W - 514, Y: main.Y + 6, W: 100, H: 26},
		Arrangement:         arrangement,
		ClipName:            Rect{X: clipX, Y: main.Y + 54, W: clipWidth, H: 28},
		BarPrevious:         Rect{X: barX, Y: main.Y + 54, W: 28, H: 28},
		BarValue:            Rect{X: barX + 32, Y: main.Y + 54, W: 76, H: 28},
		BarNext:             Rect{X: barX + 112, Y: main.Y + 54, W: 28, H: 28},
		GridSnap:            Rect{X: barX + 150, Y: main.Y + 54, W: gridSnapWidth, H: 28},
		NoteLengthDown:      Rect{X: grid.X + grid.W - 286, Y: secondaryControlY, W: 28, H: 28},
		NoteLengthValue:     Rect{X: grid.X + grid.W - 254, Y: secondaryControlY, W: 92, H: 28},
		NoteLengthUp:        Rect{X: grid.X + grid.W - 158, Y: secondaryControlY, W: 28, H: 28},
		OctaveDown:          Rect{X: grid.X + grid.W - 126, Y: secondaryControlY, W: 28, H: 28},
		OctaveUp:            Rect{X: grid.X + grid.W - 30, Y: secondaryControlY, W: 28, H: 28},
		InstrumentPrevious:  Rect{X: inspector.X + 10, Y: inspector.Y + 62, W: 28, H: 28},
		InstrumentName:      Rect{X: instrumentNameX, Y: inspector.Y + 62, W: instrumentNameWidth, H: 28},
		InstrumentNext:      Rect{X: instrumentNameX + instrumentNameWidth + 4, Y: inspector.Y + 62, W: 28, H: 28},
		InstrumentDuplicate: Rect{X: instrumentNameX + instrumentNameWidth + 36, Y: inspector.Y + 62, W: 44, H: 28},
		InstrumentPreset:    Rect{X: inspector.X + 10, Y: inspector.Y + 100, W: inspector.W - 20, H: 28},
		Advanced:            Rect{X: inspector.X + inspector.W - 82, Y: inspector.Y + 6, W: 72, H: 22},
		AddSection:          addSection,
		ReuseSection:        reuseSection,
		Loop:                loop,
		PartBarsDown:        partBarsDown,
		PartBarsValue:       partBarsValue,
		PartBarsUp:          partBarsUp,
		Meter:               meter,
		TakePrevious:        takePrevious,
		TakeNext:            takeNext,
		TakeUse:             takeUse,
	}
}

func (l NativeMusicLayout) ChannelRect(channel int) Rect {
	return Rect{X: l.Channels.X + 10, Y: l.Channels.Y + 42 + channel*70, W: l.Channels.W - 20, H: 62}
}

func (l NativeMusicLayout) ChannelMuteRect(channel int) Rect {
	r := l.ChannelRect(channel)
	return Rect{X: r.X + r.W - 38, Y: r.Y + r.H - 19, W: 16, H: 16}
}

func (l NativeMusicLayout) ChannelSoloRect(channel int) Rect {
	r := l.ChannelRect(channel)
	return Rect{X: r.X + r.W - 20, Y: r.Y + r.H - 19, W: 16, H: 16}
}

func (l NativeMusicLayout) VoiceModeRect(index int) Rect {
	return Rect{X: l.Channels.X + 10 + index%2*99, Y: l.Channels.Y + 346 + index/2*34, W: 91, H: 28}
}

func (l NativeMusicLayout) CaptureOptionRect(index int) Rect {
	switch index {
	case 0:
		return Rect{X: l.Channels.X + 10, Y: l.Channels.Y + 464, W: l.Channels.W - 20, H: 26}
	case 1:
		return Rect{X: l.Channels.X + 10, Y: l.Channels.Y + 494, W: 91, H: 26}
	case 2:
		return Rect{X: l.Channels.X + 109, Y: l.Channels.Y + 494, W: 91, H: 26}
	case 3:
		return Rect{X: l.Channels.X + 10, Y: l.Channels.Y + 524, W: l.Channels.W - 20, H: 26}
	default:
		return Rect{X: l.Channels.X + 10, Y: l.Channels.Y + 554, W: l.Channels.W - 20, H: 26}
	}
}

func (l NativeMusicLayout) ArrangementCardRect(visibleColumn int) Rect {
	top, inset := 44, 54
	if l.Arrangement.W < 1000 {
		top, inset = 80, 90
	}
	return Rect{X: l.Arrangement.X + 10 + visibleColumn*148, Y: l.Arrangement.Y + top, W: 138, H: l.Arrangement.H - inset}
}

func (l NativeMusicLayout) ArrangementLaneRect(visibleColumn, channel int) Rect {
	card := l.ArrangementCardRect(visibleColumn)
	return Rect{X: card.X + 5, Y: card.Y + 31 + channel*23, W: card.W - 10, H: 20}
}

func (l NativeMusicLayout) InspectorControlRect(channel, index int) Rect {
	const columns = 2
	gap := 8
	width := (l.Inspector.W - 28 - gap) / columns
	return Rect{
		X: l.Inspector.X + 10 + index%columns*(width+gap),
		Y: l.Inspector.Y + 160 + index/columns*55,
		W: width, H: 31,
	}
}
