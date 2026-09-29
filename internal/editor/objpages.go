package editor

import (
	"fmt"
	"sort"

	"github.com/drpaneas/mtile/internal/assetsync"
)

const ObjPageWidth = assetsync.DefaultObjSheetW
const ObjPageHeight = assetsync.DefaultObjSheetH
const objPageCols = ObjPageWidth / 8
const objPageRows = ObjPageHeight / 8
const objPageCells = objPageCols * objPageRows
const objMaxSheetHeight = assetsync.MaxObjSheetCells / objPageCols * 8

// The last board contains the remaining complete rows. A 64-cell row divides
// the 65,536-cell address space exactly, so no source tile slots are lost.
func objPageHeight(index int) int {
	return min(ObjPageHeight, max(0, objMaxSheetHeight-index*ObjPageHeight))
}

func canonicalObjSheetDimensions(width, height int) bool {
	return width == ObjPageWidth && height > 0 && height <= objMaxSheetHeight && (height%ObjPageHeight == 0 || height == objMaxSheetHeight)
}

// EnsureObjPages only supplies metadata for a canonical page bank. Legacy banks
// must pass through NormalizeObjPages, whose errors cannot be silently ignored.
func (m *Model) EnsureObjPages() {
	if m == nil || m.ObjSheet == nil || len(m.Animation.Pages) != 0 || !canonicalObjSheetDimensions(m.ObjSheet.Width, m.ObjSheet.Height) {
		return
	}
	m.Animation.Version = 1
	for y := 0; y < m.ObjSheet.Height; y += ObjPageHeight {
		m.Animation.Pages = append(m.Animation.Pages, assetsync.AnimationPage{Name: fmt.Sprintf("OBJ %d", len(m.Animation.Pages)+1), Y: y, Width: ObjPageWidth, Height: objPageHeight(len(m.Animation.Pages))})
	}
}

type objPageCell struct {
	pixels [64]int
	flags  int
}

