package assetsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

const (
	NativeAudioSchemaVersion = 2
	NativeAudioProfile       = "gameboy-psg"
	NativeAudioMaxNameBytes  = 48
)

type NativeAudioSettings struct {
	Version    int    `json:"version"`
	Profile    string `json:"profile"`
	LeftLevel  int    `json:"leftLevel"`
	RightLevel int    `json:"rightLevel"`
	PSGRatio   int    `json:"psgRatio"`
}

type NativeSoundLibrary struct {
	Version int                `json:"version"`
	Sounds  []NativeSoundAsset `json:"sounds"`
}

type NativeSoundAsset struct {
	ID    int               `json:"id"`
	Name  string            `json:"name"`
	Kind  string            `json:"kind"`
	Route string            `json:"route"`
	Pulse *NativePulseAsset `json:"pulse,omitempty"`
	Wave  *NativeWaveAsset  `json:"wave,omitempty"`
	Noise *NativeNoiseAsset `json:"noise,omitempty"`
}

type NativeEnvelopeAsset struct {
	Initial   int    `json:"initial"`
	Direction string `json:"direction"`
	Pace      int    `json:"pace"`
}

type NativeSweepAsset struct {
	Pace      int    `json:"pace"`
	Direction string `json:"direction"`
	Shift     int    `json:"shift"`
}

type NativePulseAsset struct {
	Duty      float64             `json:"duty"`
	Length    int                 `json:"length"`
	UseLength bool                `json:"useLength"`
	Envelope  NativeEnvelopeAsset `json:"envelope"`
	Sweep     *NativeSweepAsset   `json:"sweep,omitempty"`
}

type NativeWaveAsset struct {
	Samples   []int `json:"samples"`
	Level     int   `json:"level"`
	StartBank int   `json:"startBank"`
	Length    int   `json:"length"`
	UseLength bool  `json:"useLength"`
}

type NativeNoiseAsset struct {
	Width     int                 `json:"width"`
	Divisor   int                 `json:"divisor"`
	Shift     int                 `json:"shift"`
	Length    int                 `json:"length"`
	UseLength bool                `json:"useLength"`
	Envelope  NativeEnvelopeAsset `json:"envelope"`
}

type NativeMusicLibrary struct {
	Version int               `json:"version"`
	PPQN    int               `json:"ppqn"`
	Clips   []NativeClipAsset `json:"clips"`
	Parts   []NativePartAsset `json:"parts"`
	Songs   []NativeSongAsset `json:"songs"`
	Takes   []NativeTakeAsset `json:"takes,omitempty"`
}

// NativeTakeAsset is editor-only source timing. The target compiler validates
// it but never emits it into cartridge ROM.
type NativeTakeAsset struct {
	ID            int                   `json:"id"`
	Name          string                `json:"name"`
	PartID        int                   `json:"partId"`
	DurationNanos int64                 `json:"durationNanos"`
	Events        []NativeRawEventAsset `json:"events"`
}

type NativeRawEventAsset struct {
	OffsetNanos int64  `json:"offsetNanos"`
	Kind        string `json:"kind"`
	Channel     int    `json:"channel"`
	Note        int    `json:"note,omitempty"`
	Parameter   string `json:"parameter,omitempty"`
	Value       int    `json:"value,omitempty"`
}

type NativeClipAsset struct {
	ID          int                `json:"id"`
	Name        string             `json:"name"`
	Kind        string             `json:"kind"`
	SFX         bool               `json:"sfx,omitempty"`
	LengthTicks int                `json:"lengthTicks"`
	Events      []NativeEventAsset `json:"events"`
}

type NativeEventAsset struct {
	Tick          int  `json:"tick"`
	DurationTicks int  `json:"durationTicks"`
	Note          int  `json:"note"`
	SoundID       int  `json:"soundId"`
	Priority      int  `json:"priority"`
	Legato        bool `json:"legato,omitempty"`
}

type NativeLaneAssets struct {
	CH1 *int `json:"ch1"`
	CH2 *int `json:"ch2"`
	CH3 *int `json:"ch3"`
	CH4 *int `json:"ch4"`
}

