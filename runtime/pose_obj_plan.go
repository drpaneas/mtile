package runtime

// PoseOBJ is a compiler-planned hardware object. TileOffset names the first
// row-major cell in its pose's streamed tile list; dimensions are pixels.
type PoseOBJ struct {
	X, Y       int16
	TileOffset uint16
	Width      uint8
	Height     uint8
	Flags      uint8
	// Palette is zero for 8bpp, or bank+1 for resident 4bpp objects.
	Palette uint8
}

// PoseOBJRange addresses a pose's immutable object and source-tile plans.
type PoseOBJRange struct {
	Start, Count         uint32
	TileStart, TileCount uint32
}

// BuildPoseOBJPlan runs at build time. Disjoint, unflipped cells can share a
// larger hardware object; overlapping poses retain their authored painter
// order as individual cells. The runtime reads the resulting ROM tables.
func BuildPoseOBJPlan(parts []PosePart) (objects []PoseOBJ, tiles []uint16) {
	pack := true
	buckets := make(map[[2]int][]int, len(parts))
	for i, a := range parts {
		// Arithmetic shifts round negative coordinates toward minus infinity.
		// Each cell touches at most four buckets, avoiding an all-pairs scan
		// when importing a large pose.
		for by := int(a.Y) >> 3; by <= (int(a.Y)+7)>>3 && pack; by++ {
			for bx := int(a.X) >> 3; bx <= (int(a.X)+7)>>3 && pack; bx++ {
				key := [2]int{bx, by}
				for _, index := range buckets[key] {
					b := parts[index]
					if int(a.X) < int(b.X)+8 && int(b.X) < int(a.X)+8 &&
						int(a.Y) < int(b.Y)+8 && int(b.Y) < int(a.Y)+8 {
						pack = false
						break
					}
				}
				buckets[key] = append(buckets[key], i)
			}
		}
		if !pack {
			break
		}
	}
	positions := make(map[[2]int]int, len(parts))
	for i, part := range parts {
		positions[[2]int{int(part.X), int(part.Y)}] = i
	}
	used := make([]bool, len(parts))
	firstTile := make(map[uint16]int, len(parts))
	// All twelve regular GBA object shapes, largest first. Equal-area shapes
	// use a stable order so compiler budgets and generated assets agree.
	shapes := [...][2]int{{8, 8}, {8, 4}, {4, 8}, {4, 4}, {4, 2}, {2, 4}, {4, 1}, {1, 4}, {2, 2}, {2, 1}, {1, 2}, {1, 1}}
	for i, part := range parts {
		if used[i] {
			continue
		}
		width, height := 1, 1
		if pack && part.Flags == 0 {
			for _, shape := range shapes {
				fits := true
				for row := 0; row < shape[1] && fits; row++ {
					for col := 0; col < shape[0]; col++ {
						index, exists := positions[[2]int{int(part.X) + col*8, int(part.Y) + row*8}]
						if !exists || used[index] || parts[index].Flags != 0 {
							fits = false
							break
						}
					}
				}
				if fits {
					width, height = shape[0], shape[1]
					break
				}
			}
		}
		start := len(tiles)
		if width == 1 && height == 1 {
			// Repeated singleton art still shares a resident cell.
			if index, exists := firstTile[part.Tile]; exists {
				start = index
			}
			if start == len(tiles) {
				firstTile[part.Tile] = start
				tiles = append(tiles, part.Tile)
			}
			used[i] = true
		} else {
			for row := 0; row < height; row++ {
				for col := 0; col < width; col++ {
					index := positions[[2]int{int(part.X) + col*8, int(part.Y) + row*8}]
					if _, exists := firstTile[parts[index].Tile]; !exists {
						firstTile[parts[index].Tile] = len(tiles)
					}
					tiles = append(tiles, parts[index].Tile)
					used[index] = true
				}
			}
		}
		objects = append(objects, PoseOBJ{X: part.X, Y: part.Y, TileOffset: uint16(start),
			Width: uint8(width * 8), Height: uint8(height * 8), Flags: part.Flags})
	}
	return objects, tiles
}

func mode0PoseShape(width, height int) (shape, size uint16, valid bool) {
	switch {
	case width == 8 && height == 8:
		return 0, 0, true
	case width == 16 && height == 16:
		return 0, 1, true
	case width == 32 && height == 32:
		return 0, 2, true
	case width == 64 && height == 64:
		return 0, 3, true
	case width == 16 && height == 8:
		return 1, 0, true
	case width == 32 && height == 8:
		return 1, 1, true
	case width == 32 && height == 16:
		return 1, 2, true
	case width == 64 && height == 32:
		return 1, 3, true
	case width == 8 && height == 16:
		return 2, 0, true
	case width == 8 && height == 32:
		return 2, 1, true
	case width == 16 && height == 32:
		return 2, 2, true
	case width == 32 && height == 64:
		return 2, 3, true
	}
	return 0, 0, false
}
