// Package editor contains the data model shared by Mtile's native IDE and
// its persistence layer. It intentionally has no windowing
// dependency: the Ebitengine view can be added on top without putting display
// caps or GUI state into the asset contract.
package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// ReservedSpriteID is the permanently transparent map eraser. The editor may
// select it so users can erase map cells, but must never author pixels into it.
const ReservedSpriteID = 0

// Model is one editable native project. Sheet and map dimensions are project
// data, not a fixed display or cartridge size.
type Model struct {
	Dir           string
	Project       assetsync.Project
	Sheet         assetsync.Spritesheet
	Map           assetsync.Map
	SFX           assetsync.SFXLibrary
	Music         assetsync.MusicLibrary
	Samples       assetsync.SampleLibrary
	AudioSettings assetsync.AudioSettings
	AudioVersion  int
	NativeAudio   assetsync.NativeAudioAssets
	// NativeSongIndex is editor session state. It selects which song the
	// one-screen Music Lab operates on and is never serialized into assets.
	NativeSongIndex int
	ActivePalette   assetsync.Palette
	PaletteNames    []string

	// Optional audio files stay absent in legacy graphics-only projects until
	// the user actually authors that asset type. This avoids turning a normal
	// editor save or quit into an unrelated project-layout migration.
	sfxFilePresent    bool
	musicFilePresent  bool
	sampleFilePresent bool
	settingsPresent   bool
}

func optionalAudioFilePresent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Open loads and validates a project, including its active palette. Missing
// project.json means the canonical aurora palette is selected by assetsync.
func Open(dir string) (*Model, error) {
	sfxFilePresent := optionalAudioFilePresent(filepath.Join(dir, "assets", "audio", "sfx.json"))
	musicFilePresent := optionalAudioFilePresent(filepath.Join(dir, "assets", "audio", "music.json"))
	sampleFilePresent := optionalAudioFilePresent(filepath.Join(dir, "assets", "audio", "samples", "manifest.json"))
	settingsPresent := optionalAudioFilePresent(filepath.Join(dir, "assets", "audio", "settings.json"))
	nativeSoundsPresent := optionalAudioFilePresent(filepath.Join(dir, "assets", "audio", "sounds.json"))
	project, err := assetsync.LoadProject(dir)
	if err != nil {
		return nil, err
	}
	sheet, err := assetsync.LoadSpritesheet(dir)
	if err != nil {
		return nil, err
	}
	mapData, err := assetsync.LoadMap(dir)
	if err != nil {
		return nil, err
	}
	palette, err := assetsync.LoadPalette(dir, project.ActivePalette)
	if err != nil {
		return nil, err
	}
	if _, err := assetsync.Flatten(project, *sheet, *mapData, *palette); err != nil {
		return nil, err
	}
	names, err := assetsync.ListPalettes(dir)
	if err != nil {
		return nil, err
	}
	model := &Model{
		Dir:               dir,
		Project:           project,
		Sheet:             *sheet,
		Map:               *mapData,
		SFX:               assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion},
		Music:             assetsync.MusicLibrary{Version: assetsync.MusicSchemaVersion},
		Samples:           assetsync.SampleLibrary{Version: assetsync.SampleSchemaVersion},
		AudioSettings:     assetsync.DefaultAudioSettings(),
		ActivePalette:     *palette,
		PaletteNames:      names,
		sfxFilePresent:    sfxFilePresent,
		musicFilePresent:  musicFilePresent,
		sampleFilePresent: sampleFilePresent,
		settingsPresent:   settingsPresent,
	}
	version, err := assetsync.DetectAudioVersion(dir)
	if err != nil {
		return nil, err
	}
	// A graphics-only project that predates any audio schema remains untouched
	// in the editor. New projects explicitly contain v2 settings/sounds/music,
	// so this compatibility case cannot silently downgrade a new cartridge.
	if !settingsPresent && !nativeSoundsPresent && !musicFilePresent && !sfxFilePresent && !sampleFilePresent {
		version = assetsync.AudioSettingsSchemaVersion
	}
	model.AudioVersion = version
	if version == assetsync.NativeAudioSchemaVersion {
		native, err := assetsync.LoadNativeAudio(dir)
		if err != nil {
			return nil, err
		}
		model.NativeAudio = *native
		return model, nil
	}
	settings, err := assetsync.LoadAudioSettings(dir)
	if err != nil {
		return nil, err
	}
	sfx, err := assetsync.LoadSFX(dir)
	if err != nil {
		return nil, err
	}
	music, err := assetsync.LoadMusic(dir, *sfx)
	if err != nil {
		return nil, err
	}
	samples, err := assetsync.LoadSamples(dir)
	if err != nil {
		return nil, err
	}
	model.SFX, model.Music, model.Samples, model.AudioSettings = *sfx, *music, *samples, *settings
	return model, nil
}

