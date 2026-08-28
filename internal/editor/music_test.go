package editor

import (
	"slices"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestModelMusicAuthoringPersistsAndRetargetsStableIDs(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sfxIndex, err := m.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	first, err := m.AddMusicPattern()
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.AddMusicPattern()
	if err != nil {
		t.Fatal(err)
	}
	pattern, _ := m.MusicPattern(first)
	pattern.Name = "intro"
	next := m.Music.Patterns[second].ID
	pattern.Next = &next
	if err := m.UpdateMusicPattern(first, pattern); err != nil {
		t.Fatal(err)
	}

	effect, _ := m.SFXAsset(sfxIndex)
	effect.ID = 9
	if err := m.UpdateSFX(sfxIndex, effect); err != nil {
		t.Fatal(err)
	}
	for _, authored := range m.Music.Patterns {
		if authored.Channels[0] != 9 {
			t.Fatalf("SFX ID edit did not retarget music: %#v", m.Music.Patterns)
		}
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.MusicPattern(first)
	if !ok || got.Name != "intro" || got.Channels[0] != 9 || got.Next == nil || *got.Next != reloaded.Music.Patterns[second].ID {
		t.Fatalf("reloaded music pattern = %#v ok=%t", got, ok)
	}
	if err := reloaded.DeleteSFX(sfxIndex); err == nil {
		t.Fatal("deleting an SFX referenced by music succeeded")
	}
}

func TestModelMusicDeleteTurnsIncomingTransitionsIntoStops(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddSFX(); err != nil {
		t.Fatal(err)
	}
	first, _ := m.AddMusicPattern()
	second, _ := m.AddMusicPattern()
	firstPattern, _ := m.MusicPattern(first)
	secondID := m.Music.Patterns[second].ID
	firstPattern.Next = &secondID
	if err := m.UpdateMusicPattern(first, firstPattern); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteMusicPattern(second); err != nil {
		t.Fatal(err)
	}
	remaining, _ := m.MusicPattern(0)
	if remaining.Next == nil || *remaining.Next != assetsync.MusicStopPattern {
		t.Fatalf("incoming transition after delete = %#v", remaining.Next)
	}
}

func TestModelMusicInvalidEditIsTransactional(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddMusicPattern(); err == nil {
		t.Fatal("music pattern was added without any SFX")
	}
	if _, err := m.AddSFX(); err != nil {
		t.Fatal(err)
	}
	index, err := m.AddMusicPattern()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := m.MusicPattern(index)
	invalid := cloneMusicPattern(before)
	invalid.Channels = []int{-1, -1, -1, -1}
	if err := m.UpdateMusicPattern(index, invalid); err == nil {
		t.Fatal("fully silent pattern edit succeeded")
	}
	after, _ := m.MusicPattern(index)
	if !slices.Equal(before.Channels, after.Channels) {
		t.Fatalf("invalid edit mutated channels from %v to %v", before.Channels, after.Channels)
	}
}

func TestModelPasteSFXIntoMusicPatternCreatesIndependentCopy(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	source, err := m.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	patternIndex, err := m.AddMusicPattern()
	if err != nil {
		t.Fatal(err)
	}
	newIndex, err := m.PasteSFXIntoMusicPattern(source, patternIndex, 2)
	if err != nil {
		t.Fatal(err)
	}
	sourceClip, _ := m.SFXAsset(source)
	copyClip, _ := m.SFXAsset(newIndex)
	pattern, _ := m.MusicPattern(patternIndex)
	if copyClip.ID == sourceClip.ID || copyClip.Name == sourceClip.Name || pattern.Channels[2] != copyClip.ID {
		t.Fatalf("paste source=%#v copy=%#v pattern=%#v", sourceClip, copyClip, pattern)
	}
	copyClip.Steps[0].Pitch++
	if err := m.UpdateSFX(newIndex, copyClip); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := m.SFXAsset(source)
	if unchanged.Steps[0].Pitch == copyClip.Steps[0].Pitch {
		t.Fatal("editing pasted clip changed source clip")
	}
}

func TestModelMusicInsertAndDuplicateBuildLinearArrangement(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddSFX(); err != nil {
		t.Fatal(err)
	}
	first, err := m.InsertMusicPatternAfter(-1)
	if err != nil {
		t.Fatal(err)
	}
	firstPattern, _ := m.MusicPattern(first)
	if firstPattern.Next == nil || *firstPattern.Next != assetsync.MusicStopPattern {
		t.Fatalf("first pattern flow = %#v, want one-shot stop", firstPattern.Next)
	}
	second, err := m.DuplicateMusicPattern(first)
	if err != nil {
		t.Fatal(err)
	}
	firstPattern, _ = m.MusicPattern(first)
	secondPattern, _ := m.MusicPattern(second)
	if firstPattern.Next == nil || *firstPattern.Next != secondPattern.ID || secondPattern.Next == nil || *secondPattern.Next != assetsync.MusicStopPattern {
		t.Fatalf("duplicated flow first=%#v second=%#v", firstPattern, secondPattern)
	}
	if !slices.Equal(firstPattern.Channels, secondPattern.Channels) || firstPattern.Name == secondPattern.Name || firstPattern.ID == secondPattern.ID {
		t.Fatalf("duplicate did not preserve voices with unique identity: first=%#v second=%#v", firstPattern, secondPattern)
	}
}

func TestModelMusicLabMetadataAndTempoAreTransactional(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	used, err := m.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	unused, err := m.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddMusicPattern(); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateMusicName("Adventure Starts"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetMusicSpeed(12); err != nil {
		t.Fatal(err)
	}
	usedClip, _ := m.SFXAsset(used)
	unusedClip, _ := m.SFXAsset(unused)
	if usedClip.Speed != 12 || unusedClip.Speed == 12 {
		t.Fatalf("tempo update used=%d unused=%d", usedClip.Speed, unusedClip.Speed)
	}
	if err := m.SetMusicSpeed(0); err == nil || usedClip.Speed != 12 {
		t.Fatal("invalid tempo update succeeded")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(m.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Music.Name != "Adventure Starts" || reloaded.SFX.SFX[used].Speed != 12 {
		t.Fatalf("reloaded music name=%q speed=%d", reloaded.Music.Name, reloaded.SFX.SFX[used].Speed)
	}
}
