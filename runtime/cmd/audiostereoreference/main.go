// Command audiostereoreference renders the host reference used to verify the
// mGBA internal stereo-routing capture.
package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/internal/stereocapture"
)

const (
	callbackRate = 32768
	callbackGain = 192
)

func writeHeader(writer io.Writer, frames uint32) error {
	dataBytes := frames * 4
	for _, value := range []any{
		[]byte("RIFF"), uint32(36 + dataBytes), []byte("WAVEfmt "), uint32(16),
		uint16(1), uint16(2), uint32(callbackRate), uint32(callbackRate * 4),
		uint16(4), uint16(16), []byte("data"), dataBytes,
	} {
		if err := binary.Write(writer, binary.LittleEndian, value); err != nil {
			return err
		}
	}
	return nil
}

func writeReference(path string, frames int) error {
	if frames <= 0 || frames > (1<<30-44)/4 {
		return fmt.Errorf("invalid callback frame count %d", frames)
	}
	mixerSamples := (frames + 1) / 2
	left := make([]int8, mixerSamples)
	right := make([]int8, mixerSamples)
	var mixer audio.Mixer
	if !stereocapture.Start(&mixer) {
		return fmt.Errorf("stereo capture profile initialization failed")
	}
	mixer.MixStereo(left, right)

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	failed := true
	defer func() {
		if failed {
			_ = file.Close()
		}
	}()
	if err := writeHeader(writer, uint32(frames)); err != nil {
		return err
	}

	var encoded [4]byte
	for frame := 0; frame < frames; frame++ {
		index := frame / 2
		leftSample := int16(int32(left[index]) * callbackGain)
		rightSample := int16(int32(right[index]) * callbackGain)
		binary.LittleEndian.PutUint16(encoded[0:2], uint16(leftSample))
		binary.LittleEndian.PutUint16(encoded[2:4], uint16(rightSample))
		if _, err := writer.Write(encoded[:]); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	failed = false
	return nil
}

func main() {
	output := flag.String("output", "audiostereocapture-reference.wav", "output stereo WAV")
	frames := flag.Int("frames", callbackRate*2, "32,768 Hz callback frames to render")
	flag.Parse()
	if err := writeReference(*output, *frames); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
