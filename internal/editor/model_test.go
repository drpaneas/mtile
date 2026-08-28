package editor

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func editorPalette(name string, n int) assetsync.Palette {
	colors := make([]assetsync.RGB15, n)
	for i := range colors {
		colors[i] = assetsync.RGB15{R: i & 31, G: (i + 3) & 31, B: (i + 7) & 31}
	}
	return assetsync.Palette{Name: name, Colors: colors}
}

func TestLegacyGraphicsOnlySaveDoesNotCreateOptionalAudioFiles(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		filepath.Join("assets", "audio", "settings.json"),
		filepath.Join("assets", "audio", "sfx.json"),
		filepath.Join("assets", "audio", "music.json"),
		filepath.Join("assets", "audio", "samples", "manifest.json"),
	} {
		if _, err := os.Stat(filepath.Join(dir, relative)); !os.IsNotExist(err) {
			t.Fatalf("legacy save unexpectedly created %s (error %v)", relative, err)
		}
	}
}

func TestGraphicsOnlyProjectCanExplicitlyStartNativeAudioV2(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.EnableNativeAudioV2(); err != nil {
		t.Fatal(err)
	}
	if m.AudioVersion != assetsync.NativeAudioSchemaVersion || m.NativeAudio.Settings.Profile != assetsync.NativeAudioProfile {
		t.Fatalf("native audio was not initialized: version=%d assets=%#v", m.AudioVersion, m.NativeAudio)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.json", "sounds.json", "music.json"} {
		if _, err := os.Stat(filepath.Join(dir, "assets", "audio", name)); err != nil {
			t.Fatalf("native audio file %s missing: %v", name, err)
		}
	}
}

func TestLegacyAuthoredAudioCannotBeSilentlyReplacedByV2(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddSFX(); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.EnableNativeAudioV2(); err == nil || !strings.Contains(err.Error(), "convert explicitly") {
		t.Fatalf("legacy conversion error = %v", err)
	}
	if reloaded.AudioVersion == assetsync.NativeAudioSchemaVersion {
		t.Fatal("legacy assets were silently replaced")
	}
}

func TestAudioMasterVolumePersistsWithoutCreatingOtherAudioAssets(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.AudioSettings != assetsync.DefaultAudioSettings() {
		t.Fatalf("default audio settings = %#v", m.AudioSettings)
	}
	if err := m.SetAudioMasterVolume(160); err != nil {
		t.Fatal(err)
	}
	if err := m.SetAudioMasterVolume(257); err == nil || m.AudioSettings.MasterVolume != 160 {
		t.Fatalf("invalid master edit changed settings: %#v error=%v", m.AudioSettings, err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AudioSettings.MasterVolume != 160 {
		t.Fatalf("reloaded master = %d, want 160", reloaded.AudioSettings.MasterVolume)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "audio", "settings.json")); err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
	for _, relative := range []string{
		filepath.Join("assets", "audio", "sfx.json"),
		filepath.Join("assets", "audio", "music.json"),
		filepath.Join("assets", "audio", "samples", "manifest.json"),
	} {
		if _, err := os.Stat(filepath.Join(dir, relative)); !os.IsNotExist(err) {
			t.Fatalf("settings save unexpectedly created %s (error %v)", relative, err)
		}
	}
}

func TestOptionalAudioSaveCreatesOnlyAuthoredAssetType(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddSFX(); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "audio", "sfx.json")); err != nil {
		t.Fatalf("authored SFX was not saved: %v", err)
	}
	for _, relative := range []string{
		filepath.Join("assets", "audio", "music.json"),
		filepath.Join("assets", "audio", "samples", "manifest.json"),
	} {
		if _, err := os.Stat(filepath.Join(dir, relative)); !os.IsNotExist(err) {
			t.Fatalf("SFX save unexpectedly created %s (error %v)", relative, err)
		}
	}
}

func editorFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	palette := editorPalette("wide", 256)
	if err := assetsync.SavePalette(dir, &palette); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SetActivePalette(dir, palette.Name); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveSpritesheet(dir, &assetsync.Spritesheet{Width: 240, Height: 160}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveMap(dir, &assetsync.Map{Width: 300, Height: 200, Cells: make([]int, 300*200)}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestOpenSupportsNativeDimensionsAndPalettePreview(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Sheet.Width != 240 || m.Sheet.Height != 160 || m.Map.Width != 300 || m.Map.Height != 200 {
		t.Fatalf("native dimensions were changed or capped: sheet=%dx%d map=%dx%d", m.Sheet.Width, m.Sheet.Height, m.Map.Width, m.Map.Height)
	}
	if m.ActivePalette.Name != "wide" || !slices.Contains(m.PaletteNames, "wide") {
		t.Fatalf("active palette/library not loaded: active=%q names=%#v", m.ActivePalette.Name, m.PaletteNames)
	}

	if err := m.SetSpritePixel(599, 7, 7, 255); err != nil {
		t.Fatal(err)
	}
	preview, err := m.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Width != 240 || preview.Height != 160 || len(preview.Pixels) != 240*160 {
		t.Fatalf("unexpected native preview: %dx%d len=%d", preview.Width, preview.Height, len(preview.Pixels))
	}
	if preview.Pixels[159*240+239] != 255 {
		t.Fatalf("last native sprite pixel was not painted: %d", preview.Pixels[159*240+239])
	}

	newColor := assetsync.RGB15{R: 31, G: 2, B: 27}
	if err := m.SetActiveColor(255, newColor); err != nil {
		t.Fatal(err)
	}
	preview, err = m.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Colors[255] != newColor {
		t.Fatalf("palette edit did not update preview: %#v", preview.Colors[255])
	}

	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Project.ActivePalette != "wide" || reloaded.ActivePalette.Colors[255] != newColor {
		t.Fatalf("active palette did not survive save/load: project=%#v color=%#v", reloaded.Project, reloaded.ActivePalette.Colors[255])
	}
	preview, err = reloaded.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Pixels[159*240+239] != 255 || preview.Colors[255] != newColor {
		t.Fatal("saved pixel/palette did not survive preview reload")
	}
}

func TestModelSFXAuthoringPersistsAndKeepsStableIDs(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.SFX.Version != assetsync.SFXSchemaVersion || len(m.SFX.SFX) != 0 {
		t.Fatalf("initial SFX library = %#v", m.SFX)
	}
	first, err := m.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	if first != 0 || second != 1 || m.SFX.SFX[first].ID != 0 || m.SFX.SFX[second].ID != 1 {
		t.Fatalf("added SFX indices/IDs first=%d second=%d library=%#v", first, second, m.SFX.SFX)
	}
	effect, _ := m.SFXAsset(second)
	effect.Name = "laser"
	effect.Speed = 3
	effect.Steps[0] = assetsync.SFXStep{Pitch: 63, Waveform: "pulse", Volume: 5, Effect: "slide"}
	if err := m.UpdateSFX(second, effect); err != nil {
		t.Fatal(err)
	}
	newStep, err := m.AddSFXStep(second)
	if err != nil {
		t.Fatal(err)
	}
	if newStep != 1 {
		t.Fatalf("new step index = %d, want 1", newStep)
	}
	effect, _ = m.SFXAsset(second)
	effect.Loop = &assetsync.SFXLoop{Start: 0, End: 2}
	if err := m.UpdateSFX(second, effect); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.SFXAsset(second)
	if !ok || got.ID != 1 || got.Name != "laser" || got.Speed != 3 || len(got.Steps) != 2 ||
		got.Steps[0].Pitch != 63 || got.Steps[0].Waveform != "pulse" || got.Steps[0].Effect != "slide" || got.Loop == nil || *got.Loop != (assetsync.SFXLoop{Start: 0, End: 2}) {
		t.Fatalf("reloaded SFX = %#v ok=%t", got, ok)
	}
	if err := reloaded.DeleteSFX(first); err != nil {
		t.Fatal(err)
	}
	if reloaded.SFX.SFX[0].ID != 1 {
		t.Fatalf("deleting list item renumbered stable ID: %#v", reloaded.SFX.SFX)
	}
	third, err := reloaded.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SFX.SFX[third].ID != 0 {
		t.Fatalf("first free stable ID was not reused: %#v", reloaded.SFX.SFX)
	}
	if reloaded.SFX.SFX[third].Name == reloaded.SFX.SFX[0].Name {
		t.Fatalf("reused stable ID created a duplicate default name: %#v", reloaded.SFX.SFX)
	}
}

func TestModelDuplicateSFXCreatesIndependentCopyWithoutRetargetingMusic(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	original, err := m.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	pattern, err := m.AddMusicPattern()
	if err != nil {
		t.Fatal(err)
	}
	originalID := m.SFX.SFX[original].ID
	duplicate, err := m.DuplicateSFX(original)
	if err != nil {
		t.Fatal(err)
	}
	copy, _ := m.SFXAsset(duplicate)
	if duplicate != original+1 || copy.ID == originalID || copy.Name == m.SFX.SFX[original].Name || !slices.Equal(copy.Steps, m.SFX.SFX[original].Steps) {
		t.Fatalf("duplicated SFX = %#v library=%#v", copy, m.SFX.SFX)
	}
	copy.Steps[0].Pitch--
	if err := m.UpdateSFX(duplicate, copy); err != nil {
		t.Fatal(err)
	}
	if m.SFX.SFX[original].Steps[0].Pitch == copy.Steps[0].Pitch {
		t.Fatal("editing duplicate mutated original steps")
	}
	gotPattern, _ := m.MusicPattern(pattern)
	if gotPattern.Channels[0] != originalID {
		t.Fatalf("duplicate retargeted existing music to %d", gotPattern.Channels[0])
	}
}

func TestModelSFXInvalidEditIsTransactionalAndStepDeleteAdjustsLoop(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.AddSFX()
	if err != nil {
		t.Fatal(err)
	}
	for len(m.SFX.SFX[index].Steps) < 4 {
		if _, err := m.AddSFXStep(index); err != nil {
			t.Fatal(err)
		}
	}
	effect, _ := m.SFXAsset(index)
	effect.Loop = &assetsync.SFXLoop{Start: 1, End: 4}
	if err := m.UpdateSFX(index, effect); err != nil {
		t.Fatal(err)
	}
	before, _ := m.SFXAsset(index)
	invalid := before
	invalid.Speed = 0
	if err := m.UpdateSFX(index, invalid); err == nil {
		t.Fatal("invalid speed edit succeeded")
	}
	after, _ := m.SFXAsset(index)
	if after.Speed != before.Speed {
		t.Fatalf("invalid edit mutated speed from %d to %d", before.Speed, after.Speed)
	}

	if err := m.DeleteSFXStep(index, 0); err != nil {
		t.Fatal(err)
	}
	after, _ = m.SFXAsset(index)
	if after.Loop == nil || *after.Loop != (assetsync.SFXLoop{Start: 0, End: 3}) {
		t.Fatalf("loop after deleting pre-loop step = %#v", after.Loop)
	}
	if err := m.DeleteSFXStep(index, 1); err != nil {
		t.Fatal(err)
	}
	after, _ = m.SFXAsset(index)
	if after.Loop == nil || *after.Loop != (assetsync.SFXLoop{Start: 0, End: 2}) {
		t.Fatalf("loop after deleting in-loop step = %#v", after.Loop)
	}
}

func TestPaletteLibraryOperations(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreatePalette(editorPalette("dawn", 16)); err != nil {
		t.Fatal(err)
	}
	if err := m.DuplicatePalette("dawn", "dawn-copy"); err != nil {
		t.Fatal(err)
	}
	if err := m.SelectPalette("dawn-copy"); err != nil {
		t.Fatal(err)
	}
	if m.Project.ActivePalette != "dawn-copy" || len(m.ActivePalette.Colors) != 16 {
		t.Fatalf("palette selection failed: project=%#v palette=%#v", m.Project, m.ActivePalette)
	}
	if err := m.SetActiveColor(16, assetsync.RGB15{R: 1}); err == nil {
		t.Fatal("16-color shorthand exposed an editable aliased entry")
	}
	preview, err := m.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Colors[16] != preview.Colors[0] {
		t.Fatal("16-color shorthand preview did not alias color zero")
	}
	if err := m.RenamePalette("dawn-copy", "dawn-active"); err != nil {
		t.Fatal(err)
	}
	if m.Project.ActivePalette != "dawn-active" {
		t.Fatalf("active palette was not renamed in model: %q", m.Project.ActivePalette)
	}
	diskProject, err := assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if diskProject.ActivePalette != "dawn-active" {
		t.Fatalf("active palette rename left disk selection at %q", diskProject.ActivePalette)
	}
	if err := m.DeletePalette("dawn-active"); err == nil {
		t.Fatal("deleting active palette unexpectedly succeeded")
	}
	if err := m.SelectPalette(assetsync.CanonicalPaletteName); err != nil {
		t.Fatal(err)
	}
	if err := m.DeletePalette("dawn-active"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(m.PaletteNames, "dawn-active") {
		t.Fatalf("deleted palette remains in model library: %#v", m.PaletteNames)
	}
}

func TestModelDeletePaletteRejectsCanonicalBeforeActiveCheck(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = m.DeletePalette(assetsync.CanonicalPaletteName)
	if err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("expected built-in-palette error, got: %v", err)
	}
}

func TestActivePaletteLibraryOperationsPreserveUnsavedColors(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	updated := assetsync.RGB15{R: 31, G: 2, B: 9}
	if err := m.SetActiveColor(255, updated); err != nil {
		t.Fatal(err)
	}
	if err := m.RenamePalette("wide", "wide-renamed"); err != nil {
		t.Fatal(err)
	}
	renamed, err := assetsync.LoadPalette(dir, "wide-renamed")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Colors[255] != updated {
		t.Fatalf("active rename lost unsaved color: got %#v want %#v", renamed.Colors[255], updated)
	}

	updatedAgain := assetsync.RGB15{R: 3, G: 27, B: 14}
	if err := m.SetActiveColor(255, updatedAgain); err != nil {
		t.Fatal(err)
	}
	if err := m.DuplicatePalette("wide-renamed", "wide-copy"); err != nil {
		t.Fatal(err)
	}
	copy, err := assetsync.LoadPalette(dir, "wide-copy")
	if err != nil {
		t.Fatal(err)
	}
	if copy.Colors[255] != updatedAgain {
		t.Fatalf("active duplicate lost unsaved color: got %#v want %#v", copy.Colors[255], updatedAgain)
	}
}

func TestPaletteEditsAndSelectionPersistOnlyOnSave(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	original := m.ActivePalette.Colors[255]
	updated := assetsync.RGB15{R: 31, G: 1, B: 2}
	if err := m.SetActiveColor(255, updated); err != nil {
		t.Fatal(err)
	}
	diskBefore, err := assetsync.LoadPalette(dir, "wide")
	if err != nil {
		t.Fatal(err)
	}
	if diskBefore.Colors[255] != original {
		t.Fatalf("palette edit was written before Save: got %#v want %#v", diskBefore.Colors[255], original)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	diskAfter, err := assetsync.LoadPalette(dir, "wide")
	if err != nil {
		t.Fatal(err)
	}
	if diskAfter.Colors[255] != updated {
		t.Fatalf("palette edit was not written by Save: got %#v want %#v", diskAfter.Colors[255], updated)
	}

	if err := assetsync.CreatePalette(dir, editorPalette("dawn", 16)); err != nil {
		t.Fatal(err)
	}
	if err := m.SelectPalette("dawn"); err != nil {
		t.Fatal(err)
	}
	diskProject, err := assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if diskProject.ActivePalette != "wide" {
		t.Fatalf("palette selection was written before Save: %q", diskProject.ActivePalette)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	diskProject, err = assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if diskProject.ActivePalette != "dawn" {
		t.Fatalf("palette selection was not written by Save: %q", diskProject.ActivePalette)
	}
}

// TestDeletePaletteUsesInMemoryActiveSelectionNotStaleDisk guards a real bug
// found in review: SelectPalette only updates m.Project/m.ActivePalette in
// memory (Save persists it), so after selecting away from a palette without
// saving, project.json on disk still names the old palette active.
// DeletePalette's guard must trust the model's current in-memory selection,
// not delegate to assetsync.DeletePalette's disk-based check - otherwise
// deleting a palette the user has already (in-memory) moved away from fails
// with a "cannot delete active palette" error that no longer reflects
// reality.
func TestDeletePaletteUsesInMemoryActiveSelectionNotStaleDisk(t *testing.T) {
	dir := editorFixture(t) // disk and memory both start active="wide"
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreatePalette(editorPalette("dawn", 16)); err != nil {
		t.Fatal(err)
	}
	if err := m.SelectPalette("dawn"); err != nil {
		t.Fatal(err)
	}
	diskProject, err := assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if diskProject.ActivePalette != "wide" {
		t.Fatalf("test setup assumption broken: disk active = %q, want unsaved %q", diskProject.ActivePalette, "wide")
	}

	if err := m.DeletePalette("wide"); err != nil {
		t.Fatalf("deleting a palette no longer active in memory (even though unsaved on disk) should succeed: %v", err)
	}
	if slices.Contains(m.PaletteNames, "wide") {
		t.Fatal("deleted palette still present in model library")
	}
	diskProject, err = assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if diskProject.ActivePalette != "dawn" {
		t.Fatalf("delete left a dangling disk selection: %q", diskProject.ActivePalette)
	}
}

func TestRenamePaletteUsesInMemorySelectionNotStaleDisk(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreatePalette(editorPalette("dawn", 16)); err != nil {
		t.Fatal(err)
	}
	if err := m.SelectPalette("dawn"); err != nil {
		t.Fatal(err)
	}

	// The model has unsaved active="dawn", while project.json still names
	// "wide". Renaming that now-inactive palette must not rewrite the stale
	// on-disk selection as a side effect.
	if err := m.RenamePalette("wide", "wide-renamed"); err != nil {
		t.Fatal(err)
	}
	diskProject, err := assetsync.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if diskProject.ActivePalette != "wide" {
		t.Fatalf("rename changed stale disk selection to %q", diskProject.ActivePalette)
	}
	if m.Project.ActivePalette != "dawn" {
		t.Fatalf("rename changed in-memory selection to %q", m.Project.ActivePalette)
	}
	if _, err := assetsync.LoadPalette(dir, "wide-renamed"); err != nil {
		t.Fatalf("renamed palette is not loadable: %v", err)
	}
	if _, err := assetsync.LoadPalette(dir, "wide"); err == nil {
		t.Fatal("old palette name still loads after rename")
	}
}

// TestReloadDiscardsInMemoryEditsAndReflectsDisk guards Model.Reload, which
// had no test coverage despite being documented as part of the model's
// public contract alongside Open/Save. It must throw away unsaved in-memory
// state (sheet pixel, palette color, palette selection) and replace it with
// whatever is actually on disk, exactly as if the project had just been
// freshly opened.
func TestReloadDiscardsInMemoryEditsAndReflectsDisk(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.SetSpritePixel(1, 0, 0, 9); err != nil {
		t.Fatal(err)
	}
	if err := m.SetActiveColor(1, assetsync.RGB15{R: 31, G: 0, B: 0}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.CreatePalette(dir, editorPalette("dawn", 16)); err != nil {
		t.Fatal(err)
	}
	if err := m.SelectPalette("dawn"); err != nil {
		t.Fatal(err)
	}
	if got := m.SpriteFlagByte(0); got != 0 {
		t.Fatalf("test setup assumption broken: sprite 0 already had flags %#x", got)
	}

	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	if m.Dir != dir {
		t.Fatalf("Reload changed Dir: got %q, want %q", m.Dir, dir)
	}
	if m.Project.ActivePalette != "wide" {
		t.Fatalf("Reload kept the unsaved palette selection: %q", m.Project.ActivePalette)
	}
	if m.ActivePalette.Colors[1] == (assetsync.RGB15{R: 31, G: 0, B: 0}) {
		t.Fatal("Reload kept an unsaved palette color edit")
	}
	preview, err := m.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Pixels[8] == 9 {
		t.Fatal("Reload kept an unsaved sprite pixel edit")
	}
}

func TestSetSpritePixelRejectsReservedTransparentEraser(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetSpritePixel(ReservedSpriteID, 0, 0, 9); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved sprite edit error = %v", err)
	}
	preview, err := m.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if got := preview.Pixels[0]; got != 0 {
		t.Fatalf("reserved sprite pixel = %d, want transparent color 0", got)
	}
}

func TestSetSpriteFlagCreatesRecordAndPreservesPixels(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got := m.SpriteFlagByte(5); got != 0 {
		t.Fatalf("unauthored sprite flag byte = %d, want 0", got)
	}
	if err := m.SetSpriteFlag(5, 3, true); err != nil {
		t.Fatal(err)
	}
	if got := m.SpriteFlagByte(5); got != 1<<3 {
		t.Fatalf("SpriteFlagByte(5) = %#x, want %#x", got, 1<<3)
	}

	if err := m.SetSpritePixel(5, 0, 0, 7); err != nil {
		t.Fatal(err)
	}
	if err := m.SetSpriteFlag(5, 6, true); err != nil {
		t.Fatal(err)
	}
	if got := m.SpriteFlagByte(5); got != 1<<3|1<<6 {
		t.Fatalf("SpriteFlagByte(5) = %#x, want %#x (must not clobber bit 3)", got, 1<<3|1<<6)
	}
	preview, err := m.Preview()
	if err != nil {
		t.Fatal(err)
	}
	cols := m.Sheet.Width / 8
	px := (5%cols)*8 + 0
	py := (5/cols)*8 + 0
	if got := preview.Pixels[py*preview.Width+px]; got != 7 {
		t.Fatalf("flag toggle clobbered previously painted pixel: got %d, want 7", got)
	}

	if err := m.SetSpriteFlag(5, 3, false); err != nil {
		t.Fatal(err)
	}
	if got := m.SpriteFlagByte(5); got != 1<<6 {
		t.Fatalf("SpriteFlagByte(5) after clearing bit 3 = %#x, want %#x", got, 1<<6)
	}

	if err := m.SetSpriteFlag(-1, 0, true); err == nil {
		t.Fatal("negative sprite id unexpectedly accepted")
	}
	if err := m.SetSpriteFlag(0, 8, true); err == nil {
		t.Fatal("out-of-range flag bit unexpectedly accepted")
	}
}

func TestSetActiveColorRejectsRGB15OutOfRange(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetActiveColor(0, assetsync.RGB15{R: 32, G: 0, B: 0}); err == nil {
		t.Fatal("SetActiveColor accepted R=32")
	}
	if err := m.SetActiveColor(0, assetsync.RGB15{R: 0, G: -1, B: 0}); err == nil {
		t.Fatal("SetActiveColor accepted G=-1")
	}
}

func TestSetSpritePixelRejectsArtificialAndInvalidLimits(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, x, y, color int
	}{
		{id: -1, x: 0, y: 0, color: 0},
		{id: 600, x: 0, y: 0, color: 0},
		{id: 0, x: 8, y: 0, color: 0},
		{id: 0, x: 0, y: 8, color: 0},
		{id: 0, x: 0, y: 0, color: 256},
	} {
		if err := m.SetSpritePixel(tc.id, tc.x, tc.y, tc.color); err == nil {
			t.Errorf("invalid pixel operation unexpectedly passed: %#v", tc)
		}
	}
}

func TestResizeMapPreservesOverlappingCells(t *testing.T) {
	dir := t.TempDir()
	palette := editorPalette("wide", 256)
	if err := assetsync.SavePalette(dir, &palette); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SetActivePalette(dir, palette.Name); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveSpritesheet(dir, &assetsync.Spritesheet{Width: 16, Height: 16}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveMap(dir, &assetsync.Map{Width: 2, Height: 2, Cells: []int{1, 0, 0, 2}}); err != nil {
		t.Fatal(err)
	}
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.ResizeMap(4, 3); err != nil {
		t.Fatal(err)
	}
	if m.Map.Width != 4 || m.Map.Height != 3 || len(m.Map.Cells) != 12 {
		t.Fatalf("resized map = %dx%d cells=%d, want 4x3 12", m.Map.Width, m.Map.Height, len(m.Map.Cells))
	}
	if m.Map.Cells[0] != 1 {
		t.Fatalf("top-left cell = %d, want 1", m.Map.Cells[0])
	}
	if m.Map.Cells[5] != 2 {
		t.Fatalf("copied (1,1) = %d, want 2", m.Map.Cells[5])
	}
	if m.Map.Cells[3] != 0 || m.Map.Cells[11] != 0 {
		t.Fatal("new cells must be the eraser sprite 0")
	}

	if err := m.ResizeMap(1, 1); err != nil {
		t.Fatal(err)
	}
	if m.Map.Width != 1 || m.Map.Height != 1 || m.Map.Cells[0] != 1 {
		t.Fatalf("shrunk map = %dx%d cell0=%d, want 1x1 cell 1", m.Map.Width, m.Map.Height, m.Map.Cells[0])
	}

	if err := m.ResizeMap(0, 4); err == nil {
		t.Fatal("ResizeMap accepted a zero width")
	}
	if err := m.ResizeMap(4, -1); err == nil {
		t.Fatal("ResizeMap accepted a negative height")
	}
}
