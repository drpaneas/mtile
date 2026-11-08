package emulator

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

const (
	MaxCheckpointSize = 4 * 1024 * 1024
	checkpointHeader  = 1088 + Width*Height*4
	maxControlError   = 1024
)

// SaveState captures the current GBA state without advancing it. The opaque,
// versioned checkpoint includes battery data, RTC, input, ROM identity and the
// helper frame counter.
func (s *Session) SaveState() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.control(commandSaveState, nil, nil, MaxCheckpointSize)
	if err != nil {
		return nil, err
	}
	if err := validateCheckpoint(data); err != nil {
		return nil, fmt.Errorf("emulator returned %w", err)
	}
	return data, nil
}

// LoadState restores a checkpoint for the same ROM and discards frontend audio
// history. It does not advance a frame or write a cartridge's battery-save file.
// Wall-clock RTC checkpoints replay using emulated elapsed time.
func (s *Session) LoadState(data []byte) error {
	if err := validateCheckpoint(data); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(data)))
	reply, err := s.control(commandLoadState, size[:], data, 0)
	if err != nil {
		return err
	}
	if len(reply) != 0 {
		return errors.New("unexpected emulator checkpoint response")
	}
	return nil
}

func checkpointCRC(data []byte) uint32 {
	h := crc32.NewIEEE()
	_, _ = h.Write(data[:24])
	_, _ = h.Write([]byte{0, 0, 0, 0})
	_, _ = h.Write(data[28:])
	return h.Sum32()
}

func validateCheckpoint(data []byte) error {
	if len(data) < checkpointHeader || len(data) > MaxCheckpointSize {
		return fmt.Errorf("checkpoint length %d is outside %d..%d", len(data), checkpointHeader, MaxCheckpointSize)
	}
	if string(data[:4]) != "MTSS" || binary.LittleEndian.Uint32(data[4:8]) != 4 {
		return errors.New("unsupported checkpoint format; capture a new checkpoint with the current emulator")
	}
	if uint64(binary.LittleEndian.Uint32(data[20:24])) != uint64(len(data)-checkpointHeader) || len(data) == checkpointHeader {
		return errors.New("checkpoint payload length mismatch")
	}
	if binary.LittleEndian.Uint32(data[16:20]) > 0x3ff {
		return errors.New("checkpoint has invalid GBA input")
	}
	if binary.LittleEndian.Uint32(data[32:36]) > 8 || binary.LittleEndian.Uint32(data[36:40]) > 2047 || binary.LittleEndian.Uint32(data[40:44]) > 63 || binary.LittleEndian.Uint32(data[52:56]) > 1 {
		return errors.New("checkpoint has invalid audio or presentation state")
	}
	colorBytes := binary.LittleEndian.Uint32(data[1080:1084])
	if (colorBytes != 2 && colorBytes != 4) || binary.LittleEndian.Uint32(data[1084:1088]) != Width*Height*colorBytes {
		return errors.New("checkpoint has invalid framebuffer state")
	}
	if binary.LittleEndian.Uint32(data[24:28]) != checkpointCRC(data) {
		return errors.New("checkpoint checksum mismatch")
	}
	return nil
}

// control is called with s.mu held. Error responses are bounded and consumed in
// full, so a rejected request does not desynchronize subsequent frame reads.
func (s *Session) control(op byte, arguments, payload []byte, maximum int) (result []byte, err error) {
	if s.closed {
		return nil, errors.New("emulator session is closed")
	}
	if s.failure != nil {
		return nil, fmt.Errorf("emulator protocol is unusable: %w", s.failure)
	}
	framed := false
	defer func() {
		if err != nil && !framed {
			s.failure = err
		}
	}()
	command := []byte{op, 0, 0, 0}
	for _, part := range [][]byte{command, arguments, payload} {
		if len(part) == 0 {
			continue
		}
		n, err := s.stdin.Write(part)
		if err == nil && n != len(part) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return nil, s.withLog(fmt.Errorf("send emulator control: %w", err))
		}
	}
	var header [12]byte
	if _, err := io.ReadFull(s.stdout, header[:]); err != nil {
		return nil, s.withLog(fmt.Errorf("read emulator control: %w", err))
	}
	if string(header[:4]) != "DATA" {
		return nil, fmt.Errorf("invalid emulator control marker %q", header[:4])
	}
	status := binary.LittleEndian.Uint32(header[4:8])
	size := binary.LittleEndian.Uint32(header[8:12])
	limit := maximum
	if status != 0 {
		limit = maxControlError
	}
	if uint64(size) > uint64(limit) {
		return nil, fmt.Errorf("emulator control payload %d exceeds limit %d", size, limit)
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(s.stdout, data); err != nil {
		return nil, s.withLog(fmt.Errorf("read emulator control payload: %w", err))
	}
	framed = true
	if status != 0 {
		return nil, fmt.Errorf("emulator control rejected: %s", data)
	}
	return data, nil
}
