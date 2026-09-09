package fonts

import (
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

func TestOpenDyslexicAssetParsesAsMonospacedFace(t *testing.T) {
	parsed, err := opentype.Parse(OpenDyslexic)
	if err != nil {
		t.Fatalf("parse OpenDyslexic asset: %v", err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 9, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		t.Fatalf("create OpenDyslexic face: %v", err)
	}
	defer func() { _ = face.Close() }()

	if got := font.MeasureString(face, "M"); got <= 0 {
		t.Fatalf("OpenDyslexic face has no advance: %v", got)
	}
	monoAdvance := font.MeasureString(face, "M")
	if got := font.MeasureString(face, "i"); got != monoAdvance {
		t.Fatalf("OpenDyslexic Mono advances differ: M=%v i=%v", monoAdvance, got)
	}
}

func TestOpenDyslexicUIBoldAssetParses(t *testing.T) {
	parsed, err := opentype.Parse(OpenDyslexicUIBold)
	if err != nil {
		t.Fatalf("parse bold OpenDyslexic UI asset: %v", err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 13, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		t.Fatalf("create bold OpenDyslexic UI face: %v", err)
	}
	defer func() { _ = face.Close() }()
	if got := font.MeasureString(face, "MTILE"); got <= 0 {
		t.Fatalf("bold OpenDyslexic UI face has no advance: %v", got)
	}
}

func TestJetBrainsMonoAssetParsesAsMonospacedFace(t *testing.T) {
	parsed, err := opentype.Parse(JetBrainsMonoRegular)
	if err != nil {
		t.Fatalf("parse JetBrains Mono asset: %v", err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 16, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		t.Fatalf("create JetBrains Mono face: %v", err)
	}
	defer func() { _ = face.Close() }()

	want := font.MeasureString(face, "M")
	if want <= 0 {
		t.Fatalf("JetBrains Mono face has no advance: %v", want)
	}
	for r := rune(32); r <= 126; r++ {
		if got := font.MeasureString(face, string(r)); got != want {
			t.Fatalf("JetBrains Mono rune %q advance = %v, want %v", r, got, want)
		}
	}
}
