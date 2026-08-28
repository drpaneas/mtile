package editor

import (
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestAudioReportTracksInactiveAndSampleProfiles(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	report, err := m.AudioReport()
	if err != nil {
		t.Fatal(err)
	}
	if report.Enabled || report.VoiceCount != 0 || report.AssetROMBytes != 0 || report.SampleRate != assetsync.SampleTargetRate {
		t.Fatalf("inactive audio report = %#v", report)
	}
	if _, err := m.ImportSampleWAV("stereo.wav", editorPCM8WAV(t, 8192, 2, []byte{
		255, 0,
		128, 128,
		0, 255,
	})); err != nil {
		t.Fatal(err)
	}
	report, err = m.AudioReport()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Enabled || report.Output != "mono" || report.VoiceProfile != "sample" || report.VoiceCount != 12 {
		t.Fatalf("sample audio profile = %#v", report)
	}
	if report.AssetROMBytes != assetsync.CompiledSampleDefinitionBytes+6 || report.SamplePCMBytes != 6 {
		t.Fatalf("sample audio footprint = %#v", report)
	}
	if report.MixCycles != AudioSampleMixCycles || report.MixLoadTenths != 420 ||
		report.ProducerTenths != 961 || report.CoexistEmpty != AudioSampleEmptyTick ||
		report.CoexistTenths != 993 || report.CoexistPassCases != 2 ||
		report.CoexistCaseCount != 7 {
		t.Fatalf("sample measured baseline = %#v", report)
	}
	if err := m.SetAudioMasterVolume(160); err != nil {
		t.Fatal(err)
	}
	report, err = m.AudioReport()
	if err != nil {
		t.Fatal(err)
	}
	if report.MasterVolume != 160 {
		t.Fatalf("audio report master = %d, want 160", report.MasterVolume)
	}
}
