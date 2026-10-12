package editor

import "fmt"

const MapRoomWidth = 30
const MapRoomHeight = 20

// AddMapRoomEdge grows the rectangular map by one room-wide column or
// room-high row. Left/top additions shift every stored background together.
func (m *Model) AddMapRoomEdge(dx, dy int) error {
	if m == nil || !((dx == -1 || dx == 1) && dy == 0 || (dy == -1 || dy == 1) && dx == 0) {
		return fmt.Errorf("choose one map edge")
	}
	old := m.Map
	w, h := old.Width, old.Height
	if dx != 0 {
		w += MapRoomWidth
	}
	if dy != 0 {
		h += MapRoomHeight
	}
	if err := m.ResizeMap(w, h); err != nil {
		return err
	}
	ox, oy := 0, 0
	if dx < 0 {
		ox = MapRoomWidth
	}
	if dy < 0 {
		oy = MapRoomHeight
	}
	for bg := 0; bg < old.LayerCount(); bg++ {
		target := m.Map.LayerCells(bg)
		clear(target)
		source := old.LayerCells(bg)
		for y := 0; y < old.Height; y++ {
			copy(target[(y+oy)*w+ox:(y+oy)*w+ox+old.Width], source[y*old.Width:(y+1)*old.Width])
		}
	}
	return nil
}

// DeleteMapRoom removes a room from a horizontal/vertical strip and closes
// the gap. A 2D grid keeps its coordinates: clearing one room never deletes
// unrelated rooms in its row/column. The final room is cleared, not removed.
func (m *Model) DeleteMapRoom(col, row int) error {
	if m == nil || col < 0 || row < 0 || col*MapRoomWidth >= m.Map.Width || row*MapRoomHeight >= m.Map.Height {
		return fmt.Errorf("select a room inside the map")
	}
	old := m.Map
	x, y := col*MapRoomWidth, row*MapRoomHeight
	cw, ch := min(MapRoomWidth, old.Width-x), min(MapRoomHeight, old.Height-y)
	horizontal := old.Height <= MapRoomHeight && old.Width > MapRoomWidth
	vertical := old.Width <= MapRoomWidth && old.Height > MapRoomHeight
	if !horizontal && !vertical {
		for bg := 0; bg < old.LayerCount(); bg++ {
			cells := m.Map.LayerCells(bg)
			for dy := 0; dy < ch; dy++ {
				clear(cells[(y+dy)*old.Width+x : (y+dy)*old.Width+x+cw])
			}
		}
		return nil
	}
	w, h := old.Width, old.Height
	if horizontal {
		w -= cw
	} else {
		h -= ch
	}
	if err := m.ResizeMap(w, h); err != nil {
		return err
	}
	for bg := 0; bg < old.LayerCount(); bg++ {
		src, dst := old.LayerCells(bg), m.Map.LayerCells(bg)
		for ty := 0; ty < h; ty++ {
			for tx := 0; tx < w; tx++ {
				sx, sy := tx, ty
				if horizontal && tx >= x {
					sx += cw
				}
				if vertical && ty >= y {
					sy += ch
				}
				dst[ty*w+tx] = src[sy*old.Width+sx]
			}
		}
	}
	return nil
}