// Save validates and persists the current project, active palette, sheet, map,
// SFX, music, and sample libraries. Keeping writes here makes editor undo transactional:
// restoring an in-memory snapshot and then saving cannot leave a file stale.
func (m *Model) Save() error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if m.Project.ActivePalette != m.ActivePalette.Name {
		return fmt.Errorf("active palette mismatch: project selects %q, model holds %q", m.Project.ActivePalette, m.ActivePalette.Name)
	}
	if _, err := assetsync.Flatten(m.Project, m.Sheet, m.Map, m.ActivePalette); err != nil {
		return err
	}
	if m.AudioVersion == assetsync.NativeAudioSchemaVersion {
		library, err := assetsync.CompileNativeAudio(m.NativeAudio.Settings, m.NativeAudio.Sounds, m.NativeAudio.Music)
		if err != nil {
			return err
		}
		m.NativeAudio.Library = library
	} else {
		if _, _, err := assetsync.CompileSFX(m.SFX); err != nil {
			return err
		}
		if _, err := assetsync.CompileMusic(m.Music, m.SFX); err != nil {
			return err
		}
		if _, _, err := assetsync.CompileSamples(m.Dir, m.Samples); err != nil {
			return err
		}
		if err := assetsync.ValidateAudioSettings(m.AudioSettings); err != nil {
			return err
		}
	}
	if err := assetsync.SavePalette(m.Dir, &m.ActivePalette); err != nil {
		return err
	}
	if err := assetsync.SaveProject(m.Dir, m.Project); err != nil {
		return err
	}
	if err := assetsync.SaveSpritesheet(m.Dir, &m.Sheet); err != nil {
		return err
	}
	if err := assetsync.SaveMap(m.Dir, &m.Map); err != nil {
		return err
	}
	if m.AudioVersion == assetsync.NativeAudioSchemaVersion {
		if err := assetsync.SaveNativeAudioSettings(m.Dir, &m.NativeAudio.Settings); err != nil {
			return err
		}
		if err := assetsync.SaveNativeSounds(m.Dir, &m.NativeAudio.Sounds); err != nil {
			return err
		}
		if err := assetsync.SaveNativeMusic(m.Dir, m.NativeAudio.Sounds, &m.NativeAudio.Music); err != nil {
			return err
		}
		m.settingsPresent, m.musicFilePresent = true, true
		return nil
	}
	if m.sfxFilePresent || len(m.SFX.SFX) != 0 {
		if err := assetsync.SaveSFX(m.Dir, &m.SFX); err != nil {
			return err
		}
		m.sfxFilePresent = true
	}
	if m.musicFilePresent || len(m.Music.Patterns) != 0 {
		if err := assetsync.SaveMusic(m.Dir, &m.Music, m.SFX); err != nil {
			return err
		}
		m.musicFilePresent = true
	}
	if m.sampleFilePresent || len(m.Samples.Samples) != 0 {
		if err := assetsync.SaveSamples(m.Dir, &m.Samples); err != nil {
			return err
		}
		m.sampleFilePresent = true
	}
	if m.settingsPresent || m.AudioSettings != assetsync.DefaultAudioSettings() {
		if err := assetsync.SaveAudioSettings(m.Dir, &m.AudioSettings); err != nil {
			return err
		}
		m.settingsPresent = true
	}
	return nil
}

