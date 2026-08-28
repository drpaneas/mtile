package editor

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/runtime/audio"
)

const (
	AudioOutputMode      = "mono"
	AudioMasterVolume    = assetsync.AudioMasterVolumeMax
	AudioMonoStreamEWRAM = 1936
	// Current TinyGo target-map sizes after the PICO-8 oscillator/filter
	// state and append-only per-channel reverb histories were added.
	AudioPlayerEWRAM         = 19376
	AudioMixerIWRAM          = 1328
	AudioBlockCycles         = 262144
	AudioFrame30Cycles       = 561792
	AudioStandardMixCycles   = 87540
	AudioSampleMixCycles     = 110081
	AudioMaxProducerCycles   = 251989
	AudioStandardEmptyTick   = 440135
	AudioSampleEmptyTick     = 558127
	AudioMatrixCases         = 7
	AudioStandardPassCases   = 3
	AudioSamplePassCases     = 2
	AudioSampleSafePassCases = 5
	AudioStereoMixCycles     = 78168
	AudioStereoStreamEWRAM   = 3264
	AudioBenchmarkUnderruns  = 0
)

// AudioReport is a host-side, target-layout report for the current in-memory
// project. Benchmark fields are checked-in ARM7TDMI measurements, not live
// telemetry from a running cartridge.
type AudioReport struct {
	Enabled          bool
	Output           string
	SampleRate       int
	VoiceProfile     string
	VoiceCount       int
	MasterVolume     int
	AssetROMBytes    int
	StreamEWRAMBytes int
	PlayerEWRAMBytes int
	MixerIWRAMBytes  int
	SFXCount         int
	StepCount        int
	PatternCount     int
	SampleCount      int
	SamplePCMBytes   int
	MixWorkload      string
	MixCycles        int
	MixLoadTenths    int
	ProducerMax      int
	ProducerTenths   int
	CoexistEmpty     int
	CoexistTenths    int
	CoexistPassCases int
	CoexistCaseCount int
	BenchUnderruns   int
}

func audioLoadTenths(cycles int) int {
	return (cycles*1000 + AudioBlockCycles/2) / AudioBlockCycles
}

func audioFrameLoadTenths(cycles int) int {
	return (cycles*1000 + AudioFrame30Cycles/2) / AudioFrame30Cycles
}

// AudioReport compiles all authored audio through the same validation and
// packing paths as a build, then combines the exact project footprint with the
// current measured production baseline.
func (m *Model) AudioReport() (*AudioReport, error) {
	if m == nil {
		return nil, fmt.Errorf("editor model is required")
	}
	if err := assetsync.ValidateAudioSettings(m.AudioSettings); err != nil {
		return nil, err
	}
	sfx, steps, err := assetsync.CompileSFX(m.SFX)
	if err != nil {
		return nil, err
	}
	music, err := assetsync.CompileMusic(m.Music, m.SFX)
	if err != nil {
		return nil, err
	}
	samples, pcm, err := assetsync.CompileSamples(m.Dir, m.Samples)
	if err != nil {
		return nil, err
	}
	enabled := len(sfx) != 0 || len(music) != 0 || len(samples) != 0
	report := &AudioReport{
		Enabled: enabled, Output: AudioOutputMode, SampleRate: assetsync.SampleTargetRate,
		VoiceProfile: "inactive", MasterVolume: m.AudioSettings.MasterVolume,
		AssetROMBytes: assetsync.CompiledAudioROMBytes(sfx, steps, music, samples, pcm),
		SFXCount:      len(sfx), StepCount: len(steps), PatternCount: len(music),
		SampleCount: len(samples), SamplePCMBytes: len(pcm),
		CoexistCaseCount: AudioMatrixCases, BenchUnderruns: AudioBenchmarkUnderruns,
	}
	if !enabled {
		return report, nil
	}
	report.StreamEWRAMBytes = AudioMonoStreamEWRAM
	report.PlayerEWRAMBytes = AudioPlayerEWRAM
	report.MixerIWRAMBytes = AudioMixerIWRAM
	report.VoiceProfile = "standard"
	report.VoiceCount = audio.SFXMixerVoices
	report.MixWorkload = "4 synth voices"
	report.MixCycles = AudioStandardMixCycles
	report.CoexistEmpty = AudioStandardEmptyTick
	report.CoexistPassCases = AudioStandardPassCases
	if len(samples) != 0 {
		report.VoiceProfile = "sample"
		report.VoiceCount = audio.SFXSampleMixerVoices
		report.MixWorkload = "4 synth + 1 PCM"
		report.MixCycles = AudioSampleMixCycles
		report.CoexistEmpty = AudioSampleEmptyTick
		report.CoexistPassCases = AudioSamplePassCases
	}
	report.MixLoadTenths = audioLoadTenths(report.MixCycles)
	report.ProducerMax = AudioMaxProducerCycles
	report.ProducerTenths = audioLoadTenths(report.ProducerMax)
	report.CoexistTenths = audioFrameLoadTenths(report.CoexistEmpty)
	return report, nil
}
