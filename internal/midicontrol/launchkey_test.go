package midicontrol

import (
	"bytes"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/performance"
)

func TestLaunchkeyKeysPadsEncodersAndTransport(t *testing.T) {
	profile := LaunchkeyMini25MK4{}
	assertCommand := func(role PortRole, message []byte, kind CommandKind) Command {
		t.Helper()
		batch := profile.Decode(role, message, 17*time.Millisecond)
		if len(batch.Slice()) != 1 || batch.Slice()[0].Kind != kind {
			t.Fatalf("Decode(% X) = %#v, want one %d", message, batch.Slice(), kind)
		}
		return batch.Slice()[0]
	}
	note := assertCommand(PortKeys, []byte{0x90, 60, 100}, CommandPerformance)
	if note.Performance.Kind != performance.NoteOn || note.Performance.Note != 60 || note.Performance.At != 17*time.Millisecond {
		t.Fatalf("key note = %#v", note.Performance)
	}
	note = assertCommand(PortKeys, []byte{0x90, 60, 0}, CommandPerformance)
	if note.Performance.Kind != performance.NoteOff {
		t.Fatalf("zero-velocity note = %#v", note.Performance)
	}
	drum := assertCommand(PortDAW, []byte{0x90, 103, 127}, CommandPerformance)
	if drum.Performance.Note != 43 || drum.Performance.Source != performance.SourceController {
		t.Fatalf("drum pad = %#v", drum.Performance)
	}
	mode := assertCommand(PortDAW, []byte{0x90, 114, 127}, CommandMode)
	if mode.Mode != performance.ModePoly3 {
		t.Fatalf("mode pad = %v", mode.Mode)
	}
	parameter := assertCommand(PortDAW, []byte{0xBF, 85, 67}, CommandParameterDelta)
	if parameter.Parameter != performance.ParameterDuty || parameter.Delta != 3 {
		t.Fatalf("encoder = %#v", parameter)
	}
	assertCommand(PortDAW, []byte{0xBF, 115, 127}, CommandPlayToggle)
	assertCommand(PortDAW, []byte{0xBF, 117, 127}, CommandRecordToggle)
}

func TestLaunchkeyInitializationUsesDocumentedMiniProtocol(t *testing.T) {
	messages := (LaunchkeyMini25MK4{}).Initialize()
	want := [][]byte{
		{0x9F, 0x0C, 0x7F},
		{0x9F, 0x0B, 0x7F},
		{0xB6, 0x1D, 0x02},
		{0xB6, 0x1E, 0x05},
		{0xB6, 0x45, 0x01},
		{0xF0, 0x00, 0x20, 0x29, 0x02, 0x13, 0x04, 0x20, 0x01, 0xF7},
	}
	if len(messages) < len(want) {
		t.Fatalf("initialization has %d messages, want at least %d", len(messages), len(want))
	}
	for index := range want {
		if !bytes.Equal(messages[index], want[index]) {
			t.Fatalf("initialization[%d] = % X, want % X", index, messages[index], want[index])
		}
	}
	if got := messages[len(messages)-1]; !bytes.HasPrefix(got, launchkeyMiniHeader[:]) || got[len(got)-1] != 0xF7 {
		t.Fatalf("OLED text is not valid Mini SysEx: % X", got)
	}
}

func TestLaunchkeyFeedbackUsesPadColoursFlashingRecordAndOLED(t *testing.T) {
	messages := (LaunchkeyMini25MK4{}).Feedback(ControllerState{
		Mode: performance.ModePoly3, Armed: true, Recording: true, Playing: true, Metronome: true, CountIn: 1, Loop: true, Overdub: true,
	})
	contains := func(want []byte) bool {
		for _, message := range messages {
			if bytes.Equal(message, want) {
				return true
			}
		}
		return false
	}
	for _, want := range [][]byte{{0x90, 114, 13}, {0x90, 116, 45}, {0xB1, 117, 5}, {0xB0, 115, 21}} {
		if !contains(want) {
			t.Fatalf("feedback omitted % X: %#v", want, messages)
		}
	}
}
