package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/audioimport"
)

func TestPCMBatchSharedSettingsKeepNamesAndSkipIneligibleTrim(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	defer g.closePCMEditor()
	quiet := pcmQuietFixture(t)
	bell := filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/sfx_bell.wav")
	module := filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/music_theme.mod")
	g.pcmChooseFiles([]string{quiet, bell, module}, false)
	waitPCM(t, g)
	if g.pcm.batch == nil || len(g.pcm.batch.Items) != 3 || g.pcm.batch.ErrorCount() != 0 {
		t.Fatal("batch not prepared", g.pcm.message)
	}
	g.pcm.name = "my_door"
	g.pcmSelectBatch(1)
	waitPCM(t, g)
	if g.pcm.batch.Items[0].Name != "my_door" {
		t.Fatal("name lost on queue selection")
	}
	g.pcmSetQuality(audioimport.QualitySmall)
	waitPCM(t, g)
	if g.pcm.batch.Items[0].Name != "my_door" {
		t.Fatal("name lost on shared reprepare")
	}
	for _, item := range g.pcm.batch.Items {
		if !item.Prepared.Music && item.Prepared.After.Rate > 8000 {
			t.Fatal("shared quality not applied")
		}
		if item.Prepared.Data != nil {
			t.Fatal("queue retained every waveform")
		}
	}
	g.pcmToggleTrim()
	waitPCM(t, g)
	if !g.pcm.trimSilence || !g.pcm.batch.Items[0].Prepared.Trimmed || g.pcm.batch.Items[2].Prepared.Trimmed {
		t.Fatal("shared trim did not respect eligibility")
	}
	g.pcmSetQuality(audioimport.QualitySource)
	waitPCM(t, g)
	if g.pcm.trimSilence || g.pcm.batch.Items[0].Prepared.Trimmed {
		t.Fatal("Keep Source retained trimming")
	}
	old := g.pcm.batch
	g.closePCMEditor()
	if _, err := old.Load(0); err == nil {
		t.Fatal("closing the editor retained the stage")
	}
	if _, err := os.Stat(quiet); err != nil {
		t.Fatal("original source removed", err)
	}
}

