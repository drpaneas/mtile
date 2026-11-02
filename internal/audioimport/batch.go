package audioimport

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Batch owns an offline workspace. Only the selected recording needs to be in
// memory; prepared waveforms stay on disk until one atomic bank publication.
type Batch struct {
	Items   []BatchItem
	Options PrepareOptions
	dir     string
}
type BatchItem struct {
	Path, Name string
	Prepared   Prepared // metadata only; Data is read on demand
	Bytes      int
	Error      string
	staged     string
}

func PrepareBatch(paths []string, names []string, opts PrepareOptions, progress func(string)) (*Batch, error) {
	if opts.Quality == QualitySource {
		opts.TrimSilence = false
	}
	if len(paths) == 0 || len(paths) > 512 {
		return nil, fmt.Errorf("choose between 1 and 512 audio files")
	}
	if len(names) != 0 && len(names) != len(paths) {
		return nil, fmt.Errorf("batch names do not match its source files")
	}
	dir, err := os.MkdirTemp("", "mtile-audio-batch-*")
	if err != nil {
		return nil, err
	}
	b := &Batch{dir: dir, Options: opts}
	seenPaths, seenNames := map[string]bool{}, map[string]bool{}
	for i, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			b.Close()
			return nil, err
		}
		if seenPaths[absolute] {
			continue
		}
		seenPaths[absolute] = true
		if progress != nil {
			progress(fmt.Sprintf("Preparing %d/%d: %s", i+1, len(paths), filepath.Base(path)))
		}
		p, prepareErr := prepareWithOptions(path, opts, true)
		name := p.Name
		if name == "" {
			name = SafeName(filepath.Base(path))
		}
		if len(names) > 0 && names[i] != "" {
			name = names[i]
		} else {
			base := name
			for suffix := 2; seenNames[fmt.Sprintf("%t:%s", p.Music, name)]; suffix++ {
				name = fmt.Sprintf("%s_%d", base, suffix)
			}
		}
		seenNames[fmt.Sprintf("%t:%s", p.Music, name)] = true
		item := BatchItem{Path: absolute, Name: name, Prepared: p, Bytes: len(p.Data)}
		if prepareErr != nil {
			item.Error = prepareErr.Error()
		} else {
			item.staged = fmt.Sprintf("%03d%s", len(b.Items), p.Extension)
			if err := os.WriteFile(filepath.Join(dir, item.staged), p.Data, 0600); err != nil {
				b.Close()
				return nil, err
			}
		}
		item.Prepared.Data = nil
		b.Items = append(b.Items, item)
	}
	return b, nil
}

func (b *Batch) Close() error {
	if b == nil || b.dir == "" {
		return nil
	}
	err := os.RemoveAll(b.dir)
	b.dir = ""
	return err
}
func (b *Batch) Load(index int) (Prepared, error) {
	if b == nil || index < 0 || index >= len(b.Items) {
		return Prepared{}, fmt.Errorf("batch selection is not present")
	}
	item := b.Items[index]
	p := item.Prepared
	if item.Error != "" {
		return p, fmt.Errorf("%s", item.Error)
	}
	if b.dir == "" {
		return p, fmt.Errorf("batch workspace is closed; select the source files again")
	}
	raw, err := readSource(filepath.Join(b.dir, item.staged))
	p.Data = raw
	return p, err
}
func (b *Batch) ErrorCount() int {
	n := 0
	for _, item := range b.Items {
		if item.Error != "" {
			n++
		}
	}
	return n
}
func (b *Batch) PreparedBytes() int {
	n := 0
	for _, item := range b.Items {
		n += item.Bytes
	}
	return n
}
func (b *Batch) TrimEligibleCount() int {
	n := 0
	for _, item := range b.Items {
		if item.Error == "" && !item.Prepared.Music && item.Prepared.Analysis.CanTrim() {
			n++
		}
	}
	return n
}
func (b *Batch) Remove(index int) error {
	if b == nil || index < 0 || index >= len(b.Items) {
		return fmt.Errorf("batch selection is not present")
	}
	if b.Items[index].staged != "" {
		if err := os.Remove(filepath.Join(b.dir, b.Items[index].staged)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	copy(b.Items[index:], b.Items[index+1:])
	b.Items[len(b.Items)-1] = BatchItem{}
	b.Items = b.Items[:len(b.Items)-1]
	return nil
}

// Import calls the shared converter exactly once for the complete selection.
// Names are validated together before any project file can change.
func (b *Batch) Import(project string, names []string, converter string) (Result, error) {
	if err := b.ValidateNames(names); err != nil {
		return Result{}, err
	}
	export, err := os.MkdirTemp(b.dir, "export-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(export)
	paths := make([]string, 0, len(b.Items))
	for i, item := range b.Items {
		path := filepath.Join(export, names[i]+item.Prepared.Extension)
		if err := copyBatchFile(filepath.Join(b.dir, item.staged), path); err != nil {
			return Result{}, err
		}
		paths = append(paths, path)
	}
	return Import(project, paths, converter)
}

func (b *Batch) ValidateNames(names []string) error {
	if b == nil || b.dir == "" || len(b.Items) == 0 {
		return fmt.Errorf("no prepared batch to import")
	}
	if len(names) != len(b.Items) {
		return fmt.Errorf("batch names do not match its prepared files")
	}
	seen := map[string]bool{}
	for i, item := range b.Items {
		if item.Error != "" {
			return fmt.Errorf("file %d (%s): %s; fix or remove it before importing", i+1, item.Name, item.Error)
		}
		if !identifier.MatchString(names[i]) {
			return fmt.Errorf("file %d: asset name must start with a lowercase letter and contain only lowercase letters, digits or underscores", i+1)
		}
		key := fmt.Sprintf("%t:%s", item.Prepared.Music, names[i])
		if seen[key] {
			return fmt.Errorf("duplicate asset name %q in this batch; rename one before importing", names[i])
		}
		seen[key] = true
	}
	return nil
}
func copyBatchFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