type NativePartAsset struct {
	ID          int              `json:"id"`
	Name        string           `json:"name"`
	LengthTicks int              `json:"lengthTicks"`
	Tempo       int              `json:"tempo,omitempty"`
	Bars        int              `json:"bars,omitempty"`
	BeatsPerBar int              `json:"beatsPerBar,omitempty"`
	BeatUnit    int              `json:"beatUnit,omitempty"`
	Lanes       NativeLaneAssets `json:"lanes"`
}

type NativeSongAsset struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Tempo     int    `json:"tempo"`
	Sequence  []int  `json:"sequence"`
	LoopStart *int   `json:"loopStart"`
}

type NativeAudioAssets struct {
	Settings NativeAudioSettings
	Sounds   NativeSoundLibrary
	Music    NativeMusicLibrary
	Library  audio.Library
}

func DefaultNativeAudioSettings() NativeAudioSettings {
	return NativeAudioSettings{
		Version: NativeAudioSchemaVersion, Profile: NativeAudioProfile,
		LeftLevel: 7, RightLevel: 7, PSGRatio: 100,
	}
}

func DefaultNativeSoundLibrary() NativeSoundLibrary {
	return NativeSoundLibrary{Version: NativeAudioSchemaVersion}
}

func DefaultNativeMusicLibrary() NativeMusicLibrary {
	return NativeMusicLibrary{Version: NativeAudioSchemaVersion, PPQN: audio.NativePPQN}
}

func nativeSoundsPath(projectDir string) string {
	return filepath.Join(projectDir, "assets", "audio", "sounds.json")
}

func nativeMusicPath(projectDir string) string {
	return filepath.Join(projectDir, "assets", "audio", "music.json")
}

func decodeStrictNativeFile(path, label string, target any) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", label, err)
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return true, fmt.Errorf("decode %s: %w", label, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return true, fmt.Errorf("decode %s: trailing data: %w", label, err)
	}
	return true, nil
}

// DetectAudioVersion selects the project-level audio contract before either
// strict decoder runs. Projects with no authored audio start on v2.
func DetectAudioVersion(projectDir string) (int, error) {
	versionFrom := func(path, label string) (int, bool, error) {
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		if err != nil {
			return 0, false, fmt.Errorf("read %s: %w", label, err)
		}
		defer func() { _ = file.Close() }()
		var header struct {
			Version int `json:"version"`
		}
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&header); err != nil {
			return 0, true, fmt.Errorf("decode %s: %w", label, err)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return 0, true, fmt.Errorf("decode %s: trailing data", label)
		}
		return header.Version, true, nil
	}
	if version, present, err := versionFrom(audioSettingsPath(projectDir), "audio settings.json version"); err != nil {
		return 0, err
	} else if present {
		if version != AudioSettingsSchemaVersion && version != NativeAudioSchemaVersion {
			return 0, fmt.Errorf("audio settings version %d is unsupported", version)
		}
		return version, nil
	}
	if _, err := os.Stat(nativeSoundsPath(projectDir)); err == nil {
		return NativeAudioSchemaVersion, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("inspect sounds.json: %w", err)
	}
	if version, present, err := versionFrom(nativeMusicPath(projectDir), "music.json version"); err != nil {
		return 0, err
	} else if present {
		if version != MusicSchemaVersion && version != NativeAudioSchemaVersion {
			return 0, fmt.Errorf("music version %d is unsupported", version)
		}
		return version, nil
	}
	for _, legacy := range []string{
		sfxPath(projectDir), filepath.Join(projectDir, "assets", "audio", "samples", "manifest.json"),
	} {
		if _, err := os.Stat(legacy); err == nil {
			return AudioSettingsSchemaVersion, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("inspect legacy audio: %w", err)
		}
	}
	return NativeAudioSchemaVersion, nil
}