func TestPCMBatchBadRowsBlockImportAndRemovalKeepsSurvivingNames(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	defer g.closePCMEditor()
	first, second := pcmQuietFixture(t), pcmQuietFixture(t)
	bad := filepath.Join(t.TempDir(), "bad.wav")
	if err := os.WriteFile(bad, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	g.pcmChooseFiles([]string{first, bad, second}, false)
	waitPCM(t, g)
	if g.pcm.batch.ErrorCount() != 1 {
		t.Fatal("bad row hidden")
	}
	g.pcmImport()
	if g.pcm.job != nil || !strings.Contains(g.pcm.message, "fix or remove") {
		t.Fatal("invalid batch reached converter", g.pcm.message)
	}
	if a, err := assetsync.LoadDirectAudio(g.model.Dir); err != nil || a != nil {
		t.Fatal("invalid batch changed project", err)
	}
	lastName := g.pcm.batch.Items[2].Name
	g.pcmSelectBatch(1)
	waitPCM(t, g)
	g.pcmRemoveQueued()
	waitPCM(t, g)
	if len(g.pcm.batch.Items) != 2 || g.pcm.batch.ErrorCount() != 0 || g.pcm.batch.Items[1].Name != lastName {
		t.Fatal("removing a row changed another asset name")
	}
	g.pcmChooseFiles([]string{bad}, true)
	waitPCM(t, g)
	if len(g.pcm.batch.Items) != 3 || g.pcm.batch.Items[1].Name != lastName {
		t.Fatal("adding files replaced the queue")
	}
}

func TestPCMBatchRealMixedImportAndReplacementHaveStableCalls(t *testing.T) {
	tool := os.Getenv("MTILE_PCM_MMUTIL")
	if tool == "" {
		t.Skip("set MTILE_PCM_MMUTIL for real mixed batch conversion")
	}
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	defer g.closePCMEditor()
	g.pcm.converter = tool
	quiet := pcmQuietFixture(t)
	module := filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/music_theme.mod")
	g.pcmChooseFiles([]string{quiet, module}, false)
	waitPCM(t, g)
	g.pcm.name = "door"
	g.pcmSaveBatchName()
	g.pcmToggleTrim()
	waitPCM(t, g)
	g.pcmImport()
	waitPCM(t, g)
	if g.pcm.phase != "done" || len(g.pcm.batchCommands) != 2 || g.pcm.batchCommands[0] != "sample(0)" || g.pcm.batchCommands[1] != "pcm_music(0)" {
		t.Fatal("batch calls mismatch", g.pcm.message, g.pcm.batchCommands)
	}
	g.pcmSelectBatch(1)
	waitPCM(t, g)
	if g.pcm.command != "pcm_music(0)" || g.pcm.phase != "done" {
		t.Fatal("completed queue does not expose each call")
	}
	g.pcmReprepareBatch()
	waitPCM(t, g)
	g.pcmImport()
	if !g.pcm.confirmReplace || g.pcm.job != nil {
		t.Fatal("batch replacement lacked confirmation")
	}
	g.pcmImport()
	waitPCM(t, g)
	if g.pcm.batchCommands[0] != "sample(0)" || g.pcm.batchCommands[1] != "pcm_music(0)" {
		t.Fatal("replacement changed public IDs")
	}
}

func TestPCMPathListAcceptsSpacesAndQuotedFullPaths(t *testing.T) {
	paths := pcmPathLines("\"/Music/jump sound.wav\"\r\n/Music/ήχος.wav\n\n")
	if len(paths) != 2 || paths[0] != "/Music/jump sound.wav" || paths[1] != "/Music/ήχος.wav" {
		t.Fatal(paths)
	}
}

func TestPCMBatchQueueKeepsSelectionVisibleAfterManyRemovals(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	defer g.closePCMEditor()
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/sfx_bell.wav"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 12; i++ {
		path := filepath.Join(dir, fmt.Sprintf("clip%d.wav", i))
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	g.pcmChooseFiles(paths, false)
	waitPCM(t, g)
	g.pcmSelectBatch(11)
	waitPCM(t, g)
	if g.pcm.batchScroll <= 0 {
		t.Fatal("far selection is not visible")
	}
	for len(g.pcm.batch.Items) > 5 {
		g.pcmRemoveQueued()
		waitPCM(t, g)
	}
	if g.pcm.batchScroll != 0 || g.pcm.batchIndex >= len(g.pcm.batch.Items) {
		t.Fatal("queue scroll escaped remaining rows")
	}
}

func TestPCMBatchNewSelectionDoesNotInheritAnEarlierTrimOptIn(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	defer g.closePCMEditor()
	first, second := pcmQuietFixture(t), pcmQuietFixture(t)
	g.pcmChooseFiles([]string{first, second}, false)
	waitPCM(t, g)
	g.pcmToggleTrim()
	waitPCM(t, g)
	if !g.pcm.trimSilence {
		t.Fatal("initial trim opt-in did not apply")
	}
	g.closePCMEditor()
	g.openPCMEditor()
	g.pcmChooseFiles([]string{first, second}, false)
	waitPCM(t, g)
	for _, item := range g.pcm.batch.Items {
		if item.Prepared.Trimmed {
			t.Fatal("new selection silently inherited an earlier trim opt-in")
		}
	}
	if g.pcm.trimSilence {
		t.Fatal("new batch retained old trim setting")
	}
}

func TestPCMBatchShutdownKeepsStageUntilActiveJobFinishes(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	defer g.closePCMEditor()
	g.pcmChooseFiles([]string{pcmQuietFixture(t), pcmQuietFixture(t)}, false)
	waitPCM(t, g)
	b := g.pcm.batch
	started, release := make(chan struct{}), make(chan struct{})
	loaded := make(chan error, 1)
	g.pcmStartJob("Reading staged audio", func() pcmJobResult {
		close(started)
		<-release
		_, err := b.Load(0)
		loaded <- err
		return pcmJobResult{kind: "retired"}
	})
	<-started
	retired := g.pcmDiscardBatch()
	close(release)
	if err := <-loaded; err != nil {
		t.Fatal("shutdown removed data still owned by the active job", err)
	}
	<-retired
	if _, err := b.Load(0); err == nil {
		t.Fatal("finished job retained the retired stage")
	}
}

func TestPCMBatchShutdownRetiresUnacceptedPreparation(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	b, err := audioimport.PrepareBatch([]string{pcmQuietFixture(t)}, nil, audioimport.PrepareOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	}()
	release := make(chan struct{})
	g.pcmStartProgressJob("Preparing", func(progress func(string)) pcmJobResult {
		progress("First file")
		<-release
		return pcmJobResult{kind: "batch_prepare", batch: b}
	})
	retired := g.pcmDiscardBatch()
	close(release)
	<-retired
	if _, err := b.Load(0); err == nil {
		t.Fatal("shutdown retained a prepared workspace not yet accepted by the UI")
	}
}