// EnableNativeAudioV2 initializes the four-channel PSG asset model when a
// graphics-only project opens Music Lab for the first time. Real legacy audio
// is never overwritten or guessed: those projects must be converted
// explicitly because their software-mixer semantics have no lossless mapping
// to CH1–CH4.
func (m *Model) EnableNativeAudioV2() error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if m.AudioVersion == assetsync.NativeAudioSchemaVersion {
		return nil
	}
	if m.settingsPresent || m.sfxFilePresent || m.musicFilePresent || m.sampleFilePresent ||
		len(m.SFX.SFX) != 0 || len(m.Music.Patterns) != 0 || len(m.Samples.Samples) != 0 {
		return fmt.Errorf("legacy PICO/PCM audio is unsupported; convert explicitly to four-channel Game Boy audio v2")
	}
	settings := assetsync.DefaultNativeAudioSettings()
	sounds := assetsync.DefaultNativeSoundLibrary()
	music := assetsync.DefaultNativeMusicLibrary()
	library, err := assetsync.CompileNativeAudio(settings, sounds, music)
	if err != nil {
		return err
	}
	m.NativeAudio = assetsync.NativeAudioAssets{
		Settings: settings,
		Sounds:   sounds,
		Music:    music,
		Library:  library,
	}
	m.AudioVersion = assetsync.NativeAudioSchemaVersion
	return nil
}

// SetAudioMasterVolume changes the one editable measured-profile setting in
// memory. Save persists it atomically with the other editor documents.
func (m *Model) SetAudioMasterVolume(volume int) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	settings := m.AudioSettings
	settings.MasterVolume = volume
	if err := assetsync.ValidateAudioSettings(settings); err != nil {
		return err
	}
	m.AudioSettings = settings
	return nil
}

// Reload discards in-memory edits and loads the project from disk.
func (m *Model) Reload() error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	reloaded, err := Open(m.Dir)
	if err != nil {
		return err
	}
	*m = *reloaded
	return nil
}

// Preview is a native full-sheet preview. Pixels remain 8bpp indices while
// Colors contains the active palette expanded to all 256 possible indices.
type Preview struct {
	Width  int
	Height int
	Pixels []byte
	Colors [256]assetsync.RGB15
}

// Preview returns the current in-memory sheet rendered through the active
// palette. This makes palette edits visible to every sprite immediately.
func (m *Model) Preview() (*Preview, error) {
	if m == nil {
		return nil, fmt.Errorf("editor model is required")
	}
	assets, err := assetsync.Flatten(m.Project, m.Sheet, m.Map, m.ActivePalette)
	if err != nil {
		return nil, err
	}
	colors, err := assetsync.ExpandedPaletteColors(m.ActivePalette)
	if err != nil {
		return nil, err
	}
	return &Preview{
		Width:  assets.SheetW,
		Height: assets.SheetH,
		Pixels: slices.Clone(assets.SheetROM),
		Colors: colors,
	}, nil
}

// SelectPalette changes the project-wide active palette in memory and
// refreshes the model. The palette must already exist in the library; Save
// persists the project.json change.
func (m *Model) SelectPalette(name string) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	palette, err := assetsync.LoadPalette(m.Dir, name)
	if err != nil {
		return err
	}
	if _, err := assetsync.Flatten(m.Project, m.Sheet, m.Map, *palette); err != nil {
		return err
	}
	m.Project.ActivePalette = name
	m.ActivePalette = *palette
	return nil
}

// CreatePalette adds a reusable palette and refreshes the library list.
func (m *Model) CreatePalette(palette assetsync.Palette) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if err := assetsync.CreatePalette(m.Dir, palette); err != nil {
		return err
	}
	return m.refreshPalettes()
}

// DuplicatePalette copies a reusable palette under a new name.
func (m *Model) DuplicatePalette(sourceName, destinationName string) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if sourceName == m.ActivePalette.Name {
		// Duplicate the model's current palette, not the stale on-disk copy;
		// color edits intentionally remain in memory until Save.
		palette := m.ActivePalette
		palette.Name = destinationName
		if err := assetsync.CreatePalette(m.Dir, palette); err != nil {
			return err
		}
		return m.refreshPalettes()
	}
	if err := assetsync.DuplicatePalette(m.Dir, sourceName, destinationName); err != nil {
		return err
	}
	return m.refreshPalettes()
}

