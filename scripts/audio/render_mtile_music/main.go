// Command render_mtile_music exports a project's target-rate music preview.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

func writeWAV(path string, samples []int8) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	dataBytes := uint32(len(samples) * 2)
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataBytes)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], editor.SFXPreviewSampleRate)
	binary.LittleEndian.PutUint32(header[28:32], editor.SFXPreviewSampleRate*2)
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataBytes)
	if _, err := file.Write(header); err != nil {
		return err
	}
	pcm := make([]byte, len(samples)*2)
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(pcm[index*2:], uint16(int16(sample)<<8))
	}
	if _, err := file.Write(pcm); err != nil {
		return err
	}
	return file.Sync()
}

func main() {
	entry := flag.Int("entry-pattern", 0, "music pattern ID to start")
	sfxID := flag.Int("sfx-id", -1, "render one SFX ID instead of music (-1 disables)")
	muteMask := flag.Int("mute-mask", 0, "host channel mute mask (bits 0..3)")
	seconds := flag.Int("max-seconds", 30*60, "maximum render length")
	sampleLimit := flag.Int("max-samples", 0, "exact maximum render samples (overrides max-seconds)")
	packed := flag.Bool("packed", false, "render through the GBA FIFO word path")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: render_mtile_music [options] PROJECT OUTPUT.wav")
		os.Exit(2)
	}
	project, output := flag.Arg(0), flag.Arg(1)
	sfx, err := assetsync.LoadSFX(project)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	music, err := assetsync.LoadMusic(project, *sfx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *seconds <= 0 || *seconds > 30*60 {
		fmt.Fprintln(os.Stderr, "max-seconds must be in 1..1800")
		os.Exit(2)
	}
	if *sfxID < -1 || *sfxID > 63 {
		fmt.Fprintln(os.Stderr, "sfx-id must be in -1..63")
		os.Exit(2)
	}
	maxSamples := *seconds * editor.SFXPreviewSampleRate
	if *muteMask < 0 || *muteMask > 15 {
		fmt.Fprintln(os.Stderr, "mute-mask must be in 0..15")
		os.Exit(2)
	}
	if *sampleLimit != 0 {
		if *sampleLimit < 0 || *sampleLimit > 30*60*editor.SFXPreviewSampleRate {
			fmt.Fprintln(os.Stderr, "max-samples must be in 1..29491200")
			os.Exit(2)
		}
		maxSamples = *sampleLimit
	}
	var samples []int8
	if *sfxID >= 0 {
		preview, previewErr := editor.RenderSFXPreview(*sfx, *sfxID, 0, 0)
		if previewErr != nil {
			fmt.Fprintln(os.Stderr, previewErr)
			os.Exit(1)
		}
		samples = preview.Samples
		if len(samples) > maxSamples {
			samples = samples[:maxSamples]
		}
	} else {
		render := editor.RenderMusicPreviewLimit
		if *packed {
			render = editor.RenderMusicPreviewWordsLimit
		}
		preview, previewErr := render(*sfx, *music, *entry, uint8(*muteMask), maxSamples)
		if previewErr != nil {
			fmt.Fprintln(os.Stderr, previewErr)
			os.Exit(1)
		}
		samples = preview.Samples
	}
	if err := writeWAV(output, samples); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("rendered %.6f seconds to %s\n", float64(len(samples))/editor.SFXPreviewSampleRate, output)
}