func LoadNativeAudio(projectDir string) (*NativeAudioAssets, error) {
	for _, legacy := range []string{
		filepath.Join(projectDir, "assets", "audio", "sfx.json"),
		filepath.Join(projectDir, "assets", "audio", "samples", "manifest.json"),
	} {
		if _, err := os.Stat(legacy); err == nil {
			return nil, fmt.Errorf("audio v1/PCM asset %s is superseded; convert explicitly to four-channel audio v2", legacy)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect legacy audio asset: %w", err)
		}
	}

	settings := DefaultNativeAudioSettings()
	if _, err := decodeStrictNativeFile(audioSettingsPath(projectDir), "audio settings.json", &settings); err != nil {
		return nil, err
	}
	sounds := DefaultNativeSoundLibrary()
	if _, err := decodeStrictNativeFile(nativeSoundsPath(projectDir), "sounds.json", &sounds); err != nil {
		return nil, err
	}
	music := DefaultNativeMusicLibrary()
	if _, err := decodeStrictNativeFile(nativeMusicPath(projectDir), "music.json", &music); err != nil {
		return nil, err
	}
	library, err := CompileNativeAudio(settings, sounds, music)
	if err != nil {
		return nil, err
	}
	return &NativeAudioAssets{Settings: settings, Sounds: sounds, Music: music, Library: library}, nil
}

func SaveNativeAudioSettings(projectDir string, settings *NativeAudioSettings) error {
	if settings == nil {
		return errors.New("native audio settings are required")
	}
	if err := validateNativeSettings(*settings); err != nil {
		return err
	}
	return writeJSON(audioSettingsPath(projectDir), settings)
}

func SaveNativeSounds(projectDir string, sounds *NativeSoundLibrary) error {
	if sounds == nil {
		return errors.New("native sound library is required")
	}
	if _, err := compileNativeSounds(*sounds); err != nil {
		return err
	}
	return writeJSON(nativeSoundsPath(projectDir), sounds)
}

func SaveNativeMusic(projectDir string, sounds NativeSoundLibrary, music *NativeMusicLibrary) error {
	if music == nil {
		return errors.New("native music library is required")
	}
	compiledSounds, err := compileNativeSounds(sounds)
	if err != nil {
		return err
	}
	if _, _, _, err := compileNativeMusic(*music, compiledSounds); err != nil {
		return err
	}
	return writeJSON(nativeMusicPath(projectDir), music)
}

func CompileNativeAudio(settings NativeAudioSettings, sounds NativeSoundLibrary, music NativeMusicLibrary) (audio.Library, error) {
	if err := validateNativeSettings(settings); err != nil {
		return audio.Library{}, err
	}
	compiledSounds, err := compileNativeSounds(sounds)
	if err != nil {
		return audio.Library{}, err
	}
	clips, parts, songs, err := compileNativeMusic(music, compiledSounds)
	if err != nil {
		return audio.Library{}, err
	}
	library := audio.Library{Sounds: compiledSounds, Clips: clips, Parts: parts, Songs: songs}
	if !audio.ValidateLibrary(library) {
		return audio.Library{}, errors.New("compiled audio v2 library failed runtime invariant validation")
	}
	return library, nil
}

func validateNativeSettings(settings NativeAudioSettings) error {
	if settings.Version != NativeAudioSchemaVersion {
		return fmt.Errorf("audio settings version %d is superseded or unsupported (want %d)", settings.Version, NativeAudioSchemaVersion)
	}
	if settings.Profile != NativeAudioProfile {
		return fmt.Errorf("audio profile %q is unsupported (want %q)", settings.Profile, NativeAudioProfile)
	}
	if settings.LeftLevel < 0 || settings.LeftLevel > 7 || settings.RightLevel < 0 || settings.RightLevel > 7 {
		return fmt.Errorf("PSG terminal levels %d/%d are outside 0..7", settings.LeftLevel, settings.RightLevel)
	}
	if settings.PSGRatio != 25 && settings.PSGRatio != 50 && settings.PSGRatio != 100 {
		return fmt.Errorf("PSG ratio %d is unsupported (want 25, 50, or 100)", settings.PSGRatio)
	}
	return nil
}

