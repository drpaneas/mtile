package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func cloneSampleAsset(sample assetsync.SampleAsset) assetsync.SampleAsset {
	if sample.Loop != nil {
		loop := *sample.Loop
		sample.Loop = &loop
	}
	return sample
}

func cloneSampleLibrary(library assetsync.SampleLibrary) assetsync.SampleLibrary {
	library.Samples = slices.Clone(library.Samples)
	for index := range library.Samples {
		library.Samples[index] = cloneSampleAsset(library.Samples[index])
	}
	return library
}

// SampleAsset returns a detached copy so loop edits cannot bypass whole-
// manifest validation.
func (m *Model) SampleAsset(index int) (assetsync.SampleAsset, bool) {
	if m == nil || index < 0 || index >= len(m.Samples.Samples) {
		return assetsync.SampleAsset{}, false
	}
	return cloneSampleAsset(m.Samples.Samples[index]), true
}

// UpdateSample validates the complete manifest and source files before
// committing an in-memory metadata edit.
func (m *Model) UpdateSample(index int, sample assetsync.SampleAsset) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if index < 0 || index >= len(m.Samples.Samples) {
		return fmt.Errorf("sample index %d is outside 0..%d", index, len(m.Samples.Samples)-1)
	}
	candidate := cloneSampleLibrary(m.Samples)
	candidate.Samples[index] = cloneSampleAsset(sample)
	if _, _, err := assetsync.CompileSamples(m.Dir, candidate); err != nil {
		return err
	}
	m.Samples = candidate
	return nil
}

// DeleteSample removes manifest metadata but deliberately preserves the WAV
// source, making delete and undo non-destructive.
func (m *Model) DeleteSample(index int) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if index < 0 || index >= len(m.Samples.Samples) {
		return fmt.Errorf("sample index %d is outside 0..%d", index, len(m.Samples.Samples)-1)
	}
	candidate := cloneSampleLibrary(m.Samples)
	candidate.Samples = slices.Delete(candidate.Samples, index, index+1)
	if _, _, err := assetsync.CompileSamples(m.Dir, candidate); err != nil {
		return err
	}
	m.Samples = candidate
	return nil
}

func nextSampleID(library assetsync.SampleLibrary) int {
	var used uint64
	for _, sample := range library.Samples {
		if sample.ID >= 0 && sample.ID < assetsync.MaxSampleAssets {
			used |= uint64(1) << sample.ID
		}
	}
	for id := 0; id < assetsync.MaxSampleAssets; id++ {
		if used&(uint64(1)<<id) == 0 {
			return id
		}
	}
	return -1
}

func sampleStem(filename string) string {
	stem := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	var builder strings.Builder
	for _, r := range stem {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else if builder.Len() != 0 && !strings.HasSuffix(builder.String(), "-") {
			builder.WriteByte('-')
		}
		if builder.Len() >= 48 {
			break
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func uniqueSampleName(library assetsync.SampleLibrary, stem string, id int) string {
	if stem == "" {
		stem = fmt.Sprintf("sample-%02d", id)
	}
	used := make(map[string]struct{}, len(library.Samples))
	for _, sample := range library.Samples {
		used[sample.Name] = struct{}{}
	}
	for suffix := 1; ; suffix++ {
		candidate := stem
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", stem, suffix)
		}
		if len(candidate) > assetsync.MaxSampleNameLength {
			candidate = candidate[:assetsync.MaxSampleNameLength]
		}
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func uniqueSampleSource(dir, stem string) (string, string, error) {
	if stem == "" {
		stem = "sample"
	}
	for suffix := 1; ; suffix++ {
		name := stem + ".wav"
		if suffix > 1 {
			name = fmt.Sprintf("%s-%d.wav", stem, suffix)
		}
		path := filepath.Join(dir, "assets", "audio", "samples", name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return name, path, nil
		} else if err != nil {
			return "", "", err
		}
	}
}

func writeImportedSample(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sample-import-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ImportSampleWAV copies one dropped WAV into the project and appends valid
// default metadata. The copied file is removed if decoding or conversion
// fails; successful imports retain their source even if later removed from
// the manifest.
func (m *Model) ImportSampleWAV(filename string, data []byte) (int, error) {
	if m == nil {
		return -1, fmt.Errorf("editor model is required")
	}
	if !strings.EqualFold(filepath.Ext(filename), ".wav") {
		return -1, fmt.Errorf("sample import requires a .wav file")
	}
	id := nextSampleID(m.Samples)
	if id < 0 {
		return -1, fmt.Errorf("sample library already contains the maximum %d samples", assetsync.MaxSampleAssets)
	}
	stem := sampleStem(filename)
	source, destination, err := uniqueSampleSource(m.Dir, stem)
	if err != nil {
		return -1, fmt.Errorf("choose sample source path: %w", err)
	}
	if err := writeImportedSample(destination, data); err != nil {
		return -1, fmt.Errorf("copy sample source: %w", err)
	}
	sample := assetsync.SampleAsset{
		ID: id, Name: uniqueSampleName(m.Samples, stem, id), Source: source,
		TargetRate: assetsync.SampleTargetRate, Gain: 256, Priority: 64,
	}
	candidate := cloneSampleLibrary(m.Samples)
	candidate.Samples = append(candidate.Samples, sample)
	if _, _, err := assetsync.CompileSamples(m.Dir, candidate); err != nil {
		_ = os.Remove(destination)
		return -1, err
	}
	m.Samples = candidate
	return len(candidate.Samples) - 1, nil
}
