package assetsync

import "fmt"

// IsLayered reports whether the map uses the versioned four-background shape.
func (m Map) IsLayered() bool {
	return m.Version == MapSchemaVersion
}

// LayerCount returns the number of stored planes. Legacy maps expose only
// BG0; layered maps always expose all four stable logical backgrounds.
func (m Map) LayerCount() int {
	if m.IsLayered() {
		return MapLayerCount
	}
	return 1
}

// LayerCells returns one plane without allocating. A legacy map is BG0 and
// every other background is transparently empty.
func (m Map) LayerCells(layer int) []int {
	if layer < 0 || layer >= MapLayerCount {
		return nil
	}
	if !m.IsLayered() {
		if layer == 0 {
			return m.Cells
		}
		return nil
	}
	for i := range m.Layers {
		if m.Layers[i].ID == layer {
			return m.Layers[i].Cells
		}
	}
	return nil
}

// Cell reads one background cell. Invalid coordinates, invalid layer IDs,
// and absent legacy layers are transparent sprite ID 0.
func (m Map) Cell(layer, x, y int) int {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return 0
	}
	cells := m.LayerCells(layer)
	index := y*m.Width + x
	if index < 0 || index >= len(cells) {
		return 0
	}
	return cells[index]
}

// PromoteToLayers performs the reversible expand step in memory: BG0 is
// copied byte-for-byte and BG1-BG3 start transparent. It is idempotent.
func (m *Map) PromoteToLayers() error {
	if m == nil {
		return fmt.Errorf("map is required")
	}
	if m.IsLayered() {
		return nil
	}
	if m.Version != 0 {
		return fmt.Errorf("unsupported map schema version %d", m.Version)
	}
	cellN, err := checkedProduct(m.Width, m.Height)
	if err != nil {
		return fmt.Errorf("map: %w", err)
	}
	if len(m.Cells) != cellN {
		return fmt.Errorf("map has %d cells, want %d", len(m.Cells), cellN)
	}
	layers := make([]MapLayer, MapLayerCount)
	for layer := 0; layer < MapLayerCount; layer++ {
		layers[layer] = MapLayer{ID: layer, Cells: make([]int, cellN)}
	}
	copy(layers[0].Cells, m.Cells)
	m.Version = MapSchemaVersion
	m.Cells = nil
	m.Layers = layers
	return nil
}

// SetCell writes one background cell. Editing BG0 keeps a legacy map in its
// compact original shape; the first edit to BG1-BG3 promotes it to v2.
func (m *Map) SetCell(layer, x, y, id int) (bool, error) {
	if m == nil {
		return false, fmt.Errorf("map is required")
	}
	if layer < 0 || layer >= MapLayerCount {
		return false, fmt.Errorf("map layer BG%d is outside BG0..BG3", layer)
	}
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return false, nil
	}
	if layer != 0 && !m.IsLayered() {
		// An absent legacy plane already reads as transparent. Do not mutate the
		// schema merely because the eraser touched it: there is no authored
		// change to mark dirty or place on the undo stack.
		if id == 0 {
			return false, nil
		}
		if err := m.PromoteToLayers(); err != nil {
			return false, err
		}
	}
	cells := m.LayerCells(layer)
	index := y*m.Width + x
	if index < 0 || index >= len(cells) {
		return false, fmt.Errorf("map layer BG%d has invalid cell storage", layer)
	}
	if cells[index] == id {
		return false, nil
	}
	cells[index] = id
	return true, nil
}

// NewLayeredMap creates the exact v2 shape used by the editor after an
// explicit multi-background authoring action.
func NewLayeredMap(width, height int) (Map, error) {
	cellN, err := checkedProduct(width, height)
	if err != nil {
		return Map{}, fmt.Errorf("map: %w", err)
	}
	if width < 0 || height < 0 {
		return Map{}, fmt.Errorf("map dimensions must be nonnegative, got %dx%d", width, height)
	}
	m := Map{Version: MapSchemaVersion, Width: width, Height: height}
	m.Layers = make([]MapLayer, MapLayerCount)
	for layer := 0; layer < MapLayerCount; layer++ {
		m.Layers[layer] = MapLayer{ID: layer, Cells: make([]int, cellN)}
	}
	return m, nil
}
