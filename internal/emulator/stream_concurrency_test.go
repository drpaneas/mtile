package emulator

import (
	"encoding/binary"
	"runtime"
	"testing"
)

func TestPCMStreamConcurrentFramesStayWholeAndOrdered(t *testing.T) {
	const frames = 8192
	stream := &PCMStream{data: make([]byte, 7*4)}
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-start
		var frame [4]byte
		for sequence := uint32(1); sequence <= frames; sequence++ {
			// Redundant halves reveal a torn frame even when its sequence
			// happens to remain within the expected increasing range.
			binary.LittleEndian.PutUint32(frame[:], sequence<<16|sequence^0xffff)
			_, _ = stream.Write(frame[:])
			if sequence%17 == 0 {
				runtime.Gosched()
			}
		}
	}()

	var last uint32
	var output [9 * 4]byte
	read := func() {
		t.Helper()
		if n, err := stream.Read(output[:]); n != len(output) || err != nil {
			t.Fatalf("read = %d, %v", n, err)
		}
		silence := false
		for offset := 0; offset < len(output); offset += 4 {
			value := binary.LittleEndian.Uint32(output[offset:])
			if value == 0 {
				silence = true
				continue
			}
			sequence := value >> 16
			if silence || value&0xffff != sequence^0xffff || sequence <= last || sequence > frames {
				t.Fatalf("frame %#08x after sequence %d, silence=%v", value, last, silence)
			}
			last = sequence // Gaps are allowed: overruns drop whole oldest frames.
		}
	}

	close(start)
	for i := 0; i < frames; i++ {
		read()
		runtime.Gosched()
	}
	<-done
	read() // Larger than the ring: drain any frames produced after the last read.
	if queued, _, _ := stream.Stats(); queued != 0 || last != frames {
		t.Fatalf("final queue = %d, last sequence = %d; want 0, %d", queued, last, frames)
	}
}
