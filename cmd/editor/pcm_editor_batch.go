package main

import (
	"fmt"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/audioimport"
	"github.com/drpaneas/mtile/internal/hostclipboard"
	"github.com/drpaneas/mtile/internal/hostfiles"
)

func pcmPathLines(text string) []string {
	var paths []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.Trim(strings.TrimSpace(line), "\"")
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}
func (g *Game) pcmBrowseFiles() {
	g.pcmStartJob("Select one or more audio files…", func() pcmJobResult {
		paths, err := hostfiles.ChooseAudioFiles()
		return pcmJobResult{kind: "browse", paths: paths, err: err}
	})
}
func (g *Game) pcmDiscardBatch() <-chan struct{} {
	batch, job := g.pcm.batch, g.pcm.job
	g.pcm.batch = nil
	g.pcm.batchCommands = nil
	g.pcm.batchIndex, g.pcm.batchScroll = 0, 0
	if job == nil {
		if batch != nil {
			_ = batch.Close()
		}
		return nil
	}
	// Normal modal closing is blocked while busy. Window shutdown can still
	// arrive during a job: transfer workspace ownership to its result consumer,
	// instead of removing files while the worker reads them. Do not block quit.
	g.pcm.job = nil
	g.pcm.busy = ""
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			result := <-job
			if result.kind == "progress" {
				continue
			}
			if result.batch != nil && result.batch != batch {
				_ = result.batch.Close()
			}
			break
		}
		if batch != nil {
			_ = batch.Close()
		}
	}()
	return done
}
func (g *Game) closePCMEditor() {
	g.stopAudioPreview()
	g.pcmDiscardBatch()
	g.pcm.prepared = audioimport.Prepared{}
	g.pcm.open = false
	g.requestRender()
}
func (g *Game) pcmSaveBatchName() {
	s := &g.pcm
	if s.batch != nil && s.phase == "review" && s.batchIndex >= 0 && s.batchIndex < len(s.batch.Items) {
		s.batch.Items[s.batchIndex].Name = s.name
	}
}
func (g *Game) pcmBatchPathsAndNames() ([]string, []string) {
	g.pcmSaveBatchName()
	var paths, names []string
	if g.pcm.batch != nil {
		for _, item := range g.pcm.batch.Items {
			paths = append(paths, item.Path)
			names = append(names, item.Name)
		}
	}
	return paths, names
}
func (g *Game) pcmChooseFiles(paths []string, appendToBatch bool) {
	if len(paths) == 0 {
		g.pcm.message = "Choose at least one audio file."
		g.requestRender()
		return
	}
	if !appendToBatch {
		g.pcm.trimSilence = false
	}
	if len(paths) == 1 && !appendToBatch {
		g.pcmDiscardBatch()
		g.pcm.phase = "choose"
		g.pcmPrepare(paths[0])
		return
	}
	var names []string
	selected := 0
	if appendToBatch && g.pcm.batch != nil {
		old, oldNames := g.pcmBatchPathsAndNames()
		selected = g.pcm.batchIndex
		paths = append(old, paths...)
		names = append(oldNames, make([]string, len(paths)-len(old))...)
	}
	g.pcmPrepareBatch(paths, names, selected)
}
func (g *Game) pcmPrepareBatch(paths, names []string, selected int) {
	opts := audioimport.PrepareOptions{Quality: g.pcm.quality, TrimSilence: g.pcm.trimSilence}
	g.pcmStartProgressJob("Preparing selected files…", func(progress func(string)) pcmJobResult {
		b, err := audioimport.PrepareBatch(paths, names, opts, progress)
		if err != nil {
			return pcmJobResult{kind: "batch_prepare", err: err}
		}
		selected = max(0, min(selected, len(b.Items)-1))
		p, loadErr := b.Load(selected)
		problem := ""
		if loadErr != nil {
			problem = loadErr.Error()
		}
		return pcmJobResult{kind: "batch_prepare", batch: b, index: selected, prepared: p, problem: problem, phase: "review"}
	})
}
func (g *Game) pcmReprepareBatch() {
	paths, names := g.pcmBatchPathsAndNames()
	g.pcmPrepareBatch(paths, names, g.pcm.batchIndex)
}
func (g *Game) pcmAcceptBatch(result pcmJobResult) {
	g.pcmDiscardBatch()
	g.pcm.batch = result.batch
	g.pcm.quality = result.batch.Options.Quality
	g.pcm.trimSilence = result.batch.Options.TrimSilence && g.pcm.quality != audioimport.QualitySource
	g.pcmAcceptBatchSelection(result)
	if n := result.batch.ErrorCount(); n > 0 {
		g.pcm.message = fmt.Sprintf("%d file(s) need attention. Select a failed row to inspect it, choose another shared preset, or REMOVE it. The project has not changed.", n)
	} else {
		g.pcm.message = fmt.Sprintf("%d files ready. Quality and optional trim apply to all eligible WAVs. Review each file, then import the complete batch once.", len(result.batch.Items))
	}
}
func (g *Game) pcmAcceptBatchSelection(result pcmJobResult) {
	s := &g.pcm
	if s.batch == nil || result.index < 0 || result.index >= len(s.batch.Items) {
		return
	}
	item := &s.batch.Items[result.index]
	s.batchIndex = result.index
	visible := max(1, (newPCMLayout().List.H-52)/54)
	s.batchScroll = max(0, min(max(0, len(s.batch.Items)-visible), s.batchScroll))
	if s.batchIndex < s.batchScroll {
		s.batchScroll = s.batchIndex
	}
	if s.batchIndex >= s.batchScroll+visible {
		s.batchScroll = s.batchIndex - visible + 1
	}
	s.prepared = result.prepared
	s.path = item.Path
	s.name = item.Name
	s.phase = result.phase
	s.focus = 0
	s.confirmReplace = false
	if result.problem != "" {
		s.message = result.problem
		item.Error = result.problem
	} else {
		s.message = ""
	}
	if s.phase == "done" && result.index < len(s.batchCommands) {
		s.command = s.batchCommands[result.index]
	}
}
func (g *Game) pcmSelectBatch(index int) {
	s := &g.pcm
	if s.batch == nil || index < 0 || index >= len(s.batch.Items) {
		return
	}
	g.pcmSaveBatchName()
	b, phase := s.batch, s.phase
	g.pcmStartJob("Reading selected file…", func() pcmJobResult {
		p, err := b.Load(index)
		problem := ""
		if err != nil {
			problem = err.Error()
		}
		return pcmJobResult{kind: "batch_select", index: index, prepared: p, problem: problem, phase: phase}
	})
}
func (g *Game) pcmRemoveQueued() {
	s := &g.pcm
	if s.batch == nil || s.phase != "review" {
		return
	}
	g.stopAudioPreview()
	index := s.batchIndex
	if err := s.batch.Remove(index); err != nil {
		s.message = err.Error()
		g.requestRender()
		return
	}
	if len(s.batch.Items) == 0 {
		g.pcmDiscardBatch()
		s.prepared = audioimport.Prepared{}
		s.phase = "choose"
		s.path = ""
		s.message = "Import queue cleared. The project is unchanged."
		g.requestRender()
		return
	}
	next := max(0, min(index, len(s.batch.Items)-1))
	s.batchIndex = next
	s.name = s.batch.Items[next].Name
	s.prepared = s.batch.Items[next].Prepared
	g.pcmSelectBatch(next)
}
func (g *Game) pcmImportBatch() {
	s := &g.pcm
	_, names := g.pcmBatchPathsAndNames()
	if err := s.batch.ValidateNames(names); err != nil {
		s.message = err.Error()
		g.requestRender()
		return
	}
	replacements := 0
	for i, item := range s.batch.Items {
		for _, existing := range s.entries {
			if existing.Music == item.Prepared.Music && existing.Item.Name == names[i] {
				replacements++
				break
			}
		}
	}
	if replacements > 0 && !s.confirmReplace {
		s.confirmReplace = true
		s.message = fmt.Sprintf("This batch replaces %d existing asset(s), keeping their Lua IDs. Choose REPLACE + IMPORT to confirm the complete batch.", replacements)
		g.requestRender()
		return
	}
	b, project, converter := s.batch, g.model.Dir, s.converter
	g.pcmStartJob("Converting complete batch and saving bank once…", func() pcmJobResult {
		r, err := b.Import(project, names, converter)
		return pcmJobResult{kind: "batch_import", result: r, err: err}
	})
}
func (g *Game) pcmBatchImported(manifest assetsync.DirectAudioManifest) {
	s := &g.pcm
	s.batchCommands = nil
	for _, item := range s.batch.Items {
		items := manifest.Samples
		if item.Prepared.Music {
			items = manifest.Music
		}
		command := ""
		for id, stored := range items {
			if stored.Name == item.Name && !stored.Deleted {
				command = pcmEntry{Music: item.Prepared.Music, ID: id}.command()
				break
			}
		}
		s.batchCommands = append(s.batchCommands, command)
	}
	s.command = s.batchCommands[s.batchIndex]
	s.message = fmt.Sprintf("Imported all %d files and saved the bank once. Select a row for its Lua call, or COPY ALL CALLS for a commented reference list.", len(s.batch.Items))
}
func (g *Game) pcmCopyAll() {
	var lines []string
	for i, item := range g.pcm.batch.Items {
		if i < len(g.pcm.batchCommands) {
			lines = append(lines, "-- "+item.Name+": "+g.pcm.batchCommands[i])
		}
	}
	if err := hostclipboard.Write(strings.Join(lines, "\n")); err != nil {
		g.pcm.message = err.Error()
	} else {
		g.pcm.message = "Copied all calls as comments. Use each music call in _init() and each WAV call when its game event happens."
	}
	g.requestRender()
}
