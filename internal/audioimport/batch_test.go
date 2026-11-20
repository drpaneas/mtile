package audioimport

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func closeTestBatch(t *testing.T, b *Batch) {
	t.Helper()
	if err := b.Close(); err != nil {
		t.Error(err)
	}
}

func TestBatchStagesSequentiallyAppliesSharedSettingsAndCompilesOnce(t *testing.T) {
	t.Setenv("MTILE_IMPORT_HELPER", "1")
	tool, _ := os.Executable()
	project := t.TempDir()
	count := filepath.Join(t.TempDir(), "calls.txt")
	t.Setenv("MTILE_IMPORT_COUNT", count)
	one := testWave(t, 1, 16, 48000, 1, func(i, ch int) float64 {
		if i < 4800 || i >= 7200 {
			return 0
		}
		return .5 * math.Sin(2*math.Pi*1000*float64(i)/48000)
	})
	two := testWave(t, 1, 16, 8000, 1, func(i, ch int) float64 { return .5 })
	module := filepath.Join(t.TempDir(), "theme.mod")
	if err := os.WriteFile(module, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	progress := 0
	b, err := PrepareBatch([]string{one, two, module}, nil, PrepareOptions{Quality: QualitySmall, TrimSilence: true}, func(string) { progress++ })
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestBatch(t, b)
	if len(b.Items) != 3 || b.ErrorCount() != 0 || progress != 3 {
		t.Fatalf("batch: %+v", b.Items)
	}
	if !b.Items[0].Prepared.Trimmed || b.Items[1].Prepared.Trimmed || !b.Items[2].Prepared.Music {
		t.Fatal("shared trim did not skip ineligible recordings/modules")
	}
	if b.Items[0].Name == b.Items[1].Name {
		t.Fatal("duplicate basenames were not disambiguated")
	}
	for i, item := range b.Items {
		if item.Prepared.Data != nil {
			t.Fatal("batch retained every waveform in RAM")
		}
		p, err := b.Load(i)
		if err != nil || len(p.Data) != item.Bytes {
			t.Fatal("staged waveform missing", err)
		}
	}
	names := []string{"jump", "hit", "theme"}
	r, err := b.Import(project, names, tool)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Manifest.Samples) != 2 || len(r.Manifest.Music) != 1 {
		t.Fatal("batch missing items")
	}
	raw, _ := os.ReadFile(count)
	if string(raw) != "convert\n" {
		t.Fatalf("converter ran more than once: %q", raw)
	}
	if _, err := b.Import(project, []string{"same", "same", "theme"}, tool); err == nil {
		t.Fatal("duplicate edited names accepted")
	}
	after, _ := os.ReadFile(count)
	if string(raw) != string(after) {
		t.Fatal("invalid names invoked converter")
	}
	dir := b.dir
	closeTestBatch(t, b)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("batch workspace retained", err)
	}
}

func TestBatchErrorsCannotPartiallyPublishAndCanBeRemoved(t *testing.T) {
	t.Setenv("MTILE_IMPORT_HELPER", "1")
	tool, _ := os.Executable()
	project := t.TempDir()
	wave := testWave(t, 1, 16, 8000, 1, func(int, int) float64 { return .5 })
	p, e := Prepare(wave, QualityBalanced)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ImportPrepared(project, p, "existing", tool); e != nil {
		t.Fatal(e)
	}
	manifest := filepath.Join(project, "assets/audio/direct/manifest.json")
	before, _ := os.ReadFile(manifest)
	bad := filepath.Join(t.TempDir(), "bad.wav")
	if err := os.WriteFile(bad, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := PrepareBatch([]string{wave, bad}, nil, PrepareOptions{Quality: QualityBalanced}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestBatch(t, b)
	if b.ErrorCount() != 1 {
		t.Fatal("bad file not retained for review")
	}
	if _, err := b.Import(project, []string{"first", "bad"}, tool); err == nil {
		t.Fatal("partially imported batch")
	}
	after, _ := os.ReadFile(manifest)
	if string(before) != string(after) {
		t.Fatal("failed batch changed manifest")
	}
	if err := b.Remove(1); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Import(project, []string{"first"}, tool); err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(t.TempDir(), "broken.mod")
	if err := os.WriteFile(module, []byte("FAIL"), 0600); err != nil {
		t.Fatal(err)
	}
	fail, err := PrepareBatch([]string{wave, module}, nil, PrepareOptions{Quality: QualityBalanced}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestBatch(t, fail)
	before, _ = os.ReadFile(manifest)
	if _, err := fail.Import(project, []string{"replacement", "broken"}, tool); err == nil || !strings.Contains(err.Error(), "mmutil") {
		t.Fatal("conversion error ignored", err)
	}
	after, _ = os.ReadFile(manifest)
	if string(before) != string(after) {
		t.Fatal("converter failure published partial batch")
	}
}

func TestBatchDeduplicatesPathsAndPreservesExplicitEditedNames(t *testing.T) {
	path := testWave(t, 1, 16, 8000, 1, func(int, int) float64 { return 0 })
	b, err := PrepareBatch([]string{path, path}, []string{"my_sound", "duplicate"}, PrepareOptions{Quality: QualityBalanced}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestBatch(t, b)
	if len(b.Items) != 1 || b.Items[0].Name != "my_sound" {
		t.Fatal("path duplication or edited name lost")
	}
}
