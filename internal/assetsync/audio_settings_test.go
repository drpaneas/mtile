package assetsync

import (
	"os"
	"strings"
	"testing"
)

func TestAudioSettingsMissingDefaultAndStrictRoundTrip(t *testing.T) {
	dir := t.TempDir()
	settings, err := LoadAudioSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if *settings != DefaultAudioSettings() {
		t.Fatalf("missing settings = %#v, want %#v", *settings, DefaultAudioSettings())
	}
	settings.MasterVolume = 160
	if err := SaveAudioSettings(dir, settings); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadAudioSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if *reloaded != *settings {
		t.Fatalf("settings round trip = %#v, want %#v", *reloaded, *settings)
	}

	path := audioSettingsPath(dir)
	for _, raw := range []string{
		`{"version":1,"output":"mono","sampleRate":16384,"voiceProfile":"auto","masterVolume":256,"unknown":true}`,
		`{"version":1,"output":"mono","sampleRate":16384,"voiceProfile":"auto","masterVolume":256} {}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadAudioSettings(dir); err == nil {
			t.Fatalf("invalid settings JSON accepted: %s", raw)
		}
	}
}

func TestAudioSettingsRejectUnsupportedProfiles(t *testing.T) {
	tests := []struct {
		name string
		edit func(*AudioSettings)
		want string
	}{
		{name: "version", edit: func(s *AudioSettings) { s.Version = 2 }, want: "version"},
		{name: "output", edit: func(s *AudioSettings) { s.Output = "stereo" }, want: "output"},
		{name: "rate", edit: func(s *AudioSettings) { s.SampleRate = 22050 }, want: "sample rate"},
		{name: "profile", edit: func(s *AudioSettings) { s.VoiceProfile = "twelve" }, want: "voice profile"},
		{name: "master low", edit: func(s *AudioSettings) { s.MasterVolume = -1 }, want: "master volume"},
		{name: "master high", edit: func(s *AudioSettings) { s.MasterVolume = 257 }, want: "master volume"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := DefaultAudioSettings()
			test.edit(&settings)
			if err := ValidateAudioSettings(settings); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateAudioSettings() = %v, want %q", err, test.want)
			}
		})
	}
}
