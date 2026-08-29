package main

import (
	"fmt"
	"runtime"
	"time"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/midicontrol"
	"github.com/drpaneas/mtile/internal/performance"
	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Oto divides the requested device queue into four chunks on macOS and two
// ALSA periods on Linux; WASAPI can request the complete shared buffer. Keep
// two callbacks of source data on macOS: a single 10 ms chunk was vulnerable
// to an editor frame or short GC pause and presented as crackling/torn MIDI
// notes. This remains far below Oto's old 500 ms source default.
func nativeLiveBufferSize() time.Duration {
	switch runtime.GOOS {
	case "darwin":
		return editorDeviceBufferSize / 2
	case "linux":
		return editorDeviceBufferSize / 2
	default:
		return editorDeviceBufferSize
	}
}

var nativePerformanceKeys = [...]ebiten.Key{
	ebiten.KeyA, ebiten.KeyW, ebiten.KeyS, ebiten.KeyE, ebiten.KeyD, ebiten.KeyF,
	ebiten.KeyT, ebiten.KeyG, ebiten.KeyY, ebiten.KeyH, ebiten.KeyU, ebiten.KeyJ,
}

func (g *Game) initNativePerformance() {
	if g == nil {
		return
	}
	g.nativeStream = performance.NewStream(editorPlaybackSampleRate)
	g.syncNativePerformanceOutput()
	g.nativeSession = performance.NewSession(g.nativeStream, g.nativePerformanceSoundBank())
	g.nativePerformanceInstrument = g.nativeInstrument
	g.nativeClickStream = performance.NewClickStream(editorPlaybackSampleRate)
	g.nativeConductor = performance.NewConductor(g.nativeSession, g.nativeClickStream.Trigger)
	g.nativeCaptureSequence = -1
	g.nativeVoiceMode = performance.ModeMono
}

func (g *Game) syncNativePerformanceOutput() {
	if g == nil || g.nativeStream == nil {
		return
	}
	if g.model == nil {
		g.nativeStream.SetOutput(7, 7, 100)
		return
	}
	settings := g.model.NativeAudio.Settings
	g.nativeStream.SetOutput(settings.LeftLevel, settings.RightLevel, settings.PSGRatio)
}

func (g *Game) initNativeMIDI() {
	if g == nil || g.nativeMIDI != nil {
		return
	}
	backend, err := midicontrol.NewSystemBackend()
	if err != nil {
		g.statusMsg = "MIDI unavailable: " + err.Error()
		return
	}
	g.nativeMIDICommands = make(chan midicontrol.Command, 256)
	g.nativeMIDI = midicontrol.NewManager(backend, midicontrol.LaunchkeyMini25MK4{}, func(command midicontrol.Command) {
		if command.Kind == midicontrol.CommandPerformance {
			if g.nativeSession != nil {
				command.Performance.At = g.nativeSession.Now()
				g.nativeSession.Handle(command.Performance)
			}
			return
		}
		select {
		case g.nativeMIDICommands <- command:
		default:
		}
	})
	if err := g.nativeMIDI.Start(); err != nil {
		_ = g.nativeMIDI.Close()
		g.nativeMIDI = nil
		g.statusMsg = "MIDI unavailable: " + err.Error()
	}
}

func (g *Game) refreshNativePerformanceSounds() {
	if g == nil {
		return
	}
	g.syncNativePerformanceOutput()
	if g.nativeSession == nil {
		return
	}
	g.nativeSession.SetSoundBank(g.nativePerformanceSoundBank())
	g.nativePerformanceInstrument = g.nativeInstrument
}

func (g *Game) syncNativePerformanceSounds() {
	if g == nil || g.nativeSession == nil || g.nativePerformanceInstrument == g.nativeInstrument {
		return
	}
	g.refreshNativePerformanceSounds()
}

func (g *Game) nativePerformanceSoundBank() performance.SoundBank {
	bank := defaultPerformanceSoundBank()
	if g == nil || g.model == nil {
		return bank
	}
	for channel := range bank.Channels {
		selectedID := g.nativeInstrument[channel]
		for _, sound := range g.model.NativeAudio.Library.Sounds {
			if int(sound.Kind) == channel && int(sound.ID) == selectedID {
				bank.Channels[channel] = sound
				break
			}
		}
	}
	// Launchkey pads follow a stable General-MIDI-like percussion order while
	// reusing the exact authored GB Studio presets whenever they exist.
	noiseNames := [...]string{
		"Bass Drum", "Snare Drum", "Closed Hi-Hat", "Open Hi-Hat",
		"Crash", "Snare Drum 2", "Bass Drum 2", "Explosion",
	}
	for preset, name := range noiseNames {
		for _, authored := range g.model.NativeAudio.Sounds.Sounds {
			if authored.Kind != "ch4" || authored.Name != name {
				continue
			}
			for _, compiled := range g.model.NativeAudio.Library.Sounds {
				if int(compiled.ID) == authored.ID {
					bank.Noise[preset] = compiled
				}
			}
		}
	}
	return bank
}

func defaultPerformanceSoundBank() performance.SoundBank {
	var bank performance.SoundBank
	bank.Channels[0] = runtimeaudio.Sound{Kind: runtimeaudio.SoundCH1, Pulse1: hw.Pulse1Config{
		Pulse: hw.PulseConfig{Duty: hw.PulseDuty50, Envelope: hw.HardwareEnvelope{Initial: 12}, Route: hw.SoundRouteBoth},
	}}
	bank.Channels[1] = runtimeaudio.Sound{Kind: runtimeaudio.SoundCH2, Pulse2: hw.PulseConfig{
		Duty: hw.PulseDuty25, Envelope: hw.HardwareEnvelope{Initial: 11}, Route: hw.SoundRouteBoth,
	}}
	wave := performance.PulseWaveform(50)
	bank.Channels[2] = runtimeaudio.Sound{Kind: runtimeaudio.SoundCH3, Wave: hw.WaveConfig{
		Wave: hw.Waveform{Length: 32}, Level: hw.Wave100, Route: hw.SoundRouteBoth,
	}}
	copy(bank.Channels[2].Wave.Wave.Samples[:32], wave[:])
	noise := [...]hw.NoiseConfig{
		{Width: hw.Noise15Bit, Divisor: 7, Shift: 7, Envelope: hw.HardwareEnvelope{Initial: 15, Pace: 2}, Route: hw.SoundRouteBoth},
		{Width: hw.Noise15Bit, Divisor: 3, Shift: 4, Envelope: hw.HardwareEnvelope{Initial: 14, Pace: 3}, Route: hw.SoundRouteBoth},
		{Width: hw.Noise7Bit, Divisor: 1, Shift: 2, Envelope: hw.HardwareEnvelope{Initial: 10, Pace: 1}, Route: hw.SoundRouteBoth},
		{Width: hw.Noise7Bit, Divisor: 2, Shift: 3, Envelope: hw.HardwareEnvelope{Initial: 12, Pace: 4}, Route: hw.SoundRouteBoth},
		{Width: hw.Noise15Bit, Divisor: 5, Shift: 6, Envelope: hw.HardwareEnvelope{Initial: 14, Pace: 2}, Route: hw.SoundRouteBoth},
		{Width: hw.Noise15Bit, Divisor: 3, Shift: 6, Envelope: hw.HardwareEnvelope{Initial: 13, Pace: 2}, Route: hw.SoundRouteBoth},
		{Width: hw.Noise7Bit, Divisor: 1, Shift: 8, Envelope: hw.HardwareEnvelope{Initial: 11, Pace: 1}, Route: hw.SoundRouteBoth},
		{Width: hw.Noise15Bit, Divisor: 1, Shift: 3, Envelope: hw.HardwareEnvelope{Initial: 15, Pace: 6}, Route: hw.SoundRouteBoth},
	}
	for index := range bank.Noise {
		bank.Noise[index] = runtimeaudio.Sound{Kind: runtimeaudio.SoundCH4, Noise: noise[index]}
	}
	bank.Channels[3] = bank.Noise[0]
	return bank
}

func (g *Game) ensureNativeLiveAudio() bool {
	if g == nil || g.nativeStream == nil {
		return false
	}
	context, err := editorAudioContext()
	if err != nil {
		g.statusMsg = "live audio failed: " + err.Error()
		return false
	}
	if g.nativeLivePlayer == nil {
		g.nativeLivePlayer, err = context.NewPlayer(g.nativeStream)
		if err != nil {
			g.statusMsg = "live audio failed: " + err.Error()
			return false
		}
		g.nativeLivePlayer.SetBufferSize(nativeLiveBufferSize())
		g.setPreviewPlayerVolume(g.nativeLivePlayer)
	}
	if !g.nativeLivePlayer.IsPlaying() {
		g.nativeLivePlayer.Play()
	}
	if g.nativeClickPlayer == nil && g.nativeClickStream != nil {
		g.nativeClickPlayer, err = context.NewPlayer(g.nativeClickStream)
		if err != nil {
			g.statusMsg = "metronome audio failed: " + err.Error()
			return false
		}
		g.nativeClickPlayer.SetBufferSize(nativeLiveBufferSize())
		g.setPreviewPlayerVolume(g.nativeClickPlayer)
	}
	if g.nativeClickPlayer != nil && !g.nativeClickPlayer.IsPlaying() {
		g.nativeClickPlayer.Play()
	}
	return true
}

func (g *Game) closeNativePerformance() {
	if g == nil {
		return
	}
	if g.nativeMIDI != nil {
		_ = g.nativeMIDI.Close()
		g.nativeMIDI = nil
	}
	if g.nativeConductor != nil && g.nativeConductor.Active() {
		if take, ok := g.nativeConductor.Stop(); ok {
			g.commitNativeTake(take, fmt.Sprintf("Take %d recorded on exit", g.nativeTakeCount))
		}
		g.clearNativeCapture()
	}
	if g.nativeSession != nil {
		_, _ = g.nativeSession.Disconnect(g.nativeSession.Now())
	}
	if g.nativeLivePlayer != nil {
		g.nativeLivePlayer.Pause()
		_ = g.nativeLivePlayer.Close()
		g.nativeLivePlayer = nil
	}
	if g.nativeClickPlayer != nil {
		g.nativeClickPlayer.Pause()
		_ = g.nativeClickPlayer.Close()
		g.nativeClickPlayer = nil
	}
	if g.nativeStream != nil {
		_ = g.nativeStream.Close()
	}
	if g.nativeClickStream != nil {
		_ = g.nativeClickStream.Close()
	}
}

func (g *Game) setNativeArmed(armed bool) {
	if g == nil || g.nativeSession == nil {
		return
	}
	if armed && !g.ensureNativeLiveAudio() {
		return
	}
	if !armed && g.nativeConductor != nil && g.nativeConductor.Active() {
		if take, ok := g.nativeConductor.Stop(); ok {
			g.commitNativeTake(take, fmt.Sprintf("Take %d recorded", g.nativeTakeCount))
		}
		g.clearNativeCapture()
	}
	g.nativeSession.Arm(armed, g.nativeSession.Now())
	if armed {
		g.statusMsg = g.nativeVoiceMode.String() + " armed — play A–J or your controller"
	} else {
		g.statusMsg = "Performance disarmed"
	}
}

func (g *Game) setNativeVoiceMode(mode performance.VoiceMode) {
	if g == nil || g.nativeSession == nil {
		return
	}
	mono := uint8(min(g.nativeChannel+1, 3))
	g.nativeSession.SetMode(mode, mono, g.nativeSession.Now())
	g.nativeVoiceMode = mode
	if mode == performance.ModeNoise {
		g.nativeChannel = 3
	} else if mode != performance.ModeMono {
		g.nativeChannel = 0
	}
	g.statusMsg = mode.String() + " performance mode"
}

func (g *Game) toggleNativeRecording() {
	if g == nil || g.nativeSession == nil || g.nativeConductor == nil {
		return
	}
	snapshot := g.nativeSession.Snapshot()
	if !g.nativeConductor.Active() {
		if !snapshot.Armed {
			g.statusMsg = "ARM a channel or poly mode before recording"
			return
		}
		tempo, _ := g.model.NativePartTempo(g.nativeSequence)
		clock := performance.NewTrackerClock(tempo, g.nativePartTicks())
		if !g.nativeConductor.Start(performance.CaptureOptions{
			Clock: clock, CountInBars: g.nativeCountIn, Loop: false, Metronome: g.nativeMetronome,
		}) {
			g.statusMsg = "Recording could not start"
			return
		}
		g.nativeCaptureClock = clock
		g.nativeCaptureSequence = g.nativeSequence
		g.nativeCaptureTick = 0
		g.nativeCaptureVisible = false
		g.nativeCaptureEvents = g.nativeCaptureEvents[:0]
		g.nativeCaptureNotes = g.nativeCaptureNotes[:0]
		g.nativeCaptureEventCount = -1
		g.nativeCaptureDropped = 0
		g.nativeCaptureQuantize = g.nativeQuantize
		g.nativeCaptureSwing = g.nativeSwing
		g.nativeTakeCount++
		if g.nativeCountIn > 0 {
			g.statusMsg = fmt.Sprintf("COUNT-IN %d BAR — Take %d", g.nativeCountIn, g.nativeTakeCount)
		} else {
			g.statusMsg = fmt.Sprintf("REC — Take %d", g.nativeTakeCount)
		}
		return
	}
	take, ok := g.nativeConductor.Stop()
	if !ok {
		g.clearNativeCapture()
		return
	}
	g.commitNativeTake(take, fmt.Sprintf("Take %d recorded", g.nativeTakeCount))
	g.clearNativeCapture()
}

func (g *Game) stopNativeTransport() {
	g.stopAudioPreview()
	if g.nativeConductor == nil || !g.nativeConductor.Active() {
		return
	}
	if take, ok := g.nativeConductor.Stop(); ok {
		g.commitNativeTake(take, fmt.Sprintf("Take %d recorded", g.nativeTakeCount))
	}
	g.clearNativeCapture()
}

func (g *Game) updateNativeCapture() {
	if g == nil || g.nativeConductor == nil {
		return
	}
	for {
		select {
		case event := <-g.nativeConductor.Events():
			switch event.Kind {
			case performance.CaptureStarted:
				// A very fast disarm/stop can leave the asynchronous start notice
				// queued for one frame. Never let that stale event overwrite the
				// final committed status with a false REC indicator.
				if g.nativeConductor.Snapshot().Recording {
					g.statusMsg = fmt.Sprintf("REC — Take %d", g.nativeTakeCount)
				}
			case performance.CaptureTake:
				g.commitNativeTake(event.Take, fmt.Sprintf("Loop take %d recorded", g.nativeTakeCount))
				g.nativeTakeCount++
			}
		default:
			goto drained
		}
	}

drained:
	snapshot := g.nativeConductor.Snapshot()
	if !snapshot.Recording || g.nativeSession == nil || g.nativeCaptureSequence < 0 {
		return
	}
	g.nativeCaptureVisible = true
	g.nativeCaptureTick = max(0, int(snapshot.Tick))
	g.nativeSequence = g.nativeCaptureSequence
	take, ok := g.nativeSession.RecordingSnapshotInto(g.nativeCaptureEvents, g.nativeSession.Now())
	if ok {
		g.nativeCaptureEvents = take.Events
		if len(take.Events) != g.nativeCaptureEventCount || take.Dropped != g.nativeCaptureDropped ||
			g.nativeQuantize != g.nativeCaptureQuantize || g.nativeSwing != g.nativeCaptureSwing {
			materialized := performance.MaterializeSwing(take, g.nativeCaptureClock, g.nativeQuantize, g.nativeSwing)
			g.nativeCaptureNotes = append(g.nativeCaptureNotes[:0], materialized.Notes...)
			g.nativeCaptureEventCount = len(take.Events)
			g.nativeCaptureDropped = take.Dropped
			g.nativeCaptureQuantize = g.nativeQuantize
			g.nativeCaptureSwing = g.nativeSwing
		}
		// Materialization closes currently held notes at the snapshot duration.
		// Extend only those cached visual notes in place as the playhead moves;
		// completed notes are rematerialized once their stop event arrives.
		allocation := g.nativeSession.Snapshot().Allocation
		visualEnd := int(g.nativeCaptureClock.QuantizeTickSwing(snapshot.Tick, g.nativeQuantize, g.nativeSwing))
		for channel := 0; channel < 3; channel++ {
			if !allocation.Active[channel] {
				continue
			}
			for index := len(g.nativeCaptureNotes) - 1; index >= 0; index-- {
				note := &g.nativeCaptureNotes[index]
				if int(note.Channel) != channel+1 || note.Note != allocation.Notes[channel] {
					continue
				}
				note.DurationTicks = max(note.DurationTicks, visualEnd-note.StartTick)
				break
			}
		}
	}
	if part, ok := g.model.NativePartAt(g.nativeCaptureSequence); ok {
		_, _, _, ticksPerBar := editor.NativePartMeter(part)
		g.nativeBar = g.nativeCaptureTick / max(1, ticksPerBar)
	}
	g.nativeSelectedStep = g.nativeCaptureTick / editor.NativeTicksPerStep
}

func (g *Game) clearNativeCapture() {
	if g == nil {
		return
	}
	g.nativeCaptureSequence = -1
	g.nativeCaptureTick = 0
	g.nativeCaptureVisible = false
	g.nativeCaptureEvents = g.nativeCaptureEvents[:0]
	g.nativeCaptureNotes = g.nativeCaptureNotes[:0]
	g.nativeCaptureEventCount = -1
	g.nativeCaptureDropped = 0
}

func (g *Game) commitNativeTake(take performance.RawPerformanceTake, message string) {
	sequence := g.nativeSequence
	clock := g.nativeCaptureClock
	if g.nativeCaptureSequence >= 0 {
		sequence = g.nativeCaptureSequence
	} else {
		tempo, _ := g.model.NativePartTempo(sequence)
		clock = performance.NewTrackerClock(tempo, g.nativePartTicks())
	}
	id, err := g.model.ApplyNativePerformanceTakeSwing(sequence, take, clock, g.nativeQuantize, g.nativeSwing, g.nativeOverdub)
	if err != nil {
		g.statusMsg = "recording not committed: " + err.Error()
		return
	}
	g.nativeSequence = sequence
	for index, retained := range g.model.NativeTakesAt(sequence) {
		if retained.ID == id {
			g.nativeTakeSelection = index
			break
		}
	}
	timing := g.nativeQuantize.String()
	if g.nativeQuantize.Swingable() && g.nativeSwing != 50 {
		timing += fmt.Sprintf(" / %d%% swing", g.nativeSwing)
	}
	g.markNativeMusicEdit(fmt.Sprintf("%s (%s)", message, timing))
}

func (g *Game) updateNativeMIDI() {
	if g == nil || g.nativeMIDI == nil {
		return
	}
	for {
		select {
		case command := <-g.nativeMIDICommands:
			g.applyNativeMIDICommand(command)
			// MIDI commands arrive outside the mouse/keyboard input path. Wake the
			// retained UI immediately so controller state never waits for the idle
			// safety refresh before becoming visible.
			g.requestRender()
		default:
			g.nativeMIDI.SetState(g.nativeControllerState())
			return
		}
	}
}

func (g *Game) applyNativeMIDICommand(command midicontrol.Command) {
	switch command.Kind {
	case midicontrol.CommandMode:
		g.setNativeVoiceMode(command.Mode)
	case midicontrol.CommandArmToggle:
		g.setNativeArmed(!g.nativeSession.Snapshot().Armed)
	case midicontrol.CommandRecordToggle:
		g.toggleNativeRecording()
	case midicontrol.CommandPlayToggle:
		g.toggleNativeMusicPreview()
	case midicontrol.CommandStop:
		g.stopNativeTransport()
	case midicontrol.CommandMetronomeToggle:
		g.nativeMetronome = !g.nativeMetronome
		if g.nativeConductor != nil {
			g.nativeConductor.SetMetronome(g.nativeMetronome)
		}
	case midicontrol.CommandCountInCycle:
		g.nativeCountIn = (g.nativeCountIn + 1) % 3
	case midicontrol.CommandLoopToggle:
		if song, ok := g.model.NativeSong(); ok {
			if err := g.model.SetNativeSongLoop(song.LoopStart == nil); err != nil {
				g.statusMsg = err.Error()
			} else {
				g.markNativeMusicEdit("Song ending changed")
			}
		}
	case midicontrol.CommandOverdubToggle:
		g.nativeOverdub = !g.nativeOverdub
	case midicontrol.CommandParameterDelta:
		g.nudgeNativePerformanceParameter(command.Parameter, command.Delta)
	case midicontrol.CommandConnected:
		g.nativeMIDIName = command.Name
		if command.Detail != "" {
			g.statusMsg = command.Detail
		} else {
			g.statusMsg = command.Name + " connected — zero-config controls ready"
		}
	case midicontrol.CommandProblem:
		g.statusMsg = command.Detail
	case midicontrol.CommandDisconnected:
		g.nativeMIDIName = ""
		var take performance.RawPerformanceTake
		ok := false
		if g.nativeConductor != nil && g.nativeConductor.Active() {
			take, ok = g.nativeConductor.Stop()
		}
		_, _ = g.nativeSession.Disconnect(g.nativeSession.Now())
		if ok {
			g.commitNativeTake(take, fmt.Sprintf("Take %d recovered after MIDI disconnect", g.nativeTakeCount))
		} else {
			g.statusMsg = "Launchkey disconnected — all notes off"
		}
		g.clearNativeCapture()
	}
}

func (g *Game) nativeControllerState() midicontrol.ControllerState {
	state := midicontrol.ControllerState{
		Mode: g.nativeVoiceMode, Metronome: g.nativeMetronome, CountIn: g.nativeCountIn,
		Overdub: g.nativeOverdub, Playing: g.nativeMusicPreviewPlaying(), Parameter: g.nativeMIDIParam, Value: g.nativeMIDIValue,
	}
	if g.nativeSession != nil {
		snapshot := g.nativeSession.Snapshot()
		state.Armed, state.Recording = snapshot.Armed, snapshot.Recording
	}
	if g.nativeConductor != nil && g.nativeConductor.Active() {
		state.Recording = true
	}
	if song, ok := g.model.NativeSong(); ok {
		state.Loop = song.LoopStart != nil
	}
	return state
}

func (g *Game) nudgeNativePerformanceParameter(parameter performance.Parameter, delta int16) {
	if g.nativeSession == nil || delta == 0 {
		return
	}
	bank := g.nativeSession.SoundBank()
	channel := min(max(g.nativeChannel, 0), 3)
	if g.nativeVoiceMode != performance.ModeMono && g.nativeVoiceMode != performance.ModeNoise {
		channel = 0
	}
	sound := bank.Channels[channel]
	clampValue := func(value, low, high int16) int16 { return min(max(value, low), high) }
	var value int16
	var label string
	switch parameter {
	case performance.ParameterDuty:
		values := [...]int16{12, 25, 50, 75}
		index := 2
		switch sound.Kind {
		case runtimeaudio.SoundCH1:
			index = int(sound.Pulse1.Pulse.Duty)
		case runtimeaudio.SoundCH2:
			index = int(sound.Pulse2.Duty)
		case runtimeaudio.SoundCH3:
			high := 0
			for _, sample := range sound.Wave.Wave.Samples[:32] {
				if sample >= 8 {
					high++
				}
			}
			switch {
			case high <= 6:
				index = 0
			case high <= 12:
				index = 1
			case high <= 20:
				index = 2
			default:
				index = 3
			}
		}
		index = int(clampValue(int16(index)+delta, 0, int16(len(values)-1)))
		value, label = values[index], "POLY DUTY"
	case performance.ParameterVolume:
		switch sound.Kind {
		case runtimeaudio.SoundCH2:
			value = int16(sound.Pulse2.Envelope.Initial)
		case runtimeaudio.SoundCH4:
			value = int16(sound.Noise.Envelope.Initial)
		default:
			value = int16(sound.Pulse1.Pulse.Envelope.Initial)
		}
		value, label = clampValue(value+delta, 0, 15), "VOLUME"
	case performance.ParameterEnvelopePace:
		switch sound.Kind {
		case runtimeaudio.SoundCH2:
			value = int16(sound.Pulse2.Envelope.Pace)
		case runtimeaudio.SoundCH4:
			value = int16(sound.Noise.Envelope.Pace)
		default:
			value = int16(sound.Pulse1.Pulse.Envelope.Pace)
		}
		value, label = clampValue(value+delta, 0, 7), "ENV PACE"
	case performance.ParameterEnvelopeDirection:
		value, label = 0, "ENV DIR"
		var direction hw.EnvelopeDirection
		switch sound.Kind {
		case runtimeaudio.SoundCH2:
			direction = sound.Pulse2.Envelope.Direction
		case runtimeaudio.SoundCH4:
			direction = sound.Noise.Envelope.Direction
		default:
			direction = sound.Pulse1.Pulse.Envelope.Direction
		}
		if direction == hw.EnvelopeUp {
			value = 1
		}
	case performance.ParameterSweepPace:
		value, label = clampValue(int16(bank.Channels[0].Pulse1.Sweep.Pace)+delta, 0, 7), "SWEEP PACE"
	case performance.ParameterSweepDirection:
		value, label = 0, "SWEEP DIR"
		if bank.Channels[0].Pulse1.Sweep.Direction == hw.SweepUp {
			value = 1
		}
	case performance.ParameterSweepShift:
		value, label = clampValue(int16(bank.Channels[0].Pulse1.Sweep.Shift)+delta, 0, 7), "SWEEP SHIFT"
	case performance.ParameterWaveLevel:
		current := int16(4)
		switch bank.Channels[2].Wave.Level {
		case hw.WaveMute:
			current = 0
		case hw.Wave25:
			current = 1
		case hw.Wave50:
			current = 2
		case hw.Wave75:
			current = 3
		}
		value, label = clampValue(current+delta, 0, 4), "CH3 LEVEL"
	default:
		return
	}
	// Direction encoders are relative toggles; every non-zero turn changes the
	// binary hardware direction once.
	if parameter == performance.ParameterEnvelopeDirection || parameter == performance.ParameterSweepDirection {
		value = 1 - value
	}
	batch := g.nativeSession.Handle(performance.PerformanceEvent{
		At: g.nativeSession.Now(), Kind: performance.ParameterChange, Source: performance.SourceController, Parameter: parameter, Value: value,
	})
	if batch.Len == 0 {
		g.nativeMIDIParam, g.nativeMIDIValue = "RESERVED", "--"
		g.statusMsg = "That encoder has no hardware control in this mode"
		return
	}
	g.nativeMIDIParam, g.nativeMIDIValue = label, fmt.Sprint(value)
	g.statusMsg = fmt.Sprintf("%s %d", label, value)
}

func (g *Game) updateNativePerformanceKeyboard() bool {
	if g == nil || g.nativeSession == nil || !g.nativeSession.Snapshot().Armed {
		return false
	}
	handled := false
	now := g.nativeSession.Now()
	if inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		g.nativeOctave = max(2, g.nativeOctave-1)
		handled = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyX) {
		g.nativeOctave = min(6, g.nativeOctave+1)
		handled = true
	}
	for semitone, key := range nativePerformanceKeys {
		note := uint8((g.nativeOctave+1)*12 + semitone)
		if g.nativeVoiceMode == performance.ModeNoise {
			note = uint8(36 + semitone%8)
		}
		if inpututil.IsKeyJustPressed(key) {
			g.nativeKeyboardHeld[semitone] = true
			g.nativeSession.Handle(performance.PerformanceEvent{At: now, Kind: performance.NoteOn, Source: performance.SourceKeyboard, Note: note})
			handled = true
		}
		if inpututil.IsKeyJustReleased(key) && g.nativeKeyboardHeld[semitone] {
			g.nativeKeyboardHeld[semitone] = false
			g.nativeSession.Handle(performance.PerformanceEvent{At: now, Kind: performance.NoteOff, Source: performance.SourceKeyboard, Note: note})
			handled = true
		}
	}
	return handled
}

func (g *Game) nativeAllocationText() string {
	if g == nil || g.nativeSession == nil {
		return "ARM TO PLAY LIVE"
	}
	snapshot := g.nativeSession.Snapshot()
	if !snapshot.Armed {
		return "ARM TO PLAY LIVE"
	}
	text := snapshot.Allocation.Mode.String() + "  "
	active := false
	for channel := 0; channel < 4; channel++ {
		if !snapshot.Allocation.Active[channel] {
			continue
		}
		if active {
			text += "  |  "
		}
		text += fmt.Sprintf("%s -> CH%d", nativeMIDINoteName(int(snapshot.Allocation.Notes[channel])), channel+1)
		active = true
	}
	if !active {
		text += "READY"
	}
	return text
}
