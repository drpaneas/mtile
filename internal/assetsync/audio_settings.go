package assetsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	AudioSettingsSchemaVersion = 1
	AudioOutputMono            = "mono"
	AudioVoiceProfileAuto      = "auto"
	AudioMasterVolumeMax       = 256
)

// AudioSettings is the versioned assets/audio/settings.json document. Output,
// rate, and voice profile are persisted so unsupported combinations fail
// explicitly instead of being silently ignored; masterVolume is the one
// currently editable production setting.
type AudioSettings struct {
	Version      int    `json:"version"`
	Output       string `json:"output"`
	SampleRate   int    `json:"sampleRate"`
	VoiceProfile string `json:"voiceProfile"`
	MasterVolume int    `json:"masterVolume"`
}

func DefaultAudioSettings() AudioSettings {
	return AudioSettings{
		Version: AudioSettingsSchemaVersion, Output: AudioOutputMono,
		SampleRate: SampleTargetRate, VoiceProfile: AudioVoiceProfileAuto,
		MasterVolume: AudioMasterVolumeMax,
	}
}

func audioSettingsPath(projectDir string) string {
	return filepath.Join(projectDir, "assets", "audio", "settings.json")
}

func ValidateAudioSettings(settings AudioSettings) error {
	if settings.Version != AudioSettingsSchemaVersion {
		return fmt.Errorf("audio settings version %d is unsupported (want %d)", settings.Version, AudioSettingsSchemaVersion)
	}
	if settings.Output != AudioOutputMono {
		return fmt.Errorf("audio output %q is unsupported (want %q)", settings.Output, AudioOutputMono)
	}
	if settings.SampleRate != SampleTargetRate {
		return fmt.Errorf("audio sample rate %d is unsupported (want %d)", settings.SampleRate, SampleTargetRate)
	}
	if settings.VoiceProfile != AudioVoiceProfileAuto {
		return fmt.Errorf("audio voice profile %q is unsupported (want %q)", settings.VoiceProfile, AudioVoiceProfileAuto)
	}
	if settings.MasterVolume < 0 || settings.MasterVolume > AudioMasterVolumeMax {
		return fmt.Errorf("audio master volume %d is outside 0..%d", settings.MasterVolume, AudioMasterVolumeMax)
	}
	return nil
}

// LoadAudioSettings preserves legacy projects by supplying the only measured
// profile at unity when settings.json is absent. Present files are strict and
// versioned like the other authored audio documents.
func LoadAudioSettings(projectDir string) (*AudioSettings, error) {
	file, err := os.Open(audioSettingsPath(projectDir))
	if errors.Is(err, os.ErrNotExist) {
		settings := DefaultAudioSettings()
		return &settings, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read audio settings: %w", err)
	}
	defer func() { _ = file.Close() }()

	var settings AudioSettings
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return nil, fmt.Errorf("decode audio settings: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("decode audio settings: trailing data: %w", err)
	}
	if err := ValidateAudioSettings(settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func SaveAudioSettings(projectDir string, settings *AudioSettings) error {
	if settings == nil {
		return errors.New("audio settings are required")
	}
	if err := ValidateAudioSettings(*settings); err != nil {
		return err
	}
	return writeJSON(audioSettingsPath(projectDir), settings)
}
