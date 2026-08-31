package assetsync

import (
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

func Load(projectDir string) (*Assets, error) {
	project, err := LoadProject(projectDir)
	if err != nil {
		return nil, err
	}
	sheet, err := LoadSpritesheet(projectDir)
	if err != nil {
		return nil, err
	}
	mapData, err := LoadMap(projectDir)
	if err != nil {
		return nil, err
	}
	palette, err := LoadPalette(projectDir, project.ActivePalette)
	if err != nil {
		return nil, err
	}
	assets, err := Flatten(project, *sheet, *mapData, *palette)
	if err != nil {
		return nil, err
	}
	audioVersion, err := DetectAudioVersion(projectDir)
	if err != nil {
		return nil, err
	}
	assets.AudioVersion = audioVersion
	if audioVersion == NativeAudioSchemaVersion {
		native, err := LoadNativeAudio(projectDir)
		if err != nil {
			return nil, err
		}
		assets.NativeAudio = native
		return assets, nil
	}
	settings, err := LoadAudioSettings(projectDir)
	if err != nil {
		return nil, err
	}
	assets.AudioSettings = *settings
	library, err := LoadSFX(projectDir)
	if err != nil {
		return nil, err
	}
	definitions, steps, err := CompileSFX(*library)
	if err != nil {
		return nil, err
	}
	assets.SFXLibrary = *library
	assets.SFXROM = definitions
	assets.SFXStepROM = steps
	music, err := LoadMusic(projectDir, *library)
	if err != nil {
		return nil, err
	}
	patterns, err := CompileMusic(*music, *library)
	if err != nil {
		return nil, err
	}
	assets.MusicLibrary = *music
	assets.MusicROM = patterns
	samples, err := LoadSamples(projectDir)
	if err != nil {
		return nil, err
	}
	sampleDefinitions, samplePCM, err := CompileSamples(projectDir, *samples)
	if err != nil {
		return nil, err
	}
	assets.SampleLibrary = *samples
	assets.SampleROM = sampleDefinitions
	assets.SamplePCMROM = samplePCM
	return assets, nil
}

// Flatten validates native project assets and returns the ROM-ready flattened
// representation. Editors use this on in-memory documents before saving; the
// generated build path uses Load, which reads the same on-disk schema.
func Flatten(project Project, sheet Spritesheet, mapData Map, palette Palette) (*Assets, error) {
	return flatten(project, sheet, mapData, palette)
}

func LoadProject(projectDir string) (Project, error) {
	var project Project
	err := readJSON(filepath.Join(projectDir, "project.json"), &project)
	if errors.Is(err, os.ErrNotExist) {
		return Project{ActivePalette: CanonicalPaletteName, Renderer: DefaultRendererName}, nil
	}
	if err != nil {
		return Project{}, fmt.Errorf("read project.json: %w", err)
	}
	if project.ActivePalette == "" {
		project.ActivePalette = CanonicalPaletteName
	}
	if project.Renderer == "" {
		project.Renderer = DefaultRendererName
	}
	if err := validatePaletteName(project.ActivePalette); err != nil {
		return Project{}, fmt.Errorf("project activePalette: %w", err)
	}
	if err := validateProjectEntry(project.Entry); err != nil {
		return Project{}, fmt.Errorf("project entry: %w", err)
	}
	if err := validateProjectRenderer(project.Renderer); err != nil {
		return Project{}, fmt.Errorf("project renderer: %w", err)
	}
	return project, nil
}

func SaveProject(projectDir string, project Project) error {
	if project.ActivePalette == "" {
		project.ActivePalette = CanonicalPaletteName
	}
	if project.Renderer == "" {
		project.Renderer = DefaultRendererName
	}
	if err := validatePaletteName(project.ActivePalette); err != nil {
		return fmt.Errorf("project activePalette: %w", err)
	}
	if err := validateProjectEntry(project.Entry); err != nil {
		return fmt.Errorf("project entry: %w", err)
	}
	if err := validateProjectRenderer(project.Renderer); err != nil {
		return fmt.Errorf("project renderer: %w", err)
	}
	return writeJSON(filepath.Join(projectDir, "project.json"), project)
}

func validateProjectRenderer(renderer string) error {
	switch renderer {
	case "mode4", "mode0", "auto":
		return nil
	default:
		return fmt.Errorf("must be mode4, mode0, or auto, got %q", renderer)
	}
}

func validateProjectEntry(entry string) error {
	if entry == "" {
		return nil
	}
	if filepath.IsAbs(entry) || filepath.VolumeName(entry) != "" {
		return fmt.Errorf("must be project-relative")
	}
	clean := filepath.Clean(filepath.FromSlash(entry))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("must stay inside the project")
	}
	if filepath.Ext(clean) != ".lua" {
		return fmt.Errorf("must name a .lua file")
	}
	return nil
}

