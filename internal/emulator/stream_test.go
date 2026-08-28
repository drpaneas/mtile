package emulator

import (
	"bytes"
	"testing"
)

func TestPCMStreamOrderAndSilence(t *testing.T) {
	stream := &PCMStream{data: make([]byte, 12)}
	_, _ = stream.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	output := make([]byte, 12)
	if _, err := stream.Read(output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, []byte{1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 0, 0}) {
		t.Fatalf("output = %v", output)
	}
	queued, underruns, overruns := stream.Stats()
	if queued != 0 || underruns != 1 || overruns != 0 {
		t.Fatalf("stats = %d %d %d", queued, underruns, overruns)
	}
}

func TestPCMStreamDropsOldestWholeFrames(t *testing.T) {
	stream := &PCMStream{data: make([]byte, 8)}
	_, _ = stream.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	_, _ = stream.Write([]byte{9, 10, 11, 12})
	output := make([]byte, 8)
	_, _ = stream.Read(output)
	if !bytes.Equal(output, []byte{5, 6, 7, 8, 9, 10, 11, 12}) {
		t.Fatalf("output = %v", output)
	}
}

func TestPCMStreamDiscardDropsQueuedAudio(t *testing.T) {
	stream := &PCMStream{data: make([]byte, 12)}
	_, _ = stream.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	stream.Discard()

	output := make([]byte, 4)
	_, _ = stream.Read(output)
	if !bytes.Equal(output, []byte{0, 0, 0, 0}) {
		t.Fatalf("discarded stream output = %v", output)
	}
	queued, _, _ := stream.Stats()
	if queued != 0 {
		t.Fatalf("discarded stream still has %d frames queued", queued)
	}
}
