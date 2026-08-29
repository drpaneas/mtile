package midicontrol

import (
	"strings"
	"time"

	"github.com/drpaneas/mtile/internal/performance"
)

// LaunchkeyMini25MK4 implements Novation's documented DAW/programmer
// protocol. It deliberately ignores velocity and aftertouch because those do
// not map to Game Boy PSG instrument state.
type LaunchkeyMini25MK4 struct{}

var launchkeyMiniHeader = [...]byte{0xF0, 0x00, 0x20, 0x29, 0x02, 0x13}

func (LaunchkeyMini25MK4) Matches(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "launchkey") && strings.Contains(name, "mini") && strings.Contains(name, "mk4")
}

func (LaunchkeyMini25MK4) Role(name string) PortRole {
	name = strings.ToLower(name)
	if strings.Contains(name, "daw") || strings.Contains(name, "midiin2") || strings.Contains(name, "midiout2") {
		return PortDAW
	}
	return PortKeys
}

func (LaunchkeyMini25MK4) DisplayName(endpoints []Endpoint) string {
	for _, endpoint := range endpoints {
		name := strings.TrimSpace(endpoint.Name)
		lower := strings.ToLower(name)
		if !strings.Contains(lower, "launchkey") {
			continue
		}
		for _, suffix := range []string{" MIDI Out", " MIDI In", " DAW Out", " DAW In", " MIDI", " DAW"} {
			if strings.HasSuffix(strings.ToLower(name), strings.ToLower(suffix)) {
				name = strings.TrimSpace(name[:len(name)-len(suffix)])
				break
			}
		}
		if name != "" {
			return name
		}
	}
	return "Launchkey Mini 25 MK4"
}

func (LaunchkeyMini25MK4) Decode(role PortRole, data []byte, at time.Duration) CommandBatch {
	var batch CommandBatch
	if len(data) == 0 {
		return batch
	}
	if data[0] == 0xFA {
		batch.Append(Command{Kind: CommandPlayToggle})
		return batch
	}
	if data[0] == 0xFC {
		batch.Append(Command{Kind: CommandStop})
		return batch
	}
	if len(data) < 3 {
		return batch
	}
	status, value1, value2 := data[0], data[1], data[2]
	message, channel := status&0xF0, status&0x0F
	if message == 0x90 || message == 0x80 {
		pressed := message == 0x90 && value2 != 0
		if role == PortDAW {
			if !pressed {
				return batch
			}
			if command, ok := launchkeyPadCommand(value1); ok {
				command.Performance.At = at
				batch.Append(command)
			}
			return batch
		}
		kind := performance.NoteOff
		if pressed {
			kind = performance.NoteOn
		}
		batch.Append(Command{Kind: CommandPerformance, Performance: performance.PerformanceEvent{
			At: at, Kind: kind, Source: performance.SourceMIDI, Note: value1,
		}})
		return batch
	}
	if message != 0xB0 {
		return batch
	}
	// DAW controls and relative encoders report on MIDI channel 16.
	if channel == 15 {
		switch value1 {
		case 114:
			if value2 != 0 {
				batch.Append(Command{Kind: CommandStop})
			}
		case 115:
			if value2 != 0 {
				batch.Append(Command{Kind: CommandPlayToggle})
			}
		case 116:
			if value2 != 0 {
				batch.Append(Command{Kind: CommandLoopToggle})
			}
		case 117:
			if value2 != 0 {
				batch.Append(Command{Kind: CommandRecordToggle})
			}
		default:
			if value1 >= 85 && value1 <= 92 && value2 != 64 {
				parameters := [...]performance.Parameter{
					performance.ParameterDuty,
					performance.ParameterVolume,
					performance.ParameterEnvelopePace,
					performance.ParameterEnvelopeDirection,
					performance.ParameterSweepPace,
					performance.ParameterSweepDirection,
					performance.ParameterSweepShift,
					performance.ParameterWaveLevel,
				}
				batch.Append(Command{Kind: CommandParameterDelta, Parameter: parameters[value1-85], Delta: int16(value2) - 64})
			}
		}
	}
	return batch
}