func LoadSpritesheet(projectDir string) (*Spritesheet, error) {
	var sheet Spritesheet
	if err := readJSON(filepath.Join(projectDir, "assets", "spritesheet.json"), &sheet); err != nil {
		return nil, fmt.Errorf("read spritesheet.json: %w", err)
	}
	return &sheet, nil
}

func LoadMap(projectDir string) (*Map, error) {
	var mapData Map
	if err := readJSON(filepath.Join(projectDir, "assets", "map.json"), &mapData); err != nil {
		return nil, fmt.Errorf("read map.json: %w", err)
	}
	return &mapData, nil
}

func SaveSpritesheet(projectDir string, sheet *Spritesheet) error {
	if sheet == nil {
		return fmt.Errorf("spritesheet is required")
	}
	project, err := LoadProject(projectDir)
	if err != nil {
		return err
	}
	palette, err := LoadPalette(projectDir, project.ActivePalette)
	if err != nil {
		return err
	}
	if _, _, _, _, err := validateSheet(*sheet, len(palette.Colors)); err != nil {
		return err
	}
	return writeJSON(filepath.Join(projectDir, "assets", "spritesheet.json"), sheet)
}

func SaveMap(projectDir string, mapData *Map) error {
	if mapData == nil {
		return fmt.Errorf("map is required")
	}
	project, err := LoadProject(projectDir)
	if err != nil {
		return err
	}
	palette, err := LoadPalette(projectDir, project.ActivePalette)
	if err != nil {
		return err
	}
	sheet, err := LoadSpritesheet(projectDir)
	if err != nil {
		return err
	}
	_, _, _, spriteN, err := validateSheet(*sheet, len(palette.Colors))
	if err != nil {
		return err
	}
	if err := validateMap(*mapData, spriteN); err != nil {
		return err
	}
	return writeJSON(filepath.Join(projectDir, "assets", "map.json"), mapData)
}

func flatten(project Project, sheet Spritesheet, mapData Map, palette Palette) (*Assets, error) {
	sheetW, sheetH, sheetN, spriteN, err := validateSheet(sheet, len(palette.Colors))
	if err != nil {
		return nil, err
	}
	if err := validateMap(mapData, spriteN); err != nil {
		return nil, err
	}
	paletteROM, err := expandPalette(palette)
	if err != nil {
		return nil, err
	}

	flatSheet := make([]byte, sheetN)
	flags := make([]byte, spriteN)
	occupied := make([]bool, sheetN)
	seenIDs := make(map[int]struct{}, len(sheet.Sprites))
	for _, sprite := range sheet.Sprites {
		if _, exists := seenIDs[sprite.ID]; exists {
			return nil, fmt.Errorf("spritesheet sprite id %d is duplicated", sprite.ID)
		}
		seenIDs[sprite.ID] = struct{}{}
		if err := validateSpriteRecord(sprite, sheetW, sheetH, spriteN, len(palette.Colors)); err != nil {
			return nil, err
		}
		flags[sprite.ID] = byte(sprite.Flags)
		for y := 0; y < sprite.Height; y++ {
			for x := 0; x < sprite.Width; x++ {
				index := (sprite.Y+y)*sheetW + sprite.X + x
				if occupied[index] {
					return nil, fmt.Errorf("spritesheet sprites overlap at pixel (%d,%d)", sprite.X+x, sprite.Y+y)
				}
				occupied[index] = true
				flatSheet[index] = byte(sprite.Pixels[y][x])
			}
		}
	}

	mapLayers := mapData.LayerCount()
	planeCells, err := checkedProduct(mapData.Width, mapData.Height)
	if err != nil {
		return nil, fmt.Errorf("map: %w", err)
	}
	mapCellN, err := checkedProduct(planeCells, mapLayers)
	if err != nil {
		return nil, fmt.Errorf("map layers: %w", err)
	}
	flatMap := make([]uint16, mapCellN)
	for layer := 0; layer < mapLayers; layer++ {
		for i, cell := range mapData.LayerCells(layer) {
			flatMap[layer*planeCells+i] = uint16(cell)
		}
	}
	return &Assets{
		Project:      project,
		Palette:      Palette{Name: palette.Name, Colors: slices.Clone(palette.Colors)},
		PaletteROM:   paletteROM,
		SheetW:       sheetW,
		SheetH:       sheetH,
		SpriteCols:   sheetW / 8,
		SpriteRows:   sheetH / 8,
		SpriteN:      spriteN,
		MapW:         mapData.Width,
		MapH:         mapData.Height,
		MapLayers:    mapLayers,
		MapStreaming: len(flatMap) > MapStreamingThresholdCells,
		SheetROM:     flatSheet,
		MapROM:       flatMap,
		FlagROM:      flags,
	}, nil
}

