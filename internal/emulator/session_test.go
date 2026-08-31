package emulator

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"strconv"
	"testing"
)

func TestReadFrame(t *testing.T) {
	var stream bytes.Buffer
	stream.WriteString("FRAM")
	_ = binary.Write(&stream, binary.LittleEndian, uint32(7))
	_ = binary.Write(&stream, binary.LittleEndian, uint32(2))
	_ = binary.Write(&stream, binary.LittleEndian, uint32(1))
	_ = binary.Write(&stream, binary.LittleEndian, uint32(telemetryPayloadSize))
	stream.Write(make([]byte, Width*Height*4))
	stream.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	stream.Write(testTelemetryPayload())
	session := &Session{stdout: nopReadCloser{Buffer: &stream}}
	frame, err := session.readFrame()
	if err != nil {
		t.Fatal(err)
	}
	if frame.Number != 7 || len(frame.RGBA) != Width*Height*4 || len(frame.PCM) != 8 || !frame.Overflow || !frame.Telemetry.Valid {
		t.Fatalf("unexpected frame: number=%d rgba=%d pcm=%d overflow=%v", frame.Number, len(frame.RGBA), len(frame.PCM), frame.Overflow)
	}
}

func TestReadFrameRejectsUnboundedAudio(t *testing.T) {
	var stream bytes.Buffer
	stream.WriteString("FRAM")
	_ = binary.Write(&stream, binary.LittleEndian, uint32(0))
	_ = binary.Write(&stream, binary.LittleEndian, uint32(maxAudio+1))
	_ = binary.Write(&stream, binary.LittleEndian, uint32(0))
	_ = binary.Write(&stream, binary.LittleEndian, uint32(0))
	session := &Session{stdout: nopReadCloser{Buffer: &stream}}
	if _, err := session.readFrame(); err == nil {
		t.Fatal("expected oversized audio error")
	}
}

func testTelemetryPayload() []byte {
	data := make([]byte, telemetryPayloadSize)
	copy(data[:4], "TELM")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], telemetryPayloadSize)
	binary.LittleEndian.PutUint16(data[8:10], 123)
	binary.LittleEndian.PutUint16(data[10:12], 0x03fe)
	binary.LittleEndian.PutUint16(data[12:14], 0x17)
	mailbox := data[debugMailboxOffset:]
	copy(mailbox[:4], "GLUD")
	binary.LittleEndian.PutUint16(mailbox[4:6], 1)
	binary.LittleEndian.PutUint16(mailbox[6:8], debugMailboxSize)
	binary.LittleEndian.PutUint32(mailbox[8:12], 2)
	binary.LittleEndian.PutUint32(mailbox[12:16], 99)
	binary.LittleEndian.PutUint32(mailbox[debugHeapCapacity:], 56*1024)
	binary.LittleEndian.PutUint32(mailbox[debugHeapAllocated:], 41*1024)
	binary.LittleEndian.PutUint32(mailbox[debugHeapLive:], 24*1024)
	binary.LittleEndian.PutUint32(mailbox[debugHeapPeakLive:], 28*1024)
	binary.LittleEndian.PutUint32(mailbox[debugHeapHandles:], 321)
	binary.LittleEndian.PutUint32(mailbox[debugHeapHandleLimit:], 2048)
	binary.LittleEndian.PutUint32(mailbox[debugHeapError:], uint32(HeapOOM))
	mailbox[68] = 1
	copy(mailbox[debugWatchBase:], "score")
	mailbox[debugWatchBase+16] = 2
	binary.LittleEndian.PutUint32(mailbox[debugWatchBase+20:], 3<<16)
	return data
}

