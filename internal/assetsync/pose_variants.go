package assetsync

import "fmt"

func attachPoseColorVariants(a *Assets, file PoseFile) error {
	if len(file.Variants) == 0 {
		return nil
	}
	if a.ObjSheetW == 0 {
		return fmt.Errorf("pose color variants require assets/objsheet.json")
	}
	if (len(file.Variants)+1)*len(a.PoseIndex) > 65535 {
		return fmt.Errorf("too many expanded pose color variants")
	}
	a.PoseVariantBaseCount = len(a.PoseIndex)
	baseIndex := append([]FlatPose(nil), a.PoseIndex...)
	baseParts := append([]FlatPosePart(nil), a.PoseParts...)
	baseSheet := append([]byte(nil), a.ObjSheetROM...)
	cols := a.ObjSheetW / 8
	next := a.ObjSpriteN
	pixels := append([]byte(nil), baseSheet...)
	for _, variant := range file.Variants {
		a.PoseVariantNames = append(a.PoseVariantNames, variant.Name)
		var remap [256]byte
		for i := range remap {
			remap[i] = byte(i)
		}
		for _, swap := range variant.Colors {
			remap[swap.From] = byte(swap.To)
		}
		cells := map[int]int{}
		for _, pose := range baseIndex {
			target := pose
			target.Start = uint32(len(a.PoseParts))
			for _, part := range baseParts[int(pose.Start) : int(pose.Start)+int(pose.Count)] {
				source := int(part.Tile)
				tile, ok := cells[source]
				if !ok {
					tile = next
					next++
					cells[source] = tile
					needed := ((next + cols - 1) / cols) * cols * 64
					if needed > len(pixels) {
						pixels = append(pixels, make([]byte, needed-len(pixels))...)
					}
					for y := 0; y < 8; y++ {
						for x := 0; x < 8; x++ {
							src := (source/cols*8+y)*a.ObjSheetW + source%cols*8 + x
							dst := (tile/cols*8+y)*a.ObjSheetW + tile%cols*8 + x
							if src >= 0 && src < len(baseSheet) {
								pixels[dst] = remap[baseSheet[src]]
							}
						}
					}
				}
				if tile > 65535 {
					return fmt.Errorf("pose color variants exceed object tile address range")
				}
				part.Tile = uint16(tile)
				a.PoseParts = append(a.PoseParts, part)
			}
			a.PoseIndex = append(a.PoseIndex, target)
		}
	}
	a.ObjSheetROM = pixels
	a.ObjSheetH = len(pixels) / a.ObjSheetW
	a.ObjSpriteRows = a.ObjSheetH / 8
	a.ObjSpriteN = a.ObjSpriteCols * a.ObjSpriteRows
	return nil
}