func validateSheet(sheet Spritesheet, paletteLen int) (width, height, sheetN, spriteN int, err error) {
	if sheet.Width < 1 || sheet.Height < 1 {
		return 0, 0, 0, 0, fmt.Errorf("spritesheet dimensions must be positive, got %dx%d", sheet.Width, sheet.Height)
	}
	sheetN, err = checkedProduct(sheet.Width, sheet.Height)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("spritesheet: %w", err)
	}
	cols := sheet.Width / 8
	rows := sheet.Height / 8
	spriteN, err = checkedProduct(cols, rows)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("spritesheet sprite count: %w", err)
	}
	if spriteN > MaxSpriteN {
		return 0, 0, 0, 0, fmt.Errorf("spritesheet has %d complete sprites (maximum %d)", spriteN, MaxSpriteN)
	}
	if paletteLen != 16 && paletteLen != 256 {
		return 0, 0, 0, 0, fmt.Errorf("active palette has %d colors (want 16 or 256)", paletteLen)
	}
	seenIDs := make(map[int]struct{}, len(sheet.Sprites))
	for _, sprite := range sheet.Sprites {
		if _, exists := seenIDs[sprite.ID]; exists {
			return 0, 0, 0, 0, fmt.Errorf("spritesheet sprite id %d is duplicated", sprite.ID)
		}
		seenIDs[sprite.ID] = struct{}{}
		if err := validateSpriteRecord(sprite, sheet.Width, sheet.Height, spriteN, paletteLen); err != nil {
			return 0, 0, 0, 0, err
		}
	}
	if err := validateSpriteOverlaps(sheet); err != nil {
		return 0, 0, 0, 0, err
	}
	return sheet.Width, sheet.Height, sheetN, spriteN, nil
}

func validateSpriteRecord(sprite SpriteRecord, sheetW, sheetH, spriteN, paletteLen int) error {
	if sprite.ID < 0 || sprite.ID >= spriteN {
		return fmt.Errorf("spritesheet sprite id %d is outside 0..%d", sprite.ID, maxInt(spriteN-1, 0))
	}
	if sprite.X < 0 || sprite.Y < 0 || sprite.Width < 1 || sprite.Height < 1 {
		return fmt.Errorf("sprite %d has invalid rectangle (%d,%d %dx%d)", sprite.ID, sprite.X, sprite.Y, sprite.Width, sprite.Height)
	}
	x1, err := checkedSum(sprite.X, sprite.Width)
	if err != nil || x1 > sheetW {
		return fmt.Errorf("sprite %d rectangle exceeds sheet width", sprite.ID)
	}
	y1, err := checkedSum(sprite.Y, sprite.Height)
	if err != nil || y1 > sheetH {
		return fmt.Errorf("sprite %d rectangle exceeds sheet height", sprite.ID)
	}
	if len(sprite.Pixels) != sprite.Height {
		return fmt.Errorf("sprite %d has %d pixel rows, want %d", sprite.ID, len(sprite.Pixels), sprite.Height)
	}
	for y, row := range sprite.Pixels {
		if len(row) != sprite.Width {
			return fmt.Errorf("sprite %d row %d has %d pixels, want %d", sprite.ID, y, len(row), sprite.Width)
		}
		for x, pixel := range row {
			if pixel < 0 || pixel > 255 {
				return fmt.Errorf("sprite %d pixel (%d,%d)=%d is outside 8bpp range", sprite.ID, x, y, pixel)
			}
			if pixel >= paletteLen {
				return fmt.Errorf("sprite %d pixel (%d,%d)=%d is outside active palette length %d; use a 256-color palette", sprite.ID, x, y, pixel, paletteLen)
			}
		}
	}
	if sprite.Flags < 0 || sprite.Flags > 255 {
		return fmt.Errorf("sprite %d flags %d are outside 8-bit range", sprite.ID, sprite.Flags)
	}
	return nil
}