func TestDecodeTelemetry(t *testing.T) {
	telemetry := decodeTelemetry(testTelemetryPayload())
	if !telemetry.Valid || !telemetry.DebugValid || telemetry.VCount != 123 || telemetry.PSG.CH1Sweep != 0x17 {
		t.Fatalf("telemetry = %+v", telemetry)
	}
	if telemetry.Stats.Frames != 99 || len(telemetry.Watches) != 1 || telemetry.Watches[0].Name != "score" || telemetry.Watches[0].Raw != 3<<16 {
		t.Fatalf("debug telemetry = %+v", telemetry)
	}
	if telemetry.Stats.HeapCapacity != 56*1024 || telemetry.Stats.HeapAllocated != 41*1024 || telemetry.Stats.HeapLive != 24*1024 || telemetry.Stats.HeapPeakLive != 28*1024 {
		t.Fatalf("heap byte telemetry = %+v", telemetry.Stats)
	}
	if telemetry.Stats.HeapHandles != 321 || telemetry.Stats.HeapHandleLimit != 2048 || telemetry.Stats.HeapError != HeapOOM {
		t.Fatalf("heap status telemetry = %+v", telemetry.Stats)
	}
}

type nopReadCloser struct{ *bytes.Buffer }

func (nopReadCloser) Close() error { return nil }

func TestSessionIntegration(t *testing.T) {
	helper, rom := os.Getenv("MTILE_TEST_PLAYER"), os.Getenv("MTILE_TEST_ROM")
	if helper == "" || rom == "" {
		t.Skip("set MTILE_TEST_PLAYER and MTILE_TEST_ROM")
	}
	session, err := Start(helper, rom)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var keys uint16
	if value := os.Getenv("MTILE_TEST_KEYS"); value != "" {
		parsed, err := strconv.ParseUint(value, 0, 16)
		if err != nil {
			t.Fatalf("parse MTILE_TEST_KEYS: %v", err)
		}
		keys = uint16(parsed)
	}
	inputStart := 0
	if value := os.Getenv("MTILE_TEST_INPUT_START"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			t.Fatalf("parse MTILE_TEST_INPUT_START %q", value)
		}
		inputStart = parsed
	}
	frameCount := 120
	if value := os.Getenv("MTILE_TEST_FRAMES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			t.Fatalf("parse MTILE_TEST_FRAMES %q", value)
		}
		frameCount = parsed
	}
	var previous uint32
	visual, audible := false, false
	totalAudioFrames := 0
	var last Frame
	for index := 0; index < frameCount; index++ {
		frameKeys := keys
		if index < inputStart {
			frameKeys = 0
		}
		frame, err := session.Step(frameKeys, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(frame.RGBA) != Width*Height*4 || len(frame.PCM) == 0 {
			t.Fatalf("frame %d: rgba=%d pcm=%d", index, len(frame.RGBA), len(frame.PCM))
		}
		if frame.Overflow {
			t.Fatalf("frame %d: emulator audio capture overflowed", index)
		}
		totalAudioFrames += len(frame.PCM) / 4
		if index > 0 && frame.Number <= previous {
			t.Fatalf("frame counter did not advance: %d after %d", frame.Number, previous)
		}
		previous = frame.Number
		first := frame.RGBA[:3]
		for pixel := 4; pixel+2 < len(frame.RGBA); pixel += 4 {
			if !bytes.Equal(frame.RGBA[pixel:pixel+3], first) {
				visual = true
				break
			}
		}
		for _, sampleByte := range frame.PCM {
			if sampleByte != 0 {
				audible = true
				break
			}
		}
		last = frame
	}
	if !visual {
		t.Fatal("emulator returned only flat-color frames")
	}
	if frameCount >= 60 {
		expected := frameCount * SampleRate / 60
		tolerance := expected / 20
		if totalAudioFrames < expected-tolerance || totalAudioFrames > expected+tolerance {
			t.Fatalf("emulator PCM rate = %d frames across %d video frames, want %d +/- %d at %d Hz", totalAudioFrames, frameCount, expected, tolerance, SampleRate)
		}
		measuredRate := float64(totalAudioFrames) * 60 / float64(frameCount)
		t.Logf("emulator PCM rate: %.2f Hz", measuredRate)
	}
	if os.Getenv("MTILE_TEST_EXPECT_AUDIO") == "1" && !audible {
		t.Fatal("emulator returned only silent PCM")
	}
	if output := os.Getenv("MTILE_TEST_FRAME_PNG"); output != "" {
		file, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		picture := &image.RGBA{Pix: last.RGBA, Stride: Width * 4, Rect: image.Rect(0, 0, Width, Height)}
		if err := png.Encode(file, picture); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("MTILE_PLAYER_TRACE_KEYS") != "" {
		t.Log(session.stderr.String())
	}
}