// RenamePalette renames a palette and preserves project-wide active selection.
// Palette-file CRUD is immediate, so an active rename also updates
// project.json immediately; otherwise that file would point at the removed
// old filename until a later Save.
func (m *Model) RenamePalette(oldName, newName string) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	wasActive := m.Project.ActivePalette == oldName
	var activePalette assetsync.Palette
	if wasActive {
		// Rename the model's current palette contents too; SetActiveColor is
		// deliberately in-memory until Save, so loading the old file here would
		// silently discard an unsaved color edit.
		activePalette = m.ActivePalette
		activePalette.Name = newName
	}
	if err := assetsync.RenamePaletteUnchecked(m.Dir, oldName, newName); err != nil {
		return err
	}
	if wasActive {
		if err := assetsync.SavePalette(m.Dir, &activePalette); err != nil {
			return err
		}
		m.Project.ActivePalette = newName
		m.ActivePalette = activePalette
		if err := assetsync.SaveProject(m.Dir, m.Project); err != nil {
			return err
		}
	}
	return m.refreshPalettes()
}

// DeletePalette removes a palette. The active-palette guard uses the
// model's in-memory selection (not assetsync.DeletePalette's on-disk check),
// since SelectPalette does not write project.json until Save; the built-in
// canonical palette is still rejected by DeletePaletteUnchecked.
func (m *Model) DeletePalette(name string) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if name == assetsync.CanonicalPaletteName {
		// Keep the built-in rejection deterministic even when it is also the
		// model's active palette.
		return assetsync.DeletePaletteUnchecked(m.Dir, name)
	}
	if m.Project.ActivePalette == name {
		return fmt.Errorf("cannot delete active palette %q; select another palette first", name)
	}
	// A previous in-memory SelectPalette may have left project.json naming
	// this now-inactive palette. Commit the model's current selection before
	// removing the file so a failed-to-Save editor session cannot leave a
	// dangling active-palette reference. Reject the canonical palette before
	// doing that side effect, preserving the public deletion contract.
	diskProject, err := assetsync.LoadProject(m.Dir)
	if err != nil {
		return err
	}
	if diskProject.ActivePalette == name {
		if err := assetsync.SaveProject(m.Dir, m.Project); err != nil {
			return err
		}
	}
	if err := assetsync.DeletePaletteUnchecked(m.Dir, name); err != nil {
		return err
	}
	return m.refreshPalettes()
}

// SetActiveColor edits one stored color in the active palette without writing
// immediately. For a 16-color shorthand only entries 0..15 are editable;
// indices 16..255 are aliases of color zero by contract. Save persists the
// edit so undo can restore memory before disk is touched.
func (m *Model) SetActiveColor(index int, color assetsync.RGB15) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if index < 0 || index >= len(m.ActivePalette.Colors) {
		return fmt.Errorf("palette color index %d is outside stored palette length %d", index, len(m.ActivePalette.Colors))
	}
	if err := color.Valid(); err != nil {
		return err
	}
	updated := m.ActivePalette
	updated.Colors = slices.Clone(updated.Colors)
	updated.Colors[index] = color
	m.ActivePalette = updated
	return nil
}

// SetSpritePixel edits a pixel in an 8x8 native sprite cell. If the cell has
// not been authored yet, it is created at its row-major sheet location. This
// uses the actual sheet dimensions, so 240x160 and larger project-defined
// sheets work at whatever size the project declares.
func (m *Model) SetSpritePixel(id, x, y, color int) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if id == ReservedSpriteID {
		return fmt.Errorf("sprite 0 is reserved as the transparent map eraser and cannot be edited")
	}
	cols, rows := m.Sheet.Width/8, m.Sheet.Height/8
	spriteN := cols * rows
	if id < 0 || id >= spriteN {
		return fmt.Errorf("sprite id %d is outside 0..%d", id, spriteN-1)
	}
	if x < 0 || x >= 8 || y < 0 || y >= 8 {
		return fmt.Errorf("sprite pixel (%d,%d) is outside 8x8 cell", x, y)
	}
	if color < 0 || color >= len(m.ActivePalette.Colors) {
		return fmt.Errorf("pixel color %d is outside active palette length %d", color, len(m.ActivePalette.Colors))
	}
	for i := range m.Sheet.Sprites {
		sprite := &m.Sheet.Sprites[i]
		if sprite.ID != id {
			continue
		}
		if sprite.X != (id%cols)*8 || sprite.Y != (id/cols)*8 || sprite.Width != 8 || sprite.Height != 8 {
			return fmt.Errorf("sprite %d is not an editable 8x8 cell", id)
		}
		sprite.Pixels[y][x] = color
		return nil
	}
	m.Sheet.Sprites = append(m.Sheet.Sprites, newCellSprite(id, id%cols*8, id/cols*8))
	m.Sheet.Sprites[len(m.Sheet.Sprites)-1].Pixels[y][x] = color
	return nil
}