func validateSpriteOverlaps(sheet Spritesheet) error {
	if len(sheet.Sprites) < 2 {
		return nil
	}

	// Sweep rectangles from left to right. The active set contains only
	// rectangles whose x-range reaches the current rectangle; a range-max
	// tree over compressed y intervals answers whether any active rectangle
	// overlaps vertically. This keeps validation O(n log n) instead of making
	// the legal 65,536-sprite limit degenerate into billions of comparisons.
	rects := make([]spriteOverlapRect, len(sheet.Sprites))
	yCoords := make([]int, 0, len(sheet.Sprites)*2)
	for i, sprite := range sheet.Sprites {
		rects[i] = spriteOverlapRect{
			id: sprite.ID,
			x1: sprite.X,
			x2: sprite.X + sprite.Width,
			y1: sprite.Y,
			y2: sprite.Y + sprite.Height,
		}
		yCoords = append(yCoords, sprite.Y, sprite.Y+sprite.Height)
	}
	sort.Slice(rects, func(i, j int) bool {
		if rects[i].x1 != rects[j].x1 {
			return rects[i].x1 < rects[j].x1
		}
		return rects[i].x2 < rects[j].x2
	})
	sort.Ints(yCoords)
	unique := 0
	for _, coordinate := range yCoords {
		if unique == 0 || yCoords[unique-1] != coordinate {
			yCoords[unique] = coordinate
			unique++
		}
	}
	yCoords = yCoords[:unique]
	if len(yCoords) < 2 {
		return nil
	}

	tree := newIntervalMaxTree(len(yCoords) - 1)
	active := spriteOverlapHeap{}
	heap.Init(&active)
	for _, rect := range rects {
		for active.Len() > 0 && active[0].x2 <= rect.x1 {
			old := heap.Pop(&active).(activeSpriteOverlap)
			tree.add(old.y1, old.y2, -1)
		}

		y1 := sort.SearchInts(yCoords, rect.y1)
		y2 := sort.SearchInts(yCoords, rect.y2)
		if tree.query(y1, y2) > 0 {
			// Find the matching active ID only on the error path. The sweep and
			// tree do the normal case in O(n log n), while this keeps the
			// existing diagnostic useful without storing IDs in every tree node.
			for _, candidate := range active {
				if candidate.y1 < y2 && y1 < candidate.y2 {
					return fmt.Errorf("spritesheet sprites %d and %d overlap", candidate.id, rect.id)
				}
			}
			return fmt.Errorf("spritesheet sprite %d overlaps another sprite", rect.id)
		}
		tree.add(y1, y2, 1)
		heap.Push(&active, activeSpriteOverlap{x2: rect.x2, y1: y1, y2: y2, id: rect.id})
	}
	return nil
}

type spriteOverlapRect struct {
	id     int
	x1, x2 int
	y1, y2 int
}

type activeSpriteOverlap struct {
	x2 int
	y1 int
	y2 int
	id int
}

type spriteOverlapHeap []activeSpriteOverlap

func (h spriteOverlapHeap) Len() int           { return len(h) }
func (h spriteOverlapHeap) Less(i, j int) bool { return h[i].x2 < h[j].x2 }
func (h spriteOverlapHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *spriteOverlapHeap) Push(value any)    { *h = append(*h, value.(activeSpriteOverlap)) }
func (h *spriteOverlapHeap) Pop() any {
	old := *h
	n := len(old)
	value := old[n-1]
	*h = old[:n-1]
	return value
}

type intervalMaxTree struct {
	max  []int
	lazy []int
	n    int
}

func newIntervalMaxTree(n int) *intervalMaxTree {
	return &intervalMaxTree{
		max:  make([]int, 4*maxInt(n, 1)),
		lazy: make([]int, 4*maxInt(n, 1)),
		n:    n,
	}
}

