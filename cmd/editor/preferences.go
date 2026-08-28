package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const editorPreferencesVersion = 2

type interfaceTextSize uint8

const (
	textCompact interfaceTextSize = iota
	textComfort
	textLarge
	interfaceTextSizeCount
)

func (s interfaceTextSize) String() string {
	switch s {
	case textCompact:
		return "COMPACT"
	case textLarge:
		return "LARGE"
	default:
		return "COMFORT"
	}
}

type editorPreferences struct {
	Version             int               `json:"version"`
	Effects             effectLevel       `json:"effects"`
	LCDPreview          bool              `json:"lcdPreview"`
	PreviewVolume       int               `json:"previewVolume"`
	AutoSaveBeforeBuild bool              `json:"autoSaveBeforeBuild"`
	TextSize            interfaceTextSize `json:"textSize"`
}

func defaultEditorPreferences() editorPreferences {
	return editorPreferences{
		Version: editorPreferencesVersion, Effects: effectsSoft,
		LCDPreview: true, PreviewVolume: 80, AutoSaveBeforeBuild: true, TextSize: textComfort,
	}
}

func normalizeEditorPreferences(preferences editorPreferences) editorPreferences {
	if preferences.Version == 1 {
		preferences.Version = editorPreferencesVersion
		preferences.TextSize = textComfort
	}
	if preferences.Version != editorPreferencesVersion {
		return defaultEditorPreferences()
	}
	if preferences.Effects >= effectLevelCount {
		preferences.Effects = effectsSoft
	}
	preferences.PreviewVolume = min(max(preferences.PreviewVolume, 0), 100)
	if preferences.TextSize >= interfaceTextSizeCount {
		preferences.TextSize = textComfort
	}
	return preferences
}

func loadEditorPreferences(path string) (editorPreferences, error) {
	preferences := defaultEditorPreferences()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return preferences, nil
	}
	if err != nil {
		return preferences, err
	}
	if err := json.Unmarshal(data, &preferences); err != nil {
		return defaultEditorPreferences(), fmt.Errorf("decode preferences: %w", err)
	}
	return normalizeEditorPreferences(preferences), nil
}

func saveEditorPreferences(path string, preferences editorPreferences) error {
	preferences = normalizeEditorPreferences(preferences)
	data, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func editorPreferencesPath() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "mtile", "editor.json"), nil
}

func (g *Game) loadPreferences() {
	g.preferences = defaultEditorPreferences()
	path, err := editorPreferencesPath()
	if err == nil {
		g.preferences, err = loadEditorPreferences(path)
	}
	if err != nil {
		g.statusMsg = "preferences: " + err.Error()
	}
	g.applyPreferences()
}

func (g *Game) savePreferences() {
	g.preferences = normalizeEditorPreferences(g.preferences)
	path, err := editorPreferencesPath()
	if err == nil {
		err = saveEditorPreferences(path, g.preferences)
	}
	if err != nil {
		g.statusMsg = "save preferences failed: " + err.Error()
		return
	}
	g.applyPreferences()
}

func (g *Game) applyPreferences() {
	g.preferences = normalizeEditorPreferences(g.preferences)
	g.theme.level = g.preferences.Effects
	g.theme.playLCD = g.preferences.LCDPreview
	setInterfaceTextSize(g.preferences.TextSize)
	g.applyPreviewVolume()
}

func (g *Game) applyPreviewVolume() {
	volume := float64(g.preferences.PreviewVolume) / 100
	if g.previewPlayer != nil {
		g.previewPlayer.SetVolume(volume)
	}
	if g.nativeLivePlayer != nil {
		g.nativeLivePlayer.SetVolume(volume)
	}
	if g.nativeClickPlayer != nil {
		g.nativeClickPlayer.SetVolume(volume)
	}
	if g.playAudio != nil {
		g.playAudio.SetVolume(volume)
	}
}

// ebitenAudioPlayer is the small common surface used by every Ebitengine
// audio player owned by the editor. The adapter keeps preference code easy to
// test without creating a platform audio context.
type ebitenAudioPlayer interface {
	SetVolume(float64)
}

func (g *Game) setPreviewPlayerVolume(player ebitenAudioPlayer) {
	if player != nil {
		player.SetVolume(float64(g.preferences.PreviewVolume) / 100)
	}
}

type preferencesLayout struct {
	Shade, Panel, Close, Defaults editor.Rect
	Rows                          [5]editor.Rect
	Minus                         [5]editor.Rect
	Plus                          [5]editor.Rect
}

