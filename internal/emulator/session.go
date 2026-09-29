// Package emulator implements the private protocol between the Go editor and
// Mtile's headless mGBA helper. It has no dependency on mGBA headers or cgo.
package emulator

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

const (
	Width        = 240
	Height       = 160
	SampleRate   = 48000
	protocol     = 2
	maxAudio     = 4096
	maxTelemetry = 1024
)

const (
	commandStep  = 1
	commandReset = 2
	commandQuit  = 3
)

// GBA key bit positions, matching the hardware KEYINPUT register and mGBA.
const (
	KeyA uint16 = 1 << iota
	KeyB
	KeySelect
	KeyStart
	KeyRight
	KeyLeft
	KeyUp
	KeyDown
	KeyR
	KeyL
)

type Frame struct {
	Number    uint32
	RGBA      []byte
	PCM       []byte // signed 16-bit little-endian stereo at SampleRate
	Overflow  bool
	Telemetry Telemetry
}

type PSGRegisters struct {
	CH1Sweep, CH1Duty, CH1Frequency     uint16
	CH2Duty, CH2Frequency               uint16
	CH3Control, CH3Length, CH3Frequency uint16
	CH4Envelope, CH4Frequency           uint16
	MixPSG, MixDMA, Status, Bias        uint16
}

// HeapError mirrors the stable numeric heap status sent by the preview ROM.
// Keep these values in lockstep with runtime.HeapError without importing the
// cartridge runtime into the editor-side emulator protocol.
type HeapError uint32

const (
	HeapOK HeapError = iota
	HeapOOM
	HeapCorrupt
	HeapCorruptRef
	HeapBusy
)

type DebugStats struct {
	Frames, MissedFrames, LastFrame, MaxFrame uint32
	LastUpdate, MaxUpdate, LastDraw, MaxDraw  uint32
	GCCount, LastGC, MaxGC, LastBlit          uint32
	PresentStart, PresentEnd                  uint16
	DiagnosticSkips                           uint32
	HeapCapacity, HeapAllocated               uint32
	HeapLive, HeapPeakLive                    uint32
	HeapHandles, HeapHandleLimit              uint32
	HeapError                                 HeapError
	OBJTelemetryValid                         bool
	OBJStreamAvailable                        bool
	OBJStreamResidencyValid                   bool
	OBJStreamResidentBytes                    uint32
	OBJStreamResidentSlots                    uint32
	OBJUploadedBytes                          uint32
	// The first min(OBJStreamResidentSlots, 4) IDs name cached slots, not
	// necessarily visible poses. Remaining entries are 0xffff. A valid
	// prefix entry may itself be pose 65535, so use the count, not a sentinel.
	OBJStreamResidentPoseIDs [4]uint16
}

type Watch struct {
	Name string
	Kind uint8
	Raw  int32
}

type Telemetry struct {
	Valid      bool
	VCount     uint16
	KeyInput   uint16
	PSG        PSGRegisters
	DebugValid bool
	Sequence   uint32
	Stats      DebugStats
	Watches    []Watch
}

type Session struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr diagnosticLog
	mu     sync.Mutex
	closed bool
}

func Start(helperPath, romPath string) (*Session, error) {
	if helperPath == "" {
		return nil, errors.New("emulator helper path is empty")
	}
	if romPath == "" {
		return nil, errors.New("ROM path is empty")
	}
	cmd := exec.Command(helperPath, romPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	session := &Session{cmd: cmd, stdin: stdin, stdout: stdout}
	cmd.Stderr = &session.stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("start emulator helper: %w", err)
	}
	if err := session.readHandshake(); err != nil {
		_ = session.Close()
		if log := session.stderr.String(); log != "" {
			return nil, fmt.Errorf("%w: %s", err, log)
		}
		return nil, err
	}
	return session, nil
}

func (s *Session) readHandshake() error {
	var header [16]byte
	if _, err := io.ReadFull(s.stdout, header[:]); err != nil {
		return fmt.Errorf("read emulator handshake: %w", err)
	}
	if string(header[:4]) != "GLUP" {
		return fmt.Errorf("invalid emulator handshake %q", header[:4])
	}
	version := binary.LittleEndian.Uint16(header[4:6])
	width := binary.LittleEndian.Uint16(header[6:8])
	height := binary.LittleEndian.Uint16(header[8:10])
	rate := binary.LittleEndian.Uint32(header[12:16])
	if version != protocol || width != Width || height != Height || rate != SampleRate {
		return fmt.Errorf("unsupported emulator protocol v%d %dx%d @ %d Hz", version, width, height, rate)
	}
	return nil
}