// add applies delta to the half-open segment range [queryLeft, queryRight).
func (t *intervalMaxTree) add(queryLeft, queryRight, delta int) {
	if t.n == 0 || queryLeft >= queryRight {
		return
	}
	t.addNode(1, 0, t.n, queryLeft, queryRight, delta)
}

func (t *intervalMaxTree) addNode(node, left, right, queryLeft, queryRight, delta int) {
	if queryLeft >= right || queryRight <= left {
		return
	}
	if queryLeft <= left && right <= queryRight {
		t.max[node] += delta
		t.lazy[node] += delta
		return
	}
	t.push(node)
	middle := (left + right) / 2
	t.addNode(node*2, left, middle, queryLeft, queryRight, delta)
	t.addNode(node*2+1, middle, right, queryLeft, queryRight, delta)
	t.max[node] = max(t.max[node*2], t.max[node*2+1])
}

// query returns the maximum active-rectangle count in [queryLeft, queryRight).
func (t *intervalMaxTree) query(queryLeft, queryRight int) int {
	if t.n == 0 || queryLeft >= queryRight {
		return 0
	}
	return t.queryNode(1, 0, t.n, queryLeft, queryRight)
}

func (t *intervalMaxTree) queryNode(node, left, right, queryLeft, queryRight int) int {
	if queryLeft >= right || queryRight <= left {
		return 0
	}
	if queryLeft <= left && right <= queryRight {
		return t.max[node]
	}
	t.push(node)
	middle := (left + right) / 2
	return max(t.queryNode(node*2, left, middle, queryLeft, queryRight), t.queryNode(node*2+1, middle, right, queryLeft, queryRight))
}

func (t *intervalMaxTree) push(node int) {
	if t.lazy[node] == 0 {
		return
	}
	delta := t.lazy[node]
	for _, child := range []int{node * 2, node*2 + 1} {
		t.max[child] += delta
		t.lazy[child] += delta
	}
	t.lazy[node] = 0
}

func validateMap(mapData Map, spriteN int) error {
	if mapData.Width < 0 || mapData.Height < 0 {
		return fmt.Errorf("map dimensions must be nonnegative, got %dx%d", mapData.Width, mapData.Height)
	}
	cellN, err := checkedProduct(mapData.Width, mapData.Height)
	if err != nil {
		return fmt.Errorf("map: %w", err)
	}
	if mapData.Version == 0 {
		if len(mapData.Layers) != 0 {
			return fmt.Errorf("legacy map must not contain layers")
		}
		if len(mapData.Cells) != cellN {
			return fmt.Errorf("map has %d cells, want %d", len(mapData.Cells), cellN)
		}
		return validateMapCells("map", mapData.Cells, spriteN)
	}
	if mapData.Version != MapSchemaVersion {
		return fmt.Errorf("unsupported map schema version %d", mapData.Version)
	}
	if len(mapData.Cells) != 0 {
		return fmt.Errorf("map schema v%d must store cells inside layers", MapSchemaVersion)
	}
	if len(mapData.Layers) != MapLayerCount {
		return fmt.Errorf("map schema v%d has %d layers, want %d", MapSchemaVersion, len(mapData.Layers), MapLayerCount)
	}
	seen := [MapLayerCount]bool{}
	for _, layer := range mapData.Layers {
		if layer.ID < 0 || layer.ID >= MapLayerCount {
			return fmt.Errorf("map layer id %d is outside BG0..BG3", layer.ID)
		}
		if seen[layer.ID] {
			return fmt.Errorf("map layer BG%d is duplicated", layer.ID)
		}
		seen[layer.ID] = true
		if len(layer.Cells) != cellN {
			return fmt.Errorf("map layer BG%d has %d cells, want %d", layer.ID, len(layer.Cells), cellN)
		}
		if err := validateMapCells(fmt.Sprintf("map layer BG%d", layer.ID), layer.Cells, spriteN); err != nil {
			return err
		}
	}
	return nil
}

func validateMapCells(label string, cells []int, spriteN int) error {
	for i, cell := range cells {
		if cell < 0 || cell >= spriteN {
			return fmt.Errorf("%s cell %d has sprite id %d outside 0..%d", label, i, cell, maxInt(spriteN-1, 0))
		}
	}
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func readJSON(path string, dst any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".assetsync-*")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", filepath.Base(path), err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