func launchkeyPadCommand(note byte) (Command, bool) {
	// DAW pad mode reports the top row as 96..103 and the bottom row as
	// 112..119. This gives the requested eight drums plus eight workflow pads.
	if note >= 96 && note <= 103 {
		return Command{Kind: CommandPerformance, Performance: performance.PerformanceEvent{
			Kind: performance.NoteOn, Source: performance.SourceController, Note: 36 + note - 96,
		}}, true
	}
	if note < 112 || note > 119 {
		return Command{}, false
	}
	switch note {
	case 112:
		return Command{Kind: CommandMode, Mode: performance.ModeMono}, true
	case 113:
		return Command{Kind: CommandMode, Mode: performance.ModePoly2}, true
	case 114:
		return Command{Kind: CommandMode, Mode: performance.ModePoly3}, true
	case 115:
		return Command{Kind: CommandMode, Mode: performance.ModeNoise}, true
	case 116:
		return Command{Kind: CommandMetronomeToggle}, true
	case 117:
		return Command{Kind: CommandCountInCycle}, true
	case 118:
		return Command{Kind: CommandLoopToggle}, true
	default:
		return Command{Kind: CommandOverdubToggle}, true
	}
}

func (LaunchkeyMini25MK4) Initialize() [][]byte {
	return [][]byte{
		{0x9F, 0x0C, 0x7F}, // DAW mode on.
		{0x9F, 0x0B, 0x7F}, // Feature controls on.
		{0xB6, 0x1D, 0x02}, // DAW pad layout.
		{0xB6, 0x1E, 0x05}, // Relative transport encoders.
		{0xB6, 0x45, 0x01}, // Relative encoder output.
		launchkeyDisplayConfigure(0x20, 0x01),
		launchkeyDisplayText(0x20, 0, "MTILE"),
		launchkeyDisplayText(0x20, 1, "MUSIC LAB"),
	}
}

func (LaunchkeyMini25MK4) Shutdown() [][]byte {
	return [][]byte{{0x9F, 0x0C, 0x00}, {0x9F, 0x0B, 0x00}}
}

func (LaunchkeyMini25MK4) Feedback(state ControllerState) [][]byte {
	const (
		dim    = byte(3)
		yellow = byte(13)
		green  = byte(21)
		red    = byte(5)
		blue   = byte(45)
		purple = byte(49)
	)
	messages := make([][]byte, 0, 22)
	for pad := byte(96); pad <= 103; pad++ {
		messages = append(messages, []byte{0x90, pad, red})
	}
	for offset, mode := range [...]performance.VoiceMode{performance.ModeMono, performance.ModePoly2, performance.ModePoly3, performance.ModeNoise} {
		colour := dim
		if state.Mode == mode {
			colour = yellow
		}
		messages = append(messages, []byte{0x90, byte(112 + offset), colour})
	}
	flags := [...]struct {
		on     bool
		colour byte
	}{{state.Metronome, blue}, {state.CountIn > 0, blue}, {state.Loop, green}, {state.Overdub, purple}}
	for index, flag := range flags {
		colour := dim
		if flag.on {
			colour = flag.colour
		}
		messages = append(messages, []byte{0x90, byte(116 + index), colour})
	}
	// Record flashes on MIDI channel 2; ARM and play use their monochrome
	// transport LEDs when present.
	if state.Recording {
		messages = append(messages, []byte{0xB1, 117, red})
	} else {
		messages = append(messages, []byte{0xB0, 117, boolVelocity(state.Armed, yellow)})
	}
	messages = append(messages,
		[]byte{0xB0, 115, boolVelocity(state.Playing, green)},
		launchkeyDisplayConfigure(0x20, 0x01),
		launchkeyDisplayText(0x20, 0, state.Mode.String()),
		launchkeyDisplayText(0x20, 1, launchkeyStateLine(state)),
	)
	if state.Parameter != "" {
		messages = append(messages,
			launchkeyDisplayConfigure(0x21, 0x01),
			launchkeyDisplayText(0x21, 0, state.Parameter),
			launchkeyDisplayText(0x21, 1, state.Value),
		)
	}
	return messages
}

func boolVelocity(on bool, value byte) byte {
	if on {
		return value
	}
	return 0
}

func launchkeyStateLine(state ControllerState) string {
	if state.Recording {
		return "REC"
	}
	if state.Armed {
		return "ARMED"
	}
	return "READY"
}

func launchkeyDisplayConfigure(target, config byte) []byte {
	message := append([]byte(nil), launchkeyMiniHeader[:]...)
	return append(message, 0x04, target, config, 0xF7)
}

func launchkeyDisplayText(target, field byte, value string) []byte {
	message := append([]byte(nil), launchkeyMiniHeader[:]...)
	message = append(message, 0x06, target, field)
	for _, r := range value {
		if r >= 0x20 && r <= 0x7E {
			message = append(message, byte(r))
		}
	}
	return append(message, 0xF7)
}
