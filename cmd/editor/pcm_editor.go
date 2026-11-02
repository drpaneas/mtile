package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/audioimport"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
	"github.com/drpaneas/mtile/internal/hostfiles"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type pcmEntry struct {
	Music bool
	ID    int
	Item  assetsync.DirectAudioItem
}

func (e pcmEntry) command() string {
	if e.Music {
		return fmt.Sprintf("pcm_music(%d)", e.ID)
	}
	return fmt.Sprintf("sample(%d)", e.ID)
}

type pcmJobResult struct {
	kind, path     string
	keepName       bool
	prepared       audioimport.Prepared
	result         audioimport.Result
	err            error
	pcm            []byte
	rate           int
	previewSource  string
	paths          []string
	batch          *audioimport.Batch
	index          int
	phase, problem string
}
type pcmEditorState struct {
	open                          bool
	phase                         string // choose, review, asset, done
	path, name, message, command  string
	focus                         int // 1 path, 2 name
	quality                       audioimport.Quality
	trimSilence                   bool
	previewSource                 string
	previewPlaying                bool
	prepared                      audioimport.Prepared
	entries                       []pcmEntry
	selected, scroll              int
	bankBytes                     int
	converter                     string
	confirmDelete, confirmReplace bool
	busy                          string
	job                           <-chan pcmJobResult
	batch                         *audioimport.Batch
	batchIndex, batchScroll       int
	batchCommands                 []string
}
type pcmLayout struct {
	Panel, Close, Converter, List, Add, Body, Path, Browse, Prepare, Name, Preview, Original, Advice, Trim, Commit, Copy, CopyAll, RemoveQueued, Delete editor.Rect
	Profiles                                                                                                                                            [3]editor.Rect
}