// NormalizeObjPages repacks legacy artwork into 512x320-pixel art boards, with
// a smaller terminal board when the 65,536 source tile slots are exhausted.
// Shared source references remain shared; pose IDs, local offsets, flips, and
// clip playback metadata remain unchanged. The model changes only on success.
func (m *Model) NormalizeObjPages() error {
	if m == nil || m.ObjSheet == nil {
		return nil
	}
	old := m.ObjSheet
	if err := assetsync.ValidateObjSheetDimensions(old.Width, old.Height); err != nil {
		return err
	}
	oldCols, oldN := old.Width/8, old.Width/8*(old.Height/8)
	for i, pose := range m.Poses.Poses {
		for _, part := range pose.Parts {
			if part.Tile < 0 || part.Tile >= oldN {
				return fmt.Errorf("cannot normalize animation pages: frame %d uses tile %d outside its source sheet", i, part.Tile)
			}
		}
	}
	uniform := canonicalObjSheetDimensions(old.Width, old.Height)
	if uniform && len(m.Animation.Pages) > 0 {
		uniform = len(m.Animation.Pages) == (old.Height+ObjPageHeight-1)/ObjPageHeight
		for i, p := range m.Animation.Pages {
			uniform = uniform && p.X == 0 && p.Y == i*ObjPageHeight && p.Width == ObjPageWidth && p.Height == objPageHeight(i)
		}
	}
	if uniform {
		m.EnsureObjPages()
		return nil
	}

	cells := make(map[int]*objPageCell)
	cell := func(id int) *objPageCell {
		if cells[id] == nil {
			cells[id] = &objPageCell{}
		}
		return cells[id]
	}
	for _, sprite := range old.Sprites {
		if sprite.ID < 0 || sprite.ID >= oldN || sprite.X < 0 || sprite.Y < 0 || sprite.Width < 1 || sprite.Height < 1 || sprite.X > old.Width-sprite.Width || sprite.Y > old.Height-sprite.Height || len(sprite.Pixels) != sprite.Height {
			return fmt.Errorf("cannot normalize animation pages: invalid sprite %d", sprite.ID)
		}
		if sprite.Flags != 0 {
			cell(sprite.ID).flags = sprite.Flags
		}
		for y, row := range sprite.Pixels {
			if len(row) != sprite.Width {
				return fmt.Errorf("cannot normalize animation pages: invalid pixel row in sprite %d", sprite.ID)
			}
			for x, color := range row {
				if color != 0 {
					px, py := sprite.X+x, sprite.Y+y
					cell(py/8*oldCols + px/8).pixels[py%8*8+px%8] = color
				}
			}
		}
	}

	// Components keep each ordinary frame's source rectangle together. A
	// source tile referenced by several frames receives exactly one new ID.
	parents := make([]int, oldN)
	for id := range parents {
		parents[id] = -1
	}
	var find func(int) int
	find = func(id int) int {
		for parents[id] != id {
			parents[id] = parents[parents[id]]
			id = parents[id]
		}
		return id
	}
	for _, pose := range m.Poses.Poses {
		first := -1
		for _, part := range pose.Parts {
			id := part.Tile
			if parents[id] < 0 {
				parents[id] = id
			}
			if first < 0 {
				first = id
			} else {
				parents[find(id)] = find(first)
			}
		}
	}
	groups := make([][]int, 0)
	groupIndex := make(map[int]int)
	needed := make([]int, 0, len(cells))
	for id := 0; id < oldN; id++ {
		if parents[id] >= 0 {
			root := find(id)
			index, ok := groupIndex[root]
			if !ok {
				index = len(groups)
				groupIndex[root] = index
				groups = append(groups, nil)
			}
			groups[index] = append(groups[index], id)
		}
		if parents[id] >= 0 || cells[id] != nil {
			needed = append(needed, id)
		}
	}
	// Keep authored page layouts first, including unfinished art that has not
	// been registered as a pose. Reserving their blank cells preserves spacing
	// between painted cells instead of scattering spare artwork into tile strips.
	var mapping []int
	var pageCount int
	ok := false
	if pageGroups := legacyObjPageGroups(m.Animation.Pages, oldCols, oldN); pageGroups != nil {
		mapping, pageCount, ok = packObjPageCells(oldCols, oldN, needed, pageGroups, true)
	}
	if !ok {
		mapping, pageCount, ok = packObjPageCells(oldCols, oldN, needed, groups, true)
	}
	if !ok {
		// Rectangle padding must never discard otherwise addressable art.
		// Unusual oversized/shared layouts use compact cells instead.
		mapping, pageCount, ok = packObjPageCells(oldCols, oldN, needed, groups, false)
	}
	if !ok {
		return fmt.Errorf("cannot normalize animation pages: artwork exceeds %d tiles", assetsync.MaxObjSheetCells)
	}
	resized := &assetsync.Spritesheet{Width: ObjPageWidth, Height: min(pageCount*ObjPageHeight, objMaxSheetHeight)}
	for _, oldID := range needed {
		data := cells[oldID]
		if data == nil {
			continue
		}
		id := mapping[oldID]
		sprite := newCellSprite(id, id%objPageCols*8, id/objPageCols*8)
		sprite.Flags = data.flags
		for y := range sprite.Pixels {
			copy(sprite.Pixels[y], data.pixels[y*8:(y+1)*8])
		}
		resized.Sprites = append(resized.Sprites, sprite)
	}
	sort.Slice(resized.Sprites, func(i, j int) bool { return resized.Sprites[i].ID < resized.Sprites[j].ID })
	poses := assetsync.PoseFile{Version: m.Poses.Version, Poses: append([]assetsync.PoseRecord(nil), m.Poses.Poses...)}
	for i := range poses.Poses {
		poses.Poses[i].Parts = append([]assetsync.PosePartRecord(nil), poses.Poses[i].Parts...)
		for j := range poses.Poses[i].Parts {
			part := &poses.Poses[i].Parts[j]
			part.Tile = mapping[part.Tile]
		}
	}
	animation := m.Animation
	animation.Version = 1
	animation.Pages = make([]assetsync.AnimationPage, pageCount)
	usedNames := make(map[string]bool)
	for i := range animation.Pages {
		name := fmt.Sprintf("OBJ %d", i+1)
		if i < len(m.Animation.Pages) {
			name = m.Animation.Pages[i].Name
		}
		for suffix := i + 1; name == "" || usedNames[name]; suffix++ {
			name = fmt.Sprintf("OBJ %d", suffix)
		}
		usedNames[name] = true
		animation.Pages[i] = assetsync.AnimationPage{Name: name, Y: i * ObjPageHeight, Width: ObjPageWidth, Height: objPageHeight(i)}
	}
	if err := assetsync.ValidateAnimationWorkspace(animation, resized.Width, resized.Height, len(poses.Poses)); err != nil {
		return err
	}
	m.ObjSheet, m.Poses, m.Animation = resized, poses, animation
	return nil
}

