package emulator

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

type captureWriteCloser struct{ bytes.Buffer }

func (*captureWriteCloser) Close() error { return nil }

func checkpointTestData() []byte {
	data := make([]byte, checkpointHeader+4)
	copy(data, "MTSS")
	binary.LittleEndian.PutUint32(data[4:8], 4)
	binary.LittleEndian.PutUint32(data[8:12], 123)
	binary.LittleEndian.PutUint32(data[12:16], 42)
	binary.LittleEndian.PutUint32(data[16:20], uint32(KeyRight))
	binary.LittleEndian.PutUint32(data[20:24], 4)
	binary.LittleEndian.PutUint32(data[1080:1084], 4)
	binary.LittleEndian.PutUint32(data[1084:1088], Width*Height*4)
	copy(data[checkpointHeader:], []byte{1, 2, 3, 4})
	binary.LittleEndian.PutUint32(data[24:28], checkpointCRC(data))
	return data
}

func controlReply(status uint32, data []byte) []byte {
	var b bytes.Buffer
	b.WriteString("DATA")
	_ = binary.Write(&b, binary.LittleEndian, status)
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

func captureTestSession(reply []byte) (*Session, *captureWriteCloser) {
	w := &captureWriteCloser{}
	return &Session{stdin: w, stdout: nopReadCloser{bytes.NewBuffer(reply)}}, w
}

func TestCheckpointProtocolRoundTrip(t *testing.T) {
	state := checkpointTestData()
	s, written := captureTestSession(append(controlReply(0, state), controlReply(0, nil)...))
	got, err := s.SaveState()
	if err != nil || !bytes.Equal(got, state) {
		t.Fatalf("save: state=%x error=%v", got, err)
	}
	if err := s.LoadState(got); err != nil {
		t.Fatal(err)
	}
	expected := []byte{commandSaveState, 0, 0, 0, commandLoadState, 0, 0, 0}
	expected = binary.LittleEndian.AppendUint32(expected, uint32(len(state)))
	expected = append(expected, state...)
	if !bytes.Equal(written.Bytes(), expected) {
		t.Fatalf("commands=%x want=%x", written.Bytes(), expected)
	}
}

func TestCheckpointRejectsDamagedStateBeforeSending(t *testing.T) {
	for _, change := range []struct {
		name string
		fn   func([]byte) []byte
	}{
		{"short", func(b []byte) []byte { return b[:12] }},
		{"version", func(b []byte) []byte { b[4] = 99; return b }},
		{"length", func(b []byte) []byte { b[20]++; return b }},
		{"keys", func(b []byte) []byte { b[18] = 1; return b }},
		{"checksum", func(b []byte) []byte { b[len(b)-1] ^= 1; return b }},
	} {
		t.Run(change.name, func(t *testing.T) {
			s, written := captureTestSession(nil)
			if err := s.LoadState(change.fn(checkpointTestData())); err == nil {
				t.Fatal("accepted invalid checkpoint")
			}
			if written.Len() != 0 {
				t.Fatal("invalid checkpoint reached helper")
			}
		})
	}
}

func TestCheckpointControlKeepsOnlyFramedRejectionsUsable(t *testing.T) {
	state := checkpointTestData()
	session, _ := captureTestSession(append(controlReply(1, []byte("ROM mismatch")), controlReply(0, nil)...))
	if err := session.LoadState(state); err == nil || !strings.Contains(err.Error(), "ROM mismatch") {
		t.Fatalf("framed rejection = %v", err)
	}
	if err := session.LoadState(state); err != nil {
		t.Fatalf("retry after framed rejection: %v", err)
	}
	session, written := captureTestSession([]byte("invalid data"))
	if err := session.LoadState(state); err == nil {
		t.Fatal("accepted an unframed response")
	}
	before := written.Len()
	if _, err := session.Step(0, false); err == nil || written.Len() != before {
		t.Fatal("step wrote to a desynchronized emulator")
	}
}

// This proves actual serialized guest execution, not a mock implementing the
// same formulas as the lesson: every replay starts from one native mGBA state.
func TestCheckpointReplayIntegration(t *testing.T) {
	helper, rom := os.Getenv("MTILE_TEST_PLAYER"), os.Getenv("MTILE_TEST_ROM")
	if helper == "" || rom == "" {
		t.Skip("set MTILE_TEST_PLAYER and MTILE_TEST_ROM for native checkpoint replay")
	}
	s, err := Start(helper, rom)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for i := 0; i < 30; i++ {
		if _, err := s.Step(0, false); err != nil {
			t.Fatal(err)
		}
	}
	state, err := s.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	// Identical title/code does not establish identical ROM bytes. The helper
	// must enforce its own source-ROM identity even without mGBA's zlib CRC.
	variant, err := os.ReadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	variant[len(variant)-1] ^= 1
	variantPath := t.TempDir() + "/variant.gba"
	if err := os.WriteFile(variantPath, variant, 0600); err != nil {
		t.Fatal(err)
	}
	other, err := Start(helper, variantPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.LoadState(state); err == nil {
		_ = other.Close()
		t.Fatal("loaded checkpoint for different ROM with identical header")
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	wrongROM := bytes.Clone(state)
	wrongROM[8] ^= 1
	binary.LittleEndian.PutUint32(wrongROM[24:28], checkpointCRC(wrongROM))
	if err := s.LoadState(wrongROM); err == nil {
		t.Fatal("loaded checkpoint from a different ROM")
	}
	var baseline []Frame
	for pass := 0; pass < 3; pass++ {
		if err := s.LoadState(state); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 48; i++ {
			keys := uint16(0)
			if i < 24 {
				keys = KeyRight
			}
			if i == 8 || i == 9 {
				keys |= KeyA
			}
			frame, err := s.Step(keys, false)
			if err != nil {
				t.Fatal(err)
			}
			if pass == 0 {
				baseline = append(baseline, frame)
			} else if frame.Number != baseline[i].Number || !bytes.Equal(frame.RGBA, baseline[i].RGBA) || !bytes.Equal(frame.PCM, baseline[i].PCM) {
				t.Fatalf("replay %d frame %d diverged: number=%v pixels=%v audio=%v (%d/%d bytes)", pass, i,
					frame.Number == baseline[i].Number, bytes.Equal(frame.RGBA, baseline[i].RGBA), bytes.Equal(frame.PCM, baseline[i].PCM), len(frame.PCM), len(baseline[i].PCM))
			}
		}
	}
}