func newPCMLayout() pcmLayout {
	p := editor.Rect{W: min(1320, screenWidth-32), H: min(800, screenHeight-72)}
	p.X, p.Y = (screenWidth-p.W)/2, (screenHeight-p.H)/2
	l := pcmLayout{Panel: p}
	l.Close = editor.Rect{X: p.X + p.W - 108, Y: p.Y + 18, W: 88, H: 32}
	l.Converter = editor.Rect{X: l.Close.X - 144, Y: l.Close.Y, W: 132, H: 32}
	l.List = editor.Rect{X: p.X + 20, Y: p.Y + 82, W: min(280, p.W/3), H: p.H - 190}
	l.Add = editor.Rect{X: l.List.X, Y: p.Y + p.H - 60, W: l.List.W, H: 36}
	l.Body = editor.Rect{X: l.List.X + l.List.W + 24, Y: l.List.Y, W: p.W - l.List.W - 64, H: p.H - 158}
	b := l.Body
	l.Path = editor.Rect{X: b.X, Y: b.Y + 66, W: b.W - 122, H: 36}
	l.Browse = editor.Rect{X: b.X + b.W - 112, Y: b.Y + 66, W: 112, H: 36}
	l.Prepare = editor.Rect{X: b.X, Y: b.Y + 120, W: 180, H: 38}
	for i := range l.Profiles {
		l.Profiles[i] = editor.Rect{X: b.X, Y: b.Y + 120 + i*50, W: b.W, H: 44}
	}
	l.Name = editor.Rect{X: b.X, Y: b.Y + 302, W: b.W, H: 36}
	l.Advice = editor.Rect{X: b.X, Y: b.Y + 390, W: b.W, H: 116}
	l.Trim = editor.Rect{X: b.X, Y: b.Y + 518, W: b.W, H: 32}
	y := p.Y + p.H - 60
	l.Preview = editor.Rect{X: b.X, Y: y, W: 142, H: 36}
	l.Original = editor.Rect{X: b.X + 152, Y: y, W: 142, H: 36}
	l.Commit = editor.Rect{X: b.X + b.W - 208, Y: y, W: 208, H: 36}
	l.Copy = editor.Rect{X: b.X, Y: b.Y + 164, W: 180, H: 36}
	l.CopyAll = editor.Rect{X: b.X + 192, Y: l.Copy.Y, W: 180, H: 36}
	l.RemoveQueued = editor.Rect{X: l.List.X + l.List.W - 96, Y: l.List.Y + 6, W: 88, H: 28}
	l.Delete = editor.Rect{X: b.X + b.W - 208, Y: y, W: 208, H: 36}
	return l
}
func pcmButtonRect() editor.Rect {
	f := fontButtonRect()
	return editor.Rect{X: f.X + f.W + 8, Y: f.Y, W: 86, H: f.H}
}
func (g *Game) openPCMEditor() {
	g.stopAudioPreview()
	g.setNativeArmed(false)
	g.developGameFocus = false
	g.pcm.open = true
	if g.pcm.quality == "" {
		g.pcm.quality = audioimport.QualityBalanced
	}
	g.pcm.phase, g.pcm.focus = "choose", 0
	g.pcm.confirmDelete, g.pcm.confirmReplace = false, false
	g.pcm.message = ""
	g.pcm.previewPlaying = false
	g.reloadPCMLibrary()
	g.requestRender()
}
func (g *Game) reloadPCMLibrary() {
	s := &g.pcm
	a, err := assetsync.LoadDirectAudio(g.model.Dir)
	s.entries, s.bankBytes = nil, 0
	if err != nil {
		s.message = err.Error()
		return
	}
	if a == nil {
		return
	}
	s.bankBytes = len(a.Bank)
	for group, items := range [][]assetsync.DirectAudioItem{a.Manifest.Music, a.Manifest.Samples} {
		for id, item := range items {
			if !item.Deleted {
				s.entries = append(s.entries, pcmEntry{Music: group == 0, ID: id, Item: item})
			}
		}
	}
	s.scroll = min(s.scroll, max(0, len(s.entries)-1))
}
func (g *Game) pcmStartJob(label string, run func() pcmJobResult) {
	g.pcmStartProgressJob(label, func(func(string)) pcmJobResult { return run() })
}
func (g *Game) pcmStartProgressJob(label string, run func(func(string)) pcmJobResult) {
	if g.pcm.job != nil {
		return
	}
	g.stopAudioPreview()
	g.pcm.previewPlaying = false
	ch := make(chan pcmJobResult, 2)
	g.pcm.job, g.pcm.busy, g.pcm.message = ch, label, ""
	go func() {
		progress := func(message string) {
			select {
			case ch <- pcmJobResult{kind: "progress", problem: message}:
			default:
			}
		}
		ch <- run(progress)
	}()
	g.requestRender()
}
func (g *Game) pcmPrepare(path string) {
	keepName := g.pcm.phase == "review" && g.pcm.path == path
	if !keepName {
		g.pcm.trimSilence = false
	}
	opts := audioimport.PrepareOptions{Quality: g.pcm.quality, TrimSilence: g.pcm.trimSilence}
	g.pcm.path = path
	g.pcmStartJob("Preparing audio…", func() pcmJobResult {
		p, err := audioimport.PrepareWithOptions(path, opts)
		return pcmJobResult{kind: "prepare", prepared: p, keepName: keepName, err: err}
	})
}
func (g *Game) pcmPollJob() {
	s := &g.pcm
	if s.job == nil {
		return
	}
	select {
	case result := <-s.job:
		if result.kind == "progress" {
			s.busy = result.problem
			g.requestRender()
			return
		}
		s.job, s.busy = nil, ""
		if result.err != nil {
			s.message = result.err.Error()
			if result.kind == "prepare" && s.phase == "review" {
				s.quality = s.prepared.Quality
				s.trimSilence = s.prepared.Trimmed
			}
			if result.kind == "batch_prepare" && s.batch != nil {
				s.quality = s.batch.Options.Quality
				s.trimSilence = s.batch.Options.TrimSilence
			}
			g.requestRender()
			return
		}
		switch result.kind {
		case "batch_prepare":
			g.pcmAcceptBatch(result)
		case "batch_select":
			g.pcmAcceptBatchSelection(result)
		case "audition":
			g.pcmPlayPreview(result.pcm, result.rate, result.previewSource)
		case "converter":
			if result.path != "" {
				s.converter = result.path
				s.message = "Converter selected for this session. Choose IMPORT + SAVE to retry."
			}
		case "browse":
			if len(result.paths) > 0 {
				g.pcmChooseFiles(result.paths, s.batch != nil && s.phase == "review")
			} else if result.path != "" {
				s.focus = 0
				g.pcmPrepare(result.path)
			}
		case "prepare":
			s.prepared = result.prepared
			s.trimSilence = result.prepared.Trimmed
			if !result.keepName {
				s.name = result.prepared.Name
			}
			s.phase, s.focus = "review", 0
			s.confirmReplace = false
		case "asset":
			s.prepared = result.prepared
			s.phase = "asset"
		case "import", "batch_import":
			s.phase = "done"
			items := result.result.Manifest.Samples
			if s.prepared.Music {
				items = result.result.Manifest.Music
			}
			for id, item := range items {
				if item.Name == s.name && !item.Deleted {
					s.command = pcmEntry{Music: s.prepared.Music, ID: id}.command()
					break
				}
			}
			s.message = "Imported and saved. Copy the Lua call, then Build + Reload to hear it in your game."
			if result.kind == "batch_import" {
				g.pcmBatchImported(result.result.Manifest)
			}
			// Surface real converter warnings; never hide truncated/unsupported tracker features.
			for _, line := range strings.Split(result.result.ConverterOutput, "\n") {
				lower := strings.ToLower(line)
				if strings.Contains(lower, "warning") || strings.Contains(lower, "unsupported") || strings.Contains(lower, "truncat") {
					s.message += " " + strings.TrimSpace(line)
				}
			}
			g.reloadPCMLibrary()
			g.queueProjectCheck()
		case "delete":
			s.phase = "choose"
			s.confirmDelete = false
			s.command = ""
			s.message = "Deleted and saved. Other IDs are unchanged. Update any Lua calls to the deleted ID."
			if result.result.ConverterOutput != "" {
				for _, line := range strings.Split(result.result.ConverterOutput, "\n") {
					if strings.Contains(strings.ToLower(line), "warning") {
						s.message += " " + strings.TrimSpace(line)
					}
				}
			}
			g.reloadPCMLibrary()
			g.queueProjectCheck()
		}
		g.requestRender()
	default:
	}
}
func (g *Game) pcmSelectEntry(index int) {
	if index < 0 || index >= len(g.pcm.entries) {
		return
	}
	e := g.pcm.entries[index]
	path := filepath.Join(g.model.Dir, "assets/audio/direct/source", e.Item.Source)
	g.pcm.selected = index
	g.pcm.command = e.command()
	g.pcm.name = e.Item.Name
	g.pcm.confirmDelete = false
	g.pcm.phase = "asset"
	g.pcm.prepared = audioimport.Prepared{Music: e.Music, Name: e.Item.Name}
	g.pcmStartJob("Reading asset…", func() pcmJobResult {
		p, err := audioimport.Prepare(path, audioimport.QualitySource)
		return pcmJobResult{kind: "asset", prepared: p, err: err}
	})
}
func (g *Game) pcmImport() {
	s := &g.pcm
	if s.phase != "review" || s.job != nil {
		return
	}
	if s.batch != nil {
		g.pcmImportBatch()
		return
	}
	for _, e := range s.entries {
		if e.Music == s.prepared.Music && e.Item.Name == s.name && !s.confirmReplace {
			s.confirmReplace = true
			s.message = "This name already exists. Import again to replace its audio and keep the same Lua ID."
			g.requestRender()
			return
		}
	}
	project, p, name, converter := g.model.Dir, s.prepared, s.name, s.converter
	g.pcmStartJob("Converting and saving bank…", func() pcmJobResult {
		r, err := audioimport.ImportPrepared(project, p, name, converter)
		return pcmJobResult{kind: "import", result: r, err: err}
	})
}
func (g *Game) pcmDelete() {
	s := &g.pcm
	if s.phase != "asset" || s.selected < 0 || s.selected >= len(s.entries) {
		return
	}
	if !s.confirmDelete {
		s.confirmDelete = true
		s.message = "Delete " + s.command + "? This removes its project audio. Existing Lua references must be updated. Click CONFIRM DELETE to continue."
		g.requestRender()
		return
	}
	project, e, converter := g.model.Dir, s.entries[s.selected], s.converter
	g.pcmStartJob("Rebuilding bank without this asset…", func() pcmJobResult {
		r, err := audioimport.Delete(project, e.Music, e.ID, converter)
		return pcmJobResult{kind: "delete", result: r, err: err}
	})
}
func (g *Game) pcmSetQuality(quality audioimport.Quality) {
	g.pcm.quality = quality
	if quality == audioimport.QualitySource {
		g.pcm.trimSilence = false
	}
	if g.pcm.batch != nil {
		g.pcmReprepareBatch()
		return
	}
	g.pcmPrepare(g.pcm.path)
}
func (g *Game) pcmToggleTrim() {
	s := &g.pcm
	if s.phase != "review" || (!s.prepared.Analysis.CanTrim() && (s.batch == nil || s.batch.TrimEligibleCount() == 0)) {
		return
	}
	if s.quality == audioimport.QualitySource {
		s.message = "Choose Balanced or Smallest to trim quiet edges. Keep source preserves timing and loop metadata."
		g.requestRender()
		return
	}
	s.trimSilence = !s.trimSilence
	if s.batch != nil {
		g.pcmReprepareBatch()
		return
	}
	g.pcmPrepare(s.path)
}
func (g *Game) pcmAudition(original bool) {
	s := &g.pcm
	if s.prepared.Music {
		return
	}
	if !original && len(s.prepared.Data) == 0 || original && s.prepared.Analysis.Info.Rate <= 0 {
		return
	}
	variant := "prepared"
	if original {
		variant = "original"
	}
	if g.previewPlayer != nil && g.previewPlayer.IsPlaying() && s.previewSource == variant {
		g.stopAudioPreview()
		s.previewPlaying = false
		g.requestRender()
		return
	}
	p, path := s.prepared, s.path
	g.pcmStartJob("Preparing audition…", func() pcmJobResult {
		var pcm []byte
		var rate int
		var err error
		if original {
			pcm, rate, err = audioimport.PreviewFile(path)
		} else {
			pcm, rate, err = p.PreviewPCM()
		}
		return pcmJobResult{kind: "audition", pcm: pcm, rate: rate, previewSource: variant, err: err}
	})
}
func (g *Game) pcmPlayPreview(pcm []byte, rate int, variant string) {
	context, err := editorAudioContext()
	if err == nil {
		g.previewPlayer, err = newEditorAudioPlayerAtRate(context, pcm, rate)
	}
	if err != nil {
		g.pcm.message = err.Error()
		return
	}
	g.setPreviewPlayerVolume(g.previewPlayer)
	g.previewPlayer.Play()
	g.pcm.previewSource, g.pcm.previewPlaying = variant, true
	g.pcm.message = "Playing " + variant + " WAV (up to 15 seconds). Build + Reload checks the complete cartridge mix."
	g.requestRender()
}
func (g *Game) updatePCMEditor() {
	g.pcmPollJob()
	s := &g.pcm
	if s.previewPlaying && (g.previewPlayer == nil || !g.previewPlayer.IsPlaying()) {
		s.previewPlaying = false
		g.requestRender()
	}
	if s.job != nil {
		return
	}
	l := newPCMLayout()
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if s.confirmDelete || s.confirmReplace {
			s.confirmDelete, s.confirmReplace = false, false
			s.message = ""
		} else {
			g.closePCMEditor()
		}
		g.requestRender()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		if s.phase == "review" {
			s.focus = 2
		} else {
			s.focus = 1
		}
	}
	if s.focus != 0 {
		target := &s.path
		if s.focus == 2 {
			target = &s.name
		}
		mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
		typed := string(ebiten.AppendInputChars(nil))
		if mod && inpututil.IsKeyJustPressed(ebiten.KeyV) {
			if text, err := hostclipboard.Read(); err == nil {
				typed = text
			}
		}
		if s.focus == 1 && strings.Contains(typed, "\n") {
			s.path = typed
			typed = ""
		}
		if mod && inpututil.IsKeyJustPressed(ebiten.KeyA) {
			*target = ""
		}
		for _, ch := range typed {
			if !unicode.IsControl(ch) && len(*target) < 2048 {
				*target += string(ch)
			}
		}
		if codeKeyRepeat(ebiten.KeyBackspace) && *target != "" {
			_, n := utf8.DecodeLastRuneInString(*target)
			*target = (*target)[:len(*target)-n]
		}
		if s.focus == 2 {
			s.confirmReplace = false
			g.pcmSaveBatchName()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			if s.focus == 1 {
				g.pcmChooseFiles(pcmPathLines(s.path), false)
			} else {
				g.pcmImport()
			}
			return
		}
	}
	mx, my := editorCursorPosition()
	if rectContains(l.List, mx, my) {
		_, wheel := ebiten.Wheel()
		visible := max(1, (l.List.H-52)/54)
		if s.batch != nil {
			s.batchScroll = max(0, min(max(0, len(s.batch.Items)-visible), s.batchScroll-int(wheel)))
		} else {
			s.scroll = max(0, min(max(0, len(s.entries)-visible), s.scroll-int(wheel)))
		}
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	s.focus = 0
	switch {
	case s.batch != nil && s.phase == "review" && rectContains(l.RemoveQueued, mx, my):
		g.pcmRemoveQueued()
	case rectContains(l.Converter, mx, my):
		g.pcmStartJob("Choose the mmutil executable…", func() pcmJobResult {
			path, err := hostfiles.ChooseConverter()
			return pcmJobResult{kind: "converter", path: path, err: err}
		})
	case rectContains(l.Close, mx, my):
		g.closePCMEditor()
	case rectContains(l.Add, mx, my):
		if s.batch != nil && s.phase == "review" {
			g.pcmBrowseFiles()
			break
		}
		g.pcmDiscardBatch()
		g.stopAudioPreview()
		s.phase = "choose"
		s.message = ""
		s.confirmDelete = false
	case rectContains(l.List, mx, my):
		if s.batch != nil {
			if my >= l.List.Y+44 {
				g.pcmSelectBatch(s.batchScroll + (my-l.List.Y-44)/54)
			}
			break
		}
		index := s.scroll + (my-l.List.Y-44)/54
		if my >= l.List.Y+44 {
			g.pcmSelectEntry(index)
		}
	case s.phase == "choose" && rectContains(l.Path, mx, my):
		s.focus = 1
	case s.phase == "choose" && rectContains(l.Browse, mx, my):
		g.pcmBrowseFiles()
	case s.phase == "choose" && rectContains(l.Prepare, mx, my):
		g.pcmChooseFiles(pcmPathLines(s.path), false)
	case s.phase == "review" && rectContains(l.Name, mx, my):
		s.focus = 2
	case s.phase == "review" && rectContains(l.Commit, mx, my):
		g.pcmImport()
	case (s.phase == "asset" || s.phase == "review") && rectContains(l.Preview, mx, my):
		g.pcmAudition(false)
	case s.phase == "review" && rectContains(l.Original, mx, my):
		g.pcmAudition(true)
	case s.phase == "review" && rectContains(l.Trim, mx, my):
		g.pcmToggleTrim()
	case s.phase == "asset" && rectContains(l.Delete, mx, my):
		g.pcmDelete()
	case s.batch != nil && s.phase == "done" && rectContains(l.CopyAll, mx, my):
		g.pcmCopyAll()
	case (s.phase == "asset" || s.phase == "done") && rectContains(l.Copy, mx, my):
		if err := hostclipboard.Write(s.command); err != nil {
			s.message = err.Error()
		} else {
			s.message = "Copied: " + s.command
		}
	default:
		if s.phase == "review" && !s.prepared.Music {
			for i, r := range l.Profiles {
				if rectContains(r, mx, my) {
					g.pcmSetQuality([]audioimport.Quality{audioimport.QualityBalanced, audioimport.QualitySmall, audioimport.QualitySource}[i])
					break
				}
			}
		}
	}
	g.requestRender()
}