// legacyObjPageGroups returns nonoverlapping complete source rectangles that
// can fit on the new boards. Invalid or oversized legacy layouts use the pose
// packing fallback; page padding must never consume otherwise usable capacity.
func legacyObjPageGroups(pages []assetsync.AnimationPage, oldCols, oldN int) [][]int {
	if len(pages) == 0 {
		return nil
	}
	groups := make([][]int, 0, len(pages))
	seen := make([]bool, oldN)
	for _, p := range pages {
		if p.X < 0 || p.Y < 0 || p.Width <= 0 || p.Height <= 0 || p.X%8 != 0 || p.Y%8 != 0 || p.Width%8 != 0 || p.Height%8 != 0 || p.Width > ObjPageWidth || p.Height > ObjPageHeight || p.X > oldCols*8-p.Width || p.Y > oldN/oldCols*8-p.Height {
			return nil
		}
		group := make([]int, 0, p.Width/8*(p.Height/8))
		for y := p.Y / 8; y < (p.Y+p.Height)/8; y++ {
			for x := p.X / 8; x < (p.X+p.Width)/8; x++ {
				id := y*oldCols + x
				if seen[id] {
					return nil
				}
				seen[id] = true
				group = append(group, id)
			}
		}
		groups = append(groups, group)
	}
	return groups
}

func packObjPageCells(oldCols, oldN int, needed []int, groups [][]int, rectangles bool) ([]int, int, bool) {
	type page struct {
		rows [objPageRows]uint64
		free int
	}
	pages := []page{{free: objPageCells}}
	mapping := make([]int, oldN)
	for i := range mapping {
		mapping[i] = -1
	}
	reserve := func(width, height int) (int, bool) {
		for index := 0; index <= len(pages); index++ {
			if index == len(pages) {
				if objPageHeight(len(pages)) == 0 {
					return 0, false
				}
				pages = append(pages, page{free: objPageHeight(len(pages)) / 8 * objPageCols})
			}
			p := &pages[index]
			if p.free < width*height {
				continue
			}
			for y := 0; y <= objPageHeight(index)/8-height; y++ {
				for x := 0; x <= objPageCols-width; x++ {
					mask := (^uint64(0) >> uint(64-width)) << uint(x)
					clear := true
					for row := y; row < y+height; row++ {
						clear = clear && p.rows[row]&mask == 0
					}
					if clear {
						for row := y; row < y+height; row++ {
							p.rows[row] |= mask
						}
						p.free -= width * height
						return index*objPageCells + y*objPageCols + x, true
					}
				}
			}
		}
		return 0, false
	}
	if rectangles {
		for _, group := range groups {
			if len(group) < 2 {
				continue
			}
			left, top, right, bottom := oldCols, oldN/oldCols, 0, 0
			for _, id := range group {
				x, y := id%oldCols, id/oldCols
				left, top, right, bottom = min(left, x), min(top, y), max(right, x), max(bottom, y)
			}
			width, height := right-left+1, bottom-top+1
			if width > objPageCols || height > objPageRows {
				continue
			}
			base, ok := reserve(width, height)
			if !ok {
				return nil, 0, false
			}
			for _, id := range group {
				mapping[id] = base + (id/oldCols-top)*objPageCols + id%oldCols - left
			}
		}
	}
	next := 0
	for _, id := range needed {
		if mapping[id] >= 0 {
			continue
		}
		for {
			if next == assetsync.MaxObjSheetCells {
				return nil, 0, false
			}
			index, local := next/objPageCells, next%objPageCells
			if index == len(pages) {
				pages = append(pages, page{free: objPageHeight(len(pages)) / 8 * objPageCols})
			}
			row, bit := local/objPageCols, uint64(1)<<uint(local%objPageCols)
			next++
			if pages[index].rows[row]&bit == 0 {
				pages[index].rows[row] |= bit
				pages[index].free--
				mapping[id] = next - 1
				break
			}
		}
	}
	return mapping, len(pages), true
}