func validNativeName(name string) bool {
	if name == "" || len(name) > NativeAudioMaxNameBytes || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func nativeKind(value string) (audio.SoundKind, bool) {
	switch value {
	case "ch1":
		return audio.SoundCH1, true
	case "ch2":
		return audio.SoundCH2, true
	case "ch3":
		return audio.SoundCH3, true
	case "ch4":
		return audio.SoundCH4, true
	default:
		return 0, false
	}
}

func nativeRoute(value string) (hw.SoundRoute, bool) {
	switch value {
	case "none":
		return hw.SoundRouteNone, true
	case "right":
		return hw.SoundRouteRight, true
	case "left":
		return hw.SoundRouteLeft, true
	case "both":
		return hw.SoundRouteBoth, true
	default:
		return 0, false
	}
}

func nativeEnvelope(asset NativeEnvelopeAsset) (hw.HardwareEnvelope, error) {
	direction := hw.EnvelopeDown
	switch asset.Direction {
	case "down":
	case "up":
		direction = hw.EnvelopeUp
	default:
		return hw.HardwareEnvelope{}, fmt.Errorf("envelope direction %q is unsupported", asset.Direction)
	}
	if asset.Initial < 0 || asset.Initial > 15 || asset.Pace < 0 || asset.Pace > 7 {
		return hw.HardwareEnvelope{}, fmt.Errorf("envelope initial/pace %d/%d is outside 0..15/0..7", asset.Initial, asset.Pace)
	}
	return hw.HardwareEnvelope{Initial: uint8(asset.Initial), Direction: direction, Pace: uint8(asset.Pace)}, nil
}

func nativeDuty(value float64) (hw.PulseDuty, bool) {
	switch value {
	case 12.5:
		return hw.PulseDuty12_5, true
	case 25:
		return hw.PulseDuty25, true
	case 50:
		return hw.PulseDuty50, true
	case 75:
		return hw.PulseDuty75, true
	default:
		return 0, false
	}
}

func compileNativePulse(asset *NativePulseAsset, route hw.SoundRoute, sweepRequired bool) (hw.PulseConfig, hw.HardwareSweep, error) {
	if asset == nil {
		return hw.PulseConfig{}, hw.HardwareSweep{}, errors.New("pulse payload is required")
	}
	duty, ok := nativeDuty(asset.Duty)
	if !ok || asset.Length < 0 || asset.Length > 63 {
		return hw.PulseConfig{}, hw.HardwareSweep{}, fmt.Errorf("pulse duty/length %g/%d is outside hardware values", asset.Duty, asset.Length)
	}
	envelope, err := nativeEnvelope(asset.Envelope)
	if err != nil {
		return hw.PulseConfig{}, hw.HardwareSweep{}, err
	}
	pulse := hw.PulseConfig{Duty: duty, Length: uint8(asset.Length), UseLength: asset.UseLength, Envelope: envelope, Route: route}
	if !sweepRequired {
		if asset.Sweep != nil {
			return hw.PulseConfig{}, hw.HardwareSweep{}, errors.New("CH2 forbids sweep")
		}
		return pulse, hw.HardwareSweep{}, nil
	}
	if asset.Sweep == nil {
		return hw.PulseConfig{}, hw.HardwareSweep{}, errors.New("CH1 requires sweep")
	}
	sweepDirection := hw.SweepUp
	switch asset.Sweep.Direction {
	case "up":
	case "down":
		sweepDirection = hw.SweepDown
	default:
		return hw.PulseConfig{}, hw.HardwareSweep{}, fmt.Errorf("sweep direction %q is unsupported", asset.Sweep.Direction)
	}
	if asset.Sweep.Pace < 0 || asset.Sweep.Pace > 7 || asset.Sweep.Shift < 0 || asset.Sweep.Shift > 7 {
		return hw.PulseConfig{}, hw.HardwareSweep{}, fmt.Errorf("sweep pace/shift %d/%d is outside 0..7", asset.Sweep.Pace, asset.Sweep.Shift)
	}
	return pulse, hw.HardwareSweep{Pace: uint8(asset.Sweep.Pace), Direction: sweepDirection, Shift: uint8(asset.Sweep.Shift)}, nil
}

func compileNativeSounds(library NativeSoundLibrary) ([]audio.Sound, error) {
	if library.Version != NativeAudioSchemaVersion {
		return nil, fmt.Errorf("sounds version %d is superseded or unsupported (want %d)", library.Version, NativeAudioSchemaVersion)
	}
	if len(library.Sounds) > audio.NativeMaxSounds {
		return nil, fmt.Errorf("sounds has %d entries (maximum %d)", len(library.Sounds), audio.NativeMaxSounds)
	}
	compiled := make([]audio.Sound, 0, len(library.Sounds))
	seen := [audio.NativeMaxSounds]bool{}
	for index, asset := range library.Sounds {
		if asset.ID < 0 || asset.ID >= audio.NativeMaxSounds || seen[asset.ID] {
			return nil, fmt.Errorf("sound %d has invalid or duplicate id %d", index, asset.ID)
		}
		seen[asset.ID] = true
		if !validNativeName(asset.Name) {
			return nil, fmt.Errorf("sound %d name must be 1..%d UTF-8 bytes without controls or edge whitespace", asset.ID, NativeAudioMaxNameBytes)
		}
		kind, ok := nativeKind(asset.Kind)
		if !ok {
			return nil, fmt.Errorf("sound %d kind %q is unsupported", asset.ID, asset.Kind)
		}
		route, ok := nativeRoute(asset.Route)
		if !ok {
			return nil, fmt.Errorf("sound %d route %q is unsupported", asset.ID, asset.Route)
		}
		payloadCount := 0
		if asset.Pulse != nil {
			payloadCount++
		}
		if asset.Wave != nil {
			payloadCount++
		}
		if asset.Noise != nil {
			payloadCount++
		}
		if payloadCount != 1 {
			return nil, fmt.Errorf("sound %d must contain exactly one channel payload", asset.ID)
		}
		sound := audio.Sound{ID: uint8(asset.ID), Kind: kind}
		switch kind {
		case audio.SoundCH1:
			if asset.Wave != nil || asset.Noise != nil {
				return nil, fmt.Errorf("sound %d CH1 requires only pulse", asset.ID)
			}
			pulse, sweep, err := compileNativePulse(asset.Pulse, route, true)
			if err != nil {
				return nil, fmt.Errorf("sound %d: %w", asset.ID, err)
			}
			sound.Pulse1 = hw.Pulse1Config{Pulse: pulse, Sweep: sweep}
		case audio.SoundCH2:
			if asset.Wave != nil || asset.Noise != nil {
				return nil, fmt.Errorf("sound %d CH2 requires only pulse", asset.ID)
			}
			pulse, _, err := compileNativePulse(asset.Pulse, route, false)
			if err != nil {
				return nil, fmt.Errorf("sound %d: %w", asset.ID, err)
			}
			sound.Pulse2 = pulse
		case audio.SoundCH3:
			if asset.Pulse != nil || asset.Noise != nil || asset.Wave == nil {
				return nil, fmt.Errorf("sound %d CH3 requires only wave", asset.ID)
			}
			wave := asset.Wave
			if len(wave.Samples) != 32 && len(wave.Samples) != 64 || wave.StartBank < 0 || wave.StartBank > 1 || wave.Length < 0 || wave.Length > 255 {
				return nil, fmt.Errorf("sound %d has invalid CH3 length/bank/wave size", asset.ID)
			}
			var level hw.WaveLevel
			switch wave.Level {
			case 0:
				level = hw.WaveMute
			case 25:
				level = hw.Wave25
			case 50:
				level = hw.Wave50
			case 75:
				level = hw.Wave75
			case 100:
				level = hw.Wave100
			default:
				return nil, fmt.Errorf("sound %d CH3 level %d is unsupported", asset.ID, wave.Level)
			}
			var waveform hw.Waveform
			waveform.Length = uint8(len(wave.Samples))
			for sampleIndex, sample := range wave.Samples {
				if sample < 0 || sample > 15 {
					return nil, fmt.Errorf("sound %d wave sample %d is outside 0..15", asset.ID, sampleIndex)
				}
				waveform.Samples[sampleIndex] = uint8(sample)
			}
			sound.Wave = hw.WaveConfig{Wave: waveform, Level: level, StartBank: uint8(wave.StartBank), Length: uint8(wave.Length), UseLength: wave.UseLength, Route: route}
		case audio.SoundCH4:
			if asset.Pulse != nil || asset.Wave != nil || asset.Noise == nil {
				return nil, fmt.Errorf("sound %d CH4 requires only noise", asset.ID)
			}
			noise := asset.Noise
			if noise.Width != 7 && noise.Width != 15 || noise.Divisor < 0 || noise.Divisor > 7 || noise.Shift < 0 || noise.Shift > 13 || noise.Length < 0 || noise.Length > 63 {
				return nil, fmt.Errorf("sound %d has invalid CH4 width/divisor/shift/length", asset.ID)
			}
			envelope, err := nativeEnvelope(noise.Envelope)
			if err != nil {
				return nil, fmt.Errorf("sound %d: %w", asset.ID, err)
			}
			width := hw.Noise15Bit
			if noise.Width == 7 {
				width = hw.Noise7Bit
			}
			sound.Noise = hw.NoiseConfig{Width: width, Divisor: uint8(noise.Divisor), Shift: uint8(noise.Shift), Length: uint8(noise.Length), UseLength: noise.UseLength, Envelope: envelope, Route: route}
		}
		compiled = append(compiled, sound)
	}
	return compiled, nil
}

func compiledSoundByID(sounds []audio.Sound, id int) *audio.Sound {
	if id < 0 || id > 255 {
		return nil
	}
	for index := range sounds {
		if int(sounds[index].ID) == id {
			return &sounds[index]
		}
	}
	return nil
}

func compileNativeMusic(library NativeMusicLibrary, sounds []audio.Sound) ([]audio.Clip, []audio.Part, []audio.Song, error) {
	if library.Version != NativeAudioSchemaVersion || library.PPQN != audio.NativePPQN {
		return nil, nil, nil, fmt.Errorf("music version/ppqn %d/%d is unsupported (want %d/%d)", library.Version, library.PPQN, NativeAudioSchemaVersion, audio.NativePPQN)
	}
	if len(library.Clips) > audio.NativeMaxClips || len(library.Parts) > audio.NativeMaxParts || len(library.Songs) > audio.NativeMaxSongs {
		return nil, nil, nil, errors.New("music exceeds v2 clip/part/song limits")
	}
	clips := make([]audio.Clip, 0, len(library.Clips))
	clipIDs := [audio.NativeMaxClips]bool{}
	standaloneSFX := [audio.NativeMaxClips]bool{}
	eventCount := 0
	for index, asset := range library.Clips {
		if asset.ID < 0 || asset.ID >= audio.NativeMaxClips || clipIDs[asset.ID] {
			return nil, nil, nil, fmt.Errorf("clip %d has invalid or duplicate id %d", index, asset.ID)
		}
		clipIDs[asset.ID] = true
		standaloneSFX[asset.ID] = asset.SFX
		if !validNativeName(asset.Name) || asset.LengthTicks < 1 {
			return nil, nil, nil, fmt.Errorf("clip %d has invalid name or length", asset.ID)
		}
		kind, ok := nativeKind(asset.Kind)
		if !ok {
			return nil, nil, nil, fmt.Errorf("clip %d kind %q is unsupported", asset.ID, asset.Kind)
		}
		clip := audio.Clip{ID: uint8(asset.ID), Kind: kind, LengthTicks: uint32(asset.LengthTicks), NoteEvents: make([]audio.NoteEvent, 0, len(asset.Events))}
		previousEnd := 0
		for eventIndex, event := range asset.Events {
			if event.Tick < 0 || event.DurationTicks < 1 || event.DurationTicks > 65535 || event.Tick+event.DurationTicks > asset.LengthTicks || eventIndex > 0 && event.Tick < previousEnd {
				return nil, nil, nil, fmt.Errorf("clip %d event %d has invalid/overlapping timing", asset.ID, eventIndex)
			}
			sound := compiledSoundByID(sounds, event.SoundID)
			if sound == nil || sound.Kind != kind {
				return nil, nil, nil, fmt.Errorf("clip %d event %d sound %d does not match %s", asset.ID, eventIndex, event.SoundID, asset.Kind)
			}
			if event.Note < 0 || event.Note > 127 || event.Priority < 0 || event.Priority > 255 {
				return nil, nil, nil, fmt.Errorf("clip %d event %d note/priority is outside 0..127/0..255", asset.ID, eventIndex)
			}
			if kind != audio.SoundCH4 {
				if _, ok := audio.NativeSoundPeriod(*sound, uint8(event.Note)); !ok {
					return nil, nil, nil, fmt.Errorf("clip %d event %d note %d is outside %s hardware range", asset.ID, eventIndex, event.Note, asset.Kind)
				}
			} else if event.Legato {
				return nil, nil, nil, fmt.Errorf("clip %d event %d requests legato on CH4 noise", asset.ID, eventIndex)
			}
			clip.NoteEvents = append(clip.NoteEvents, audio.NoteEvent{Tick: uint32(event.Tick), DurationTicks: uint16(event.DurationTicks), Note: uint8(event.Note), SoundID: uint8(event.SoundID), Priority: uint8(event.Priority), Legato: event.Legato})
			previousEnd = event.Tick + event.DurationTicks
		}
		eventCount += len(asset.Events)
		if eventCount > audio.NativeMaxEvents {
			return nil, nil, nil, fmt.Errorf("music has more than %d events", audio.NativeMaxEvents)
		}
		clips = append(clips, clip)
	}

	parts := make([]audio.Part, 0, len(library.Parts))
	partIDs := [audio.NativeMaxParts]bool{}
	for index, asset := range library.Parts {
		if asset.ID < 0 || asset.ID >= audio.NativeMaxParts || partIDs[asset.ID] {
			return nil, nil, nil, fmt.Errorf("part %d has invalid or duplicate id %d", index, asset.ID)
		}
		partIDs[asset.ID] = true
		if !validNativeName(asset.Name) || asset.LengthTicks < 1 || asset.Tempo != 0 && (asset.Tempo < 20 || asset.Tempo > 400) {
			return nil, nil, nil, fmt.Errorf("part %d has invalid name or length", asset.ID)
		}
		if asset.Bars != 0 || asset.BeatsPerBar != 0 || asset.BeatUnit != 0 {
			validUnit := asset.BeatUnit == 1 || asset.BeatUnit == 2 || asset.BeatUnit == 4 || asset.BeatUnit == 8 || asset.BeatUnit == 16 || asset.BeatUnit == 32
			if asset.Bars < 1 || asset.Bars > 16 || asset.BeatsPerBar < 1 || asset.BeatsPerBar > 16 || !validUnit {
				return nil, nil, nil, fmt.Errorf("part %d has invalid bars/time signature", asset.ID)
			}
			ticksPerBar := audio.NativePPQN * 4 * asset.BeatsPerBar / asset.BeatUnit
			if ticksPerBar < 1 || ticksPerBar*asset.Bars != asset.LengthTicks {
				return nil, nil, nil, fmt.Errorf("part %d length does not match %d bars of %d/%d", asset.ID, asset.Bars, asset.BeatsPerBar, asset.BeatUnit)
			}
		}
		part := audio.Part{ID: uint8(asset.ID), LengthTicks: uint32(asset.LengthTicks), Tempo: uint16(asset.Tempo), PSGClips: [4]int16{-1, -1, -1, -1}}
		lanes := [4]*int{asset.Lanes.CH1, asset.Lanes.CH2, asset.Lanes.CH3, asset.Lanes.CH4}
		for channel, clipID := range lanes {
			if clipID == nil {
				continue
			}
			if *clipID < 0 || *clipID > 255 {
				return nil, nil, nil, fmt.Errorf("part %d lane CH%d clip id %d is invalid", asset.ID, channel+1, *clipID)
			}
			var found *audio.Clip
			for clipIndex := range clips {
				if int(clips[clipIndex].ID) == *clipID {
					found = &clips[clipIndex]
					break
				}
			}
			if found == nil || standaloneSFX[*clipID] || int(found.Kind) != channel || found.LengthTicks > uint32(asset.LengthTicks) {
				return nil, nil, nil, fmt.Errorf("part %d lane CH%d has missing, wrong-kind, or oversized clip %d", asset.ID, channel+1, *clipID)
			}
			part.PSGClips[channel] = int16(*clipID)
		}
		parts = append(parts, part)
	}
	if err := validateNativeTakes(library.Takes, partIDs); err != nil {
		return nil, nil, nil, err
	}

	songs := make([]audio.Song, 0, len(library.Songs))
	songIDs := [audio.NativeMaxSongs]bool{}
	for index, asset := range library.Songs {
		if asset.ID < 0 || asset.ID >= audio.NativeMaxSongs || songIDs[asset.ID] {
			return nil, nil, nil, fmt.Errorf("song %d has invalid or duplicate id %d", index, asset.ID)
		}
		songIDs[asset.ID] = true
		if !validNativeName(asset.Name) || asset.Tempo < 20 || asset.Tempo > 400 || len(asset.Sequence) == 0 || len(asset.Sequence) > audio.NativeMaxSequenceParts {
			return nil, nil, nil, fmt.Errorf("song %d has invalid name, tempo, or sequence length", asset.ID)
		}
		song := audio.Song{ID: uint8(asset.ID), Tempo: uint16(asset.Tempo), Sequence: make([]uint8, len(asset.Sequence)), LoopStart: -1}
		for sequenceIndex, partID := range asset.Sequence {
			if partID < 0 || partID >= audio.NativeMaxParts || !partIDs[partID] {
				return nil, nil, nil, fmt.Errorf("song %d sequence %d references missing part %d", asset.ID, sequenceIndex, partID)
			}
			song.Sequence[sequenceIndex] = uint8(partID)
		}
		if asset.LoopStart != nil {
			if *asset.LoopStart < 0 || *asset.LoopStart >= len(asset.Sequence) {
				return nil, nil, nil, fmt.Errorf("song %d loopStart %d is outside sequence", asset.ID, *asset.LoopStart)
			}
			song.LoopStart = int16(*asset.LoopStart)
		}
		songs = append(songs, song)
	}
	return clips, parts, songs, nil
}

func validateNativeTakes(takes []NativeTakeAsset, partIDs [audio.NativeMaxParts]bool) error {
	if len(takes) > audio.NativeMaxParts {
		return fmt.Errorf("music has %d raw takes (maximum %d)", len(takes), audio.NativeMaxParts)
	}
	ids := [audio.NativeMaxParts]bool{}
	totalEvents := 0
	validParameter := map[string]bool{
		"duty": true, "volume": true, "envelope-pace": true, "envelope-direction": true,
		"sweep-pace": true, "sweep-direction": true, "sweep-shift": true,
		"wave-level": true, "noise-width": true, "noise-divisor": true, "noise-shift": true,
	}
	for index, take := range takes {
		if take.ID < 0 || take.ID >= len(ids) || ids[take.ID] {
			return fmt.Errorf("raw take %d has invalid or duplicate id %d", index, take.ID)
		}
		ids[take.ID] = true
		if !validNativeName(take.Name) || take.PartID < 0 || take.PartID >= len(partIDs) || !partIDs[take.PartID] || take.DurationNanos < 0 {
			return fmt.Errorf("raw take %d has invalid name, part, or duration", take.ID)
		}
		for eventIndex, event := range take.Events {
			if event.OffsetNanos < 0 || event.OffsetNanos > take.DurationNanos || event.Channel < 1 || event.Channel > 4 {
				return fmt.Errorf("raw take %d event %d has invalid time or channel", take.ID, eventIndex)
			}
			switch event.Kind {
			case "start", "stop":
				if event.Note < 0 || event.Note > 127 || event.Parameter != "" {
					return fmt.Errorf("raw take %d event %d has invalid note fields", take.ID, eventIndex)
				}
			case "parameter":
				if !validParameter[event.Parameter] {
					return fmt.Errorf("raw take %d event %d parameter %q is unsupported", take.ID, eventIndex, event.Parameter)
				}
			default:
				return fmt.Errorf("raw take %d event %d kind %q is unsupported", take.ID, eventIndex, event.Kind)
			}
		}
		totalEvents += len(take.Events)
		if totalEvents > audio.NativeMaxEvents {
			return fmt.Errorf("raw takes have more than %d events", audio.NativeMaxEvents)
		}
	}
	return nil
}
