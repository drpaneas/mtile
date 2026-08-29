// Package midicontrol turns MIDI controller messages into source-independent
// performance and editor commands. MIDI is an authoring input only; none of
// these types are part of the cartridge format.
package midicontrol

import (
	"io"
	"time"

	"github.com/drpaneas/mtile/internal/performance"
)

type PortRole uint8

const (
	PortKeys PortRole = iota
	PortDAW
)

type CommandKind uint8

const (
	CommandPerformance CommandKind = iota
	CommandMode
	CommandArmToggle
	CommandRecordToggle
	CommandPlayToggle
	CommandStop
	CommandMetronomeToggle
	CommandCountInCycle
	CommandLoopToggle
	CommandOverdubToggle
	CommandParameterDelta
	CommandConnected
	CommandDisconnected
	CommandProblem
)

type Command struct {
	Kind        CommandKind
	Performance performance.PerformanceEvent
	Mode        performance.VoiceMode
	Parameter   performance.Parameter
	Delta       int16
	Name        string
	Detail      string
}

const MaxCommandsPerMessage = 4

type CommandBatch struct {
	commands [MaxCommandsPerMessage]Command
	length   uint8
}

func (b *CommandBatch) Append(command Command) {
	if int(b.length) >= len(b.commands) {
		return
	}
	b.commands[b.length] = command
	b.length++
}

func (b CommandBatch) Slice() []Command { return b.commands[:b.length] }

type ControllerState struct {
	Mode      performance.VoiceMode
	Armed     bool
	Recording bool
	Playing   bool
	Metronome bool
	CountIn   int
	Loop      bool
	Overdub   bool
	Parameter string
	Value     string
}

type Endpoint struct {
	ID     string
	Name   string
	Input  bool
	Output bool
}

type Output interface {
	io.Closer
	Send([]byte) error
}

type Backend interface {
	List() ([]Endpoint, error)
	Listen(Endpoint, func([]byte)) (io.Closer, error)
	OpenOutput(Endpoint) (Output, error)
	Close() error
}

type Profile interface {
	Matches(string) bool
	Role(string) PortRole
	Decode(PortRole, []byte, time.Duration) CommandBatch
	Initialize() [][]byte
	Shutdown() [][]byte
	Feedback(ControllerState) [][]byte
}

// ProfileDisplayNamer lets a device profile collapse CoreMIDI's separate
// input/output endpoint names into the product name shown in the UI.
type ProfileDisplayNamer interface {
	DisplayName([]Endpoint) string
}