// NextObjPageDimensions describes the next blank board in a normalized bank.
// Zero dimensions mean no board can be added. This query never changes artwork.
func (m *Model) NextObjPageDimensions() (width, height int) {
	if m == nil || m.ObjSheet == nil || !canonicalObjSheetDimensions(m.ObjSheet.Width, m.ObjSheet.Height) {
		return 0, 0
	}
	height = min(ObjPageHeight, objMaxSheetHeight-m.ObjSheet.Height)
	if height <= 0 {
		return 0, 0
	}
	return ObjPageWidth, height
}

// AddObjPage appends a blank art board after transactional normalization. The
// fixed row stride keeps every existing tile reference and pixel unchanged.
func (m *Model) AddObjPage() (int, error) {
	if m == nil || m.ObjSheet == nil {
		return 0, fmt.Errorf("open Animation before adding a page")
	}
	work := *m
	if err := work.NormalizeObjPages(); err != nil {
		return 0, err
	}
	y := work.ObjSheet.Height
	width, height := work.NextObjPageDimensions()
	if height == 0 {
		return 0, fmt.Errorf("cannot add a page: all %d source tile slots are allocated", assetsync.MaxObjSheetCells)
	}
	if err := assetsync.ValidateObjSheetDimensions(width, y+height); err != nil {
		return 0, fmt.Errorf("cannot add a page: %w", err)
	}
	sheet := *work.ObjSheet
	sheet.Height += height
	work.ObjSheet = &sheet
	index := len(work.Animation.Pages)
	name := fmt.Sprintf("OBJ %d", index+1)
	for suffix := index + 1; ; suffix++ {
		name = fmt.Sprintf("OBJ %d", suffix)
		used := false
		for _, p := range work.Animation.Pages {
			used = used || p.Name == name
		}
		if !used {
			break
		}
	}
	work.Animation.Pages = append(append([]assetsync.AnimationPage(nil), work.Animation.Pages...), assetsync.AnimationPage{
		Name: name, Y: y, Width: width, Height: height,
	})
	m.ObjSheet, m.Poses, m.Animation = work.ObjSheet, work.Poses, work.Animation
	return index, nil
}

func ObjPageContainsTile(page assetsync.AnimationPage, tile, sheetCols int) bool {
	if tile < 0 || sheetCols <= 0 {
		return false
	}
	x, y := tile%sheetCols*8, tile/sheetCols*8
	return x >= page.X && y >= page.Y && x < page.X+page.Width && y < page.Y+page.Height
}

func PoseIDsOnObjPage(poses []assetsync.PoseRecord, page assetsync.AnimationPage, sheetCols int) []int {
	var ids []int
	for id, pose := range poses {
		for _, part := range pose.Parts {
			if ObjPageContainsTile(page, part.Tile, sheetCols) {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids
}