func newPreferencesLayout() preferencesLayout {
	panel := editor.Rect{W: min(760, screenWidth-80), H: min(580, screenHeight-100)}
	panel.X, panel.Y = (screenWidth-panel.W)/2, (screenHeight-panel.H)/2
	layout := preferencesLayout{
		Shade: editor.Rect{X: 0, Y: 0, W: screenWidth, H: screenHeight}, Panel: panel,
		Close:    editor.Rect{X: panel.X + panel.W - 122, Y: panel.Y + 18, W: 96, H: 34},
		Defaults: editor.Rect{X: panel.X + 24, Y: panel.Y + panel.H - 58, W: 176, H: 34},
	}
	for index := range layout.Rows {
		row := editor.Rect{X: panel.X + 24, Y: panel.Y + 88 + index*76, W: panel.W - 48, H: 64}
		layout.Rows[index] = row
		layout.Minus[index] = editor.Rect{X: row.X + row.W - 212, Y: row.Y + 14, W: 42, H: 36}
		layout.Plus[index] = editor.Rect{X: row.X + row.W - 42, Y: row.Y + 14, W: 42, H: 36}
	}
	return layout
}

func (g *Game) updatePreferences() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.preferencesOpen = false
		return
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	layout := newPreferencesLayout()
	mx, my := ebiten.CursorPosition()
	if rectContains(layout.Close, mx, my) || !rectContains(layout.Panel, mx, my) {
		g.preferencesOpen = false
		return
	}
	if rectContains(layout.Defaults, mx, my) {
		g.preferences = defaultEditorPreferences()
		g.savePreferences()
		g.statusMsg = "preferences restored"
		return
	}
	for index := range layout.Rows {
		delta := 0
		if rectContains(layout.Minus[index], mx, my) {
			delta = -1
		} else if rectContains(layout.Plus[index], mx, my) {
			delta = 1
		}
		if delta == 0 {
			continue
		}
		switch index {
		case 0:
			g.preferences.Effects = effectLevel((int(g.preferences.Effects) + delta + int(effectLevelCount)) % int(effectLevelCount))
		case 1:
			g.preferences.LCDPreview = !g.preferences.LCDPreview
		case 2:
			g.preferences.PreviewVolume = min(max(g.preferences.PreviewVolume+delta*10, 0), 100)
		case 3:
			g.preferences.AutoSaveBeforeBuild = !g.preferences.AutoSaveBeforeBuild
		case 4:
			g.preferences.TextSize = interfaceTextSize((int(g.preferences.TextSize) + delta + int(interfaceTextSizeCount)) % int(interfaceTextSizeCount))
		}
		g.savePreferences()
		return
	}
}

func (g *Game) drawPreferences(screen *ebiten.Image) {
	layout := newPreferencesLayout()
	fillRect(screen, layout.Shade, color.RGBA{R: 4, G: 5, B: 14, A: 214})
	drawHardwarePanel(screen, layout.Panel, color.RGBA{R: 23, G: 22, B: 52, A: 255}, colorCode)
	drawTextScale(screen, "PREFERENCES", layout.Panel.X+24, layout.Panel.Y+12, 2, colorText)
	drawText(screen, "Real editor behaviour — saved for every project", layout.Panel.X+24, layout.Panel.Y+50, colorDim)
	drawNativeButton(screen, layout.Close, "DONE", false, colorCode)

	labels := [...]string{"VISUAL EFFECTS", "GBA LCD TREATMENT", "PREVIEW AUDIO", "AUTOSAVE BEFORE BUILD", "INTERFACE TEXT"}
	details := [...]string{
		"Motion and shell lighting; work surfaces remain still.",
		"Dot-matrix shader on the embedded mGBA screen.",
		"Music, note audition, metronome, and game preview volume.",
		"OFF refuses dirty builds until source files are saved.",
		"Readable editor chrome and source text; COMFORT is the default.",
	}
	values := [...]string{
		g.preferences.Effects.String(),
		map[bool]string{false: "OFF", true: "ON"}[g.preferences.LCDPreview],
		fmt.Sprintf("%d%%", g.preferences.PreviewVolume),
		map[bool]string{false: "OFF", true: "ON"}[g.preferences.AutoSaveBeforeBuild],
		g.preferences.TextSize.String(),
	}
	for index, row := range layout.Rows {
		fillRoundedRect(screen, row, 6, color.RGBA{R: 18, G: 19, B: 44, A: 255})
		strokeRoundedRect(screen, row, 6, colorBorder)
		drawText(screen, labels[index], row.X+14, row.Y+6, colorText)
		detailWidth := layout.Minus[index].X - row.X - 28
		drawText(screen, truncateUIText(details[index], detailWidth), row.X+14, row.Y+35, colorDim)
		drawNativeButton(screen, layout.Minus[index], "-", false, colorCode)
		valueX := layout.Minus[index].X + layout.Minus[index].W
		valueW := layout.Plus[index].X - valueX
		valueRect := editor.Rect{X: valueX, Y: row.Y + 14, W: valueW, H: 36}
		fillRect(screen, valueRect, colorBackground)
		strokeRect(screen, valueRect, colorBorder)
		drawTextCentered(screen, values[index], valueRect, colorLCD)
		drawNativeButton(screen, layout.Plus[index], "+", false, colorCode)
	}
	drawNativeButton(screen, layout.Defaults, "RESTORE DEFAULTS", false, colorHighlight)
	drawText(screen, "F6 cycles visual effects  /  Esc closes", layout.Panel.X+layout.Panel.W-334, layout.Panel.Y+layout.Panel.H-45, colorDim)
}