// SetSpritePixelBlock atomically replaces a tile-aligned rectangle of sheet
// pixels. It is used by multi-tile imports, where a validation failure must
// not leave the first part of the image applied. Empty tiles remain sparse;
// existing flags are preserved on every overwritten tile.
func (m *Model) SetSpritePixelBlock(startCol, startRow, width, height int, pixels []int) (bool, error) {
	if m == nil {
		return false, fmt.Errorf("editor model is required")
	}
	if width <= 0 || height <= 0 || width%8 != 0 || height%8 != 0 {
		return false, fmt.Errorf("sprite pixel block must have positive dimensions divisible by 8 (got %dx%d)", width, height)
	}
	if len(pixels) != width*height {
		return false, fmt.Errorf("sprite pixel block has %d pixels (want %d)", len(pixels), width*height)
	}
	cols, rows := m.Sheet.Width/8, m.Sheet.Height/8
	tileCols, tileRows := width/8, height/8
	if startCol < 0 || startRow < 0 || startCol+tileCols > cols || startRow+tileRows > rows {
		return false, fmt.Errorf("%dx%d tile paste at (%d,%d) exceeds the %dx%d spritesheet", tileCols, tileRows, startCol, startRow, cols, rows)
	}
	for _, colorIndex := range pixels {
		if colorIndex < 0 || colorIndex >= len(m.ActivePalette.Colors) {
			return false, fmt.Errorf("pixel color %d is outside active palette length %d", colorIndex, len(m.ActivePalette.Colors))
		}
	}

	updated := cloneSpritesheet(m.Sheet)
	records := make(map[int]int, len(updated.Sprites))
	for i := range updated.Sprites {
		records[updated.Sprites[i].ID] = i
	}
	for tileY := 0; tileY < tileRows; tileY++ {
		for tileX := 0; tileX < tileCols; tileX++ {
			col, row := startCol+tileX, startRow+tileY
			id := row*cols + col
			if id == ReservedSpriteID {
				return false, fmt.Errorf("sprite 0 is reserved as the transparent map eraser and cannot be edited")
			}
			if index, ok := records[id]; ok {
				sprite := updated.Sprites[index]
				if sprite.X != col*8 || sprite.Y != row*8 || sprite.Width != 8 || sprite.Height != 8 || len(sprite.Pixels) != 8 {
					return false, fmt.Errorf("sprite %d is not an editable 8x8 cell", id)
				}
				for _, pixelRow := range sprite.Pixels {
					if len(pixelRow) != 8 {
						return false, fmt.Errorf("sprite %d is not an editable 8x8 cell", id)
					}
				}
			}
		}
	}

	changed := false
	for tileY := 0; tileY < tileRows; tileY++ {
		for tileX := 0; tileX < tileCols; tileX++ {
			col, row := startCol+tileX, startRow+tileY
			id := row*cols + col
			index, exists := records[id]
			if !exists && !pixelTileHasInk(pixels, width, tileX, tileY) {
				continue
			}
			if !exists {
				updated.Sprites = append(updated.Sprites, newCellSprite(id, col*8, row*8))
				index = len(updated.Sprites) - 1
				records[id] = index
			}
			sprite := &updated.Sprites[index]
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					value := pixels[(tileY*8+y)*width+tileX*8+x]
					if sprite.Pixels[y][x] != value {
						sprite.Pixels[y][x] = value
						changed = true
					}
				}
			}
		}
	}
	if changed {
		m.Sheet = updated
	}
	return changed, nil
}