func (s *Session) Step(keys uint16, reset bool) (Frame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Frame{}, errors.New("emulator session is closed")
	}
	op := byte(commandStep)
	if reset {
		op = commandReset
	}
	command := [4]byte{op, 0, byte(keys), byte(keys >> 8)}
	if _, err := s.stdin.Write(command[:]); err != nil {
		return Frame{}, s.withLog(fmt.Errorf("send emulator input: %w", err))
	}
	return s.readFrame()
}

func (s *Session) readFrame() (Frame, error) {
	var header [20]byte
	if _, err := io.ReadFull(s.stdout, header[:]); err != nil {
		return Frame{}, s.withLog(fmt.Errorf("read emulator frame: %w", err))
	}
	if string(header[:4]) != "FRAM" {
		return Frame{}, fmt.Errorf("invalid emulator frame marker %q", header[:4])
	}
	number := binary.LittleEndian.Uint32(header[4:8])
	audioFrames := binary.LittleEndian.Uint32(header[8:12])
	if audioFrames > maxAudio {
		return Frame{}, fmt.Errorf("emulator sent %d audio frames, maximum is %d", audioFrames, maxAudio)
	}
	telemetrySize := binary.LittleEndian.Uint32(header[16:20])
	if telemetrySize > maxTelemetry {
		return Frame{}, fmt.Errorf("emulator sent %d telemetry bytes, maximum is %d", telemetrySize, maxTelemetry)
	}
	frame := Frame{
		Number:   number,
		RGBA:     make([]byte, Width*Height*4),
		PCM:      make([]byte, int(audioFrames)*4),
		Overflow: binary.LittleEndian.Uint32(header[12:16]) != 0,
	}
	if _, err := io.ReadFull(s.stdout, frame.RGBA); err != nil {
		return Frame{}, s.withLog(fmt.Errorf("read emulator pixels: %w", err))
	}
	if _, err := io.ReadFull(s.stdout, frame.PCM); err != nil {
		return Frame{}, s.withLog(fmt.Errorf("read emulator audio: %w", err))
	}
	telemetry := make([]byte, int(telemetrySize))
	if _, err := io.ReadFull(s.stdout, telemetry); err != nil {
		return Frame{}, s.withLog(fmt.Errorf("read emulator telemetry: %w", err))
	}
	frame.Telemetry = decodeTelemetry(telemetry)
	return frame, nil
}

const (
	telemetryPayloadSize = 552
	debugMailboxOffset   = 40
	debugMailboxSize     = 512
	debugWatchBase       = 72
	debugWatchSize       = 24
	debugWatchSlots      = 16
	debugHeapBase        = debugWatchBase + debugWatchSlots*debugWatchSize
	debugHeapCapacity    = debugHeapBase
	debugHeapAllocated   = debugHeapBase + 4
	debugHeapLive        = debugHeapBase + 8
	debugHeapPeakLive    = debugHeapBase + 12
	debugHeapHandles     = debugHeapBase + 16
	debugHeapHandleLimit = debugHeapBase + 20
	debugHeapError       = debugHeapBase + 24
	debugOBJBase         = debugHeapError + 4
	debugOBJMagic        = uint32(0x314a424f)
	debugOBJFlags        = debugOBJBase + 4
	debugOBJResident     = debugOBJBase + 8
	debugOBJSlots        = debugOBJBase + 12
	debugOBJUploaded     = debugOBJBase + 16
	debugOBJPoseIDs      = debugOBJBase + 20
	debugOBJAvailable    = uint32(1 << 0)
	debugOBJResidencySet = uint32(1 << 1)
)

