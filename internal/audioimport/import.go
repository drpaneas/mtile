// Package audioimport builds the same Maxmod soundbank used by Butano. Asset
// loading and live IDE diagnostics never execute the external converter.
package audioimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
)

type Result struct {
	Manifest        assetsync.DirectAudioManifest
	ConverterOutput string
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var definition = regexp.MustCompile(`(?m)^#define\s+(MOD|SFX)_([A-Z0-9_]+)\s+(\d+)\s*$`)

// Import adds or replaces named sources. It rebuilds all imported assets in a
// private workspace, then publishes the content-addressed bank and finally its
// manifest. Failed conversion leaves the previously playable bank untouched.
func Import(project string, files []string, converter string) (Result, error) {
	var result Result
	if len(files) == 0 {
		return result, errors.New("import-audio requires one or more WAV, MOD, XM, S3M or IT files")
	}
	if _, err := os.Stat(project); err != nil {
		return result, fmt.Errorf("project: %w", err)
	}
	old, err := assetsync.LoadDirectAudio(project)
	if err != nil {
		return result, err
	}
	manifest := assetsync.DirectAudioManifest{Version: 1, Music: []assetsync.DirectAudioItem{}, Samples: []assetsync.DirectAudioItem{}}
	if old != nil {
		manifest = old.Manifest
	}
	if converter == "" {
		converter = os.Getenv("MMUTIL")
	}
	if converter == "" {
		converter = "mmutil"
	}
	tool, err := exec.LookPath(converter)
	if err != nil {
		return result, errors.New("mmutil not found; install the Maxmod converter used by Butano, set MMUTIL, or pass -mmutil /path/to/mmutil (see docs/audio/samples.md)")
	}
	tool, err = filepath.Abs(tool)
	if err != nil {
		return result, err
	}
	work, err := os.MkdirTemp("", "mtile-audio-import-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(work)
	sources := map[string][]byte{}
	for _, group := range [][]assetsync.DirectAudioItem{manifest.Music, manifest.Samples} {
		for _, item := range group {
			raw, err := readSource(filepath.Join(project, "assets", "audio", "direct", "source", item.Source))
			if err != nil {
				return result, err
			}
			sources[item.Source] = raw
		}
	}
	seen := map[string]bool{}
	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file))
		name := strings.ToLower(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
		if !identifier.MatchString(name) {
			return result, fmt.Errorf("audio filename %q: use a letter followed by letters, digits or underscores", filepath.Base(file))
		}
		music := false
		switch ext {
		case ".wav":
		case ".mod", ".xm", ".s3m", ".it":
			music = true
		default:
			return result, fmt.Errorf("unsupported audio format %q; use WAV, MOD, XM, S3M or IT", ext)
		}
		key := fmt.Sprintf("%t:%s", music, name)
		if seen[key] {
			return result, fmt.Errorf("duplicate audio name %s", name)
		}
		seen[key] = true
		raw, err := readSource(file)
		if err != nil {
			return result, err
		}
		items := &manifest.Samples
		if music {
			items = &manifest.Music
		}
		index := -1
		for i, item := range *items {
			if item.Name == name {
				index = i
				break
			}
		}
		if index < 0 {
			if len(*items) >= 256 {
				return result, errors.New("at most 256 songs and 256 WAV effects are supported")
			}
			index = len(*items)
			*items = append(*items, assetsync.DirectAudioItem{Name: name})
		}
		oldSource := (*items)[index].Source
		// Prefix only the converter's working filename, so a song and a WAV
		// effect may have the same public name without C header collisions.
		prefix := "sfx_"
		if music {
			prefix = "music_"
		}
		source := prefix + name + ext
		(*items)[index].Source = source
		sources[source] = raw
		if oldSource != "" && oldSource != source {
			delete(sources, oldSource)
		}
	}
	args := []string{}
	for _, group := range [][]assetsync.DirectAudioItem{manifest.Music, manifest.Samples} {
		for _, item := range group {
			if err := os.WriteFile(filepath.Join(work, item.Source), sources[item.Source], 0o644); err != nil {
				return result, err
			}
			args = append(args, item.Source)
		}
	}
	args = append(args, "-obank.bin", "-hbank.h", "-y")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = work
	output, err := cmd.CombinedOutput()
	result.ConverterOutput = string(output)
	if err != nil {
		return result, fmt.Errorf("mmutil import failed: %w\n%s", err, output)
	}
	bank, err := os.ReadFile(filepath.Join(work, "bank.bin"))
	if err != nil {
		return result, err
	}
	header, err := os.ReadFile(filepath.Join(work, "bank.h"))
	if err != nil {
		return result, err
	}
	ids := map[string]int{}
	for _, match := range definition.FindAllStringSubmatch(string(header), -1) {
		id, _ := strconv.Atoi(match[3])
		ids[match[1]+"_"+match[2]] = id
	}
	for _, group := range []struct {
		items  []assetsync.DirectAudioItem
		prefix string
	}{{manifest.Music, "MOD_"}, {manifest.Samples, "SFX_"}} {
		for i := range group.items {
			item := &group.items[i]
			key := group.prefix + strings.ToUpper(strings.TrimSuffix(item.Source, filepath.Ext(item.Source)))
			id, ok := ids[key]
			if !ok {
				return result, fmt.Errorf("mmutil did not import %s", item.Source)
			}
			item.BankID = id
		}
	}
	if len(bank) < 20 {
		return result, errors.New("mmutil produced an empty soundbank")
	}
	// MAS prefix version selects the matching player layout at link time.
	off := uint64(bank[12]) | uint64(bank[13])<<8 | uint64(bank[14])<<16 | uint64(bank[15])<<24
	if off+5 >= uint64(len(bank)) {
		return result, errors.New("mmutil produced an invalid first item")
	}
	manifest.Format = int(bank[off+5])
	hash := sha256.Sum256(bank)
	manifest.SHA256 = hex.EncodeToString(hash[:])
	manifest.Bank = "bank-" + manifest.SHA256 + ".bin"
	if err := assetsync.ValidateDirectAudio(manifest, bank); err != nil {
		return result, err
	}
	dir := filepath.Join(project, "assets", "audio", "direct")
	if err := os.MkdirAll(filepath.Join(dir, "source"), 0o755); err != nil {
		return result, err
	}
	for file, raw := range sources {
		if err := writeAtomic(filepath.Join(dir, "source", file), raw); err != nil {
			return result, err
		}
	}
	if err := writeAtomic(filepath.Join(dir, manifest.Bank), bank); err != nil {
		return result, err
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return result, err
	}
	if err := writeAtomic(filepath.Join(dir, "manifest.json"), append(encoded, '\n')); err != nil {
		return result, err
	}
	result.Manifest = manifest
	return result, nil
}

func readSource(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 24<<20 {
		return nil, fmt.Errorf("audio source %s must be a regular file up to 24 MiB", path)
	}
	return os.ReadFile(path)
}
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".audio-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
