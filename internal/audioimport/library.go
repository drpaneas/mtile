package audioimport

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// Delete removes audio data without renumbering another public Lua ID. Deleted
// slots are retained; reimporting the same name restores its previous ID.
func Delete(project string, music bool, id int, converter string) (Result, error) {
	var result Result
	old, err := assetsync.LoadDirectAudio(project)
	if err != nil {
		return result, err
	}
	if old == nil {
		return result, fmt.Errorf("no imported audio")
	}
	manifest := old.Manifest
	manifest.Version = 2
	items := manifest.Samples
	if music {
		items = manifest.Music
	}
	if id < 0 || id >= len(items) || items[id].Deleted {
		return result, fmt.Errorf("audio ID %d is not present", id)
	}
	deletedSource := items[id].Source
	items[id].Deleted, items[id].Source, items[id].BankID = true, "", 65535
	sources := map[string][]byte{}
	for _, group := range [][]assetsync.DirectAudioItem{manifest.Music, manifest.Samples} {
		for _, item := range group {
			if item.Deleted {
				continue
			}
			raw, err := readSource(filepath.Join(project, "assets/audio/direct/source", item.Source))
			if err != nil {
				return result, err
			}
			sources[item.Source] = raw
		}
	}
	result, err = compileBank(project, manifest, sources, converter)
	if err != nil {
		return result, err
	}
	// The manifest switch succeeded. Only now remove the obsolete generated bank
	// and the explicitly deleted source, never arbitrary files from the directory.
	dir := filepath.Join(project, "assets/audio/direct")
	if old.Manifest.Bank != result.Manifest.Bank {
		if err := os.Remove(filepath.Join(dir, old.Manifest.Bank)); err != nil && !os.IsNotExist(err) {
			result.ConverterOutput += "\nWarning: previous generated bank could not be removed: " + err.Error()
		}
	}
	if err := os.Remove(filepath.Join(dir, "source", deletedSource)); err != nil && !os.IsNotExist(err) {
		result.ConverterOutput += "\nWarning: deleted source could not be removed: " + err.Error()
	}
	return result, nil
}