func decodeTelemetry(data []byte) Telemetry {
	if len(data) != telemetryPayloadSize || string(data[:4]) != "TELM" || binary.LittleEndian.Uint16(data[4:6]) != 1 || int(binary.LittleEndian.Uint16(data[6:8])) != len(data) {
		return Telemetry{}
	}
	result := Telemetry{Valid: true, VCount: binary.LittleEndian.Uint16(data[8:10]), KeyInput: binary.LittleEndian.Uint16(data[10:12])}
	registers := make([]uint16, 14)
	for index := range registers {
		registers[index] = binary.LittleEndian.Uint16(data[12+index*2:])
	}
	result.PSG = PSGRegisters{
		CH1Sweep: registers[0], CH1Duty: registers[1], CH1Frequency: registers[2],
		CH2Duty: registers[3], CH2Frequency: registers[4],
		CH3Control: registers[5], CH3Length: registers[6], CH3Frequency: registers[7],
		CH4Envelope: registers[8], CH4Frequency: registers[9],
		MixPSG: registers[10], MixDMA: registers[11], Status: registers[12], Bias: registers[13],
	}
	mailbox := data[debugMailboxOffset:]
	if string(mailbox[:4]) != "GLUD" || binary.LittleEndian.Uint16(mailbox[4:6]) != 1 || binary.LittleEndian.Uint16(mailbox[6:8]) != debugMailboxSize {
		return result
	}
	sequence := binary.LittleEndian.Uint32(mailbox[8:12])
	if sequence&1 != 0 {
		return result
	}
	result.DebugValid, result.Sequence = true, sequence
	values := make([]uint32, 12)
	for index := range values {
		values[index] = binary.LittleEndian.Uint32(mailbox[12+index*4:])
	}
	result.Stats = DebugStats{
		Frames: values[0], MissedFrames: values[1], LastFrame: values[2], MaxFrame: values[3],
		LastUpdate: values[4], MaxUpdate: values[5], LastDraw: values[6], MaxDraw: values[7],
		GCCount: values[8], LastGC: values[9], MaxGC: values[10], LastBlit: values[11],
		PresentStart: binary.LittleEndian.Uint16(mailbox[60:62]), PresentEnd: binary.LittleEndian.Uint16(mailbox[62:64]),
		DiagnosticSkips: binary.LittleEndian.Uint32(mailbox[64:68]),
		HeapCapacity:    binary.LittleEndian.Uint32(mailbox[debugHeapCapacity:]),
		HeapAllocated:   binary.LittleEndian.Uint32(mailbox[debugHeapAllocated:]),
		HeapLive:        binary.LittleEndian.Uint32(mailbox[debugHeapLive:]),
		HeapPeakLive:    binary.LittleEndian.Uint32(mailbox[debugHeapPeakLive:]),
		HeapHandles:     binary.LittleEndian.Uint32(mailbox[debugHeapHandles:]),
		HeapHandleLimit: binary.LittleEndian.Uint32(mailbox[debugHeapHandleLimit:]),
		HeapError:       HeapError(binary.LittleEndian.Uint32(mailbox[debugHeapError:])),
	}
	if binary.LittleEndian.Uint32(mailbox[debugOBJBase:]) == debugOBJMagic {
		flags := binary.LittleEndian.Uint32(mailbox[debugOBJFlags:])
		result.Stats.OBJTelemetryValid = true
		result.Stats.OBJStreamAvailable = flags&debugOBJAvailable != 0
		result.Stats.OBJStreamResidencyValid = result.Stats.OBJStreamAvailable && flags&debugOBJResidencySet != 0
		result.Stats.OBJStreamResidentBytes = binary.LittleEndian.Uint32(mailbox[debugOBJResident:])
		result.Stats.OBJStreamResidentSlots = binary.LittleEndian.Uint32(mailbox[debugOBJSlots:])
		result.Stats.OBJUploadedBytes = binary.LittleEndian.Uint32(mailbox[debugOBJUploaded:])
		for index := range result.Stats.OBJStreamResidentPoseIDs {
			result.Stats.OBJStreamResidentPoseIDs[index] = binary.LittleEndian.Uint16(mailbox[debugOBJPoseIDs+index*2:])
		}
	}
	count := min(int(mailbox[68]), debugWatchSlots)
	result.Watches = make([]Watch, 0, count)
	for index := 0; index < count; index++ {
		offset := debugWatchBase + index*debugWatchSize
		nameBytes := mailbox[offset : offset+16]
		if end := bytes.IndexByte(nameBytes, 0); end >= 0 {
			nameBytes = nameBytes[:end]
		}
		result.Watches = append(result.Watches, Watch{Name: string(nameBytes), Kind: mailbox[offset+16], Raw: int32(binary.LittleEndian.Uint32(mailbox[offset+20:]))})
	}
	return result
}

func (s *Session) withLog(err error) error {
	if log := s.stderr.String(); log != "" {
		return fmt.Errorf("%w: %s", err, log)
	}
	return err
}

func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	_, _ = s.stdin.Write([]byte{commandQuit, 0, 0, 0})
	_ = s.stdin.Close()
	_ = s.stdout.Close()
	s.mu.Unlock()
	if err := s.cmd.Wait(); err != nil {
		// A closed pipe can make the helper observe EOF/SIGPIPE during normal
		// shutdown. Surface a real diagnostic only when it wrote one.
		if log := s.stderr.String(); log != "" {
			return fmt.Errorf("emulator helper stopped: %w: %s", err, log)
		}
	}
	return nil
}
