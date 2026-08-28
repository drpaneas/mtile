//go:build ignore

// Command generate_samples recreates the deterministic PCM WAV inputs used by
// the native sample example.
package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

const sampleRate = 16384

func writeWAV(path string, samples []int16) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	dataBytes := uint32(len(samples) * 2)
	if _, err := file.Write([]byte("RIFF")); err != nil {
		return err
	}
	for _, value := range []any{
		uint32(36) + dataBytes, []byte("WAVEfmt "), uint32(16), uint16(1), uint16(1),
		uint32(sampleRate), uint32(sampleRate * 2), uint16(2), uint16(16), []byte("data"), dataBytes,
	} {
		if err := binary.Write(file, binary.LittleEndian, value); err != nil {
			return err
		}
	}
	return binary.Write(file, binary.LittleEndian, samples)
}

func main() {
	root := filepath.Join("assets", "audio", "samples")
	impact := make([]int16, 4098)
	for index := 1; index < len(impact)-1; index++ {
		envelope := float64(len(impact)-1-index) / float64(len(impact)-2)
		phase := 2 * math.Pi * (180*float64(index)/sampleRate + 620*float64(index*index)/float64(2*sampleRate*len(impact)))
		impact[index] = int16(math.Sin(phase) * envelope * envelope * 28000)
	}
	engine := make([]int16, 4096)
	for index := range engine {
		phase := 2 * math.Pi * 64 * float64(index) / sampleRate
		engine[index] = int16((math.Sin(phase) + 0.35*math.Sin(phase*3)) * 18000)
	}
	if err := writeWAV(filepath.Join(root, "impact.wav"), impact); err != nil {
		panic(err)
	}
	if err := writeWAV(filepath.Join(root, "engine.wav"), engine); err != nil {
		panic(err)
	}
}