func cloneSpritesheet(sheet assetsync.Spritesheet) assetsync.Spritesheet {
	cloned := sheet
	cloned.Sprites = slices.Clone(sheet.Sprites)
	for i := range cloned.Sprites {
		cloned.Sprites[i].Pixels = slices.Clone(sheet.Sprites[i].Pixels)
		for y := range cloned.Sprites[i].Pixels {
			cloned.Sprites[i].Pixels[y] = slices.Clone(sheet.Sprites[i].Pixels[y])
		}
	}
	return cloned
}

func pixelTileHasInk(pixels []int, width, tileX, tileY int) bool {
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if pixels[(tileY*8+y)*width+tileX*8+x] != 0 {
				return true
			}
		}
	}
	return false
}

// SpriteFlagByte returns sprite id's whole flag byte, or 0 if the sprite has
// never been authored (matching an unpainted sprite's implicit zero pixels).
func (m *Model) SpriteFlagByte(id int) int {
	if m == nil {
		return 0
	}
	for _, sprite := range m.Sheet.Sprites {
		if sprite.ID == id {
			return sprite.Flags
		}
	}
	return 0
}

// SetSpriteFlag sets or clears one flag bit (0..7) for sprite id, creating
// an empty 8x8 cell record first if the sprite has not been authored yet -
// mirroring SetSpritePixel's create-on-write behavior, since a flag can be
// meaningfully set on a sprite that has no painted pixels.
func (m *Model) SetSpriteFlag(id, bit int, value bool) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	cols, rows := m.Sheet.Width/8, m.Sheet.Height/8
	spriteN := cols * rows
	if id < 0 || id >= spriteN {
		return fmt.Errorf("sprite id %d is outside 0..%d", id, spriteN-1)
	}
	if bit < 0 || bit > 7 {
		return fmt.Errorf("sprite flag bit %d is outside 0..7", bit)
	}
	mask := 1 << bit
	for i := range m.Sheet.Sprites {
		sprite := &m.Sheet.Sprites[i]
		if sprite.ID != id {
			continue
		}
		if sprite.X != (id%cols)*8 || sprite.Y != (id/cols)*8 || sprite.Width != 8 || sprite.Height != 8 {
			return fmt.Errorf("sprite %d is not an editable 8x8 cell", id)
		}
		if value {
			sprite.Flags |= mask
		} else {
			sprite.Flags &^= mask
		}
		return nil
	}
	sprite := newCellSprite(id, id%cols*8, id/cols*8)
	if value {
		sprite.Flags |= mask
	}
	m.Sheet.Sprites = append(m.Sheet.Sprites, sprite)
	return nil
}

// ResizeMap changes the tile map's cell grid, copying the overlapping
// top-left region and filling new cells with 0 (the eraser sprite). There
// is no hardcoded 128x32/128x64 cap; size is project-defined.
func (m *Model) ResizeMap(width, height int) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if width < 1 || height < 1 {
		return fmt.Errorf("map dimensions must be positive, got %dx%d", width, height)
	}
	oldW, oldH := m.Map.Width, m.Map.Height
	if width == oldW && height == oldH {
		return nil
	}
	cells := make([]int, width*height)
	copyW := min(oldW, width)
	copyH := min(oldH, height)
	for y := 0; y < copyH; y++ {
		src := y * oldW
		dst := y * width
		copy(cells[dst:dst+copyW], m.Map.Cells[src:src+copyW])
	}
	resized := assetsync.Map{Width: width, Height: height, Cells: cells}
	if _, err := assetsync.Flatten(m.Project, m.Sheet, resized, m.ActivePalette); err != nil {
		return err
	}
	m.Map = resized
	return nil
}

func newCellSprite(id, x, y int) assetsync.SpriteRecord {
	pixels := make([][]int, 8)
	for row := range pixels {
		pixels[row] = make([]int, 8)
	}
	return assetsync.SpriteRecord{ID: id, X: x, Y: y, Width: 8, Height: 8, Pixels: pixels}
}

func (m *Model) refreshPalettes() error {
	names, err := assetsync.ListPalettes(m.Dir)
	if err != nil {
		return err
	}
	m.PaletteNames = names
	return nil
}
