package transpiler

import "fmt"

// Packed map storage keeps opaque source runs; uniform runs store their color
// in the span itself. Capacities include a bounded allowance for initialization
// edits. Later mutations use live cells without rebuilding inside a frame.
type packedMapPlane struct {
	pixels, spans int
}

func (g *generator) hasPackedMap() bool {
	return g.packedMap != [4]packedMapPlane{}
}

func (g *generator) renderPackedMapStorage(b *sourceBuilder) {
	if !g.hasPackedMap() {
		return
	}
	pixels, spans := 0, 0
	for _, plane := range g.packedMap {
		pixels += plane.pixels
		spans += plane.spans
	}
	// As with the small-map raster cache, use target sizes and reserve both
	// Lua semispaces, the stack, mailbox and a further 8 KiB. Large games get
	// zero-length backing arrays and use ordinary cell rendering.
	budget := "256*1024 - 512 - 8192 - rt.MapRasterHeapReserve - int(unsafe.Sizeof(ctx)) - int(unsafe.Sizeof(cartData)) - int(unsafe.Sizeof(game)) - int(unsafe.Sizeof(sheetRAM)) - int(unsafe.Sizeof(sheetIndexRAM)) - int(unsafe.Sizeof(mapRAM)) - int(unsafe.Sizeof(flagRAM))"
	if g.saveEnabled() {
		budget += " - int(unsafe.Sizeof(saveMemory))"
	}
	if g.nativeAudioEnabled {
		budget += " - int(unsafe.Sizeof(nativeAudioController))"
	}
	b.line("const mapPackedBudget = " + budget)
	b.line(fmt.Sprintf("const mapPackedBytes = %d + %d*int(unsafe.Sizeof(rt.MapPackedSpan{}))", pixels, spans))
	b.line("const mapPackedEnabled = max(0, min(1, mapPackedBudget/mapPackedBytes))")
	// A byte array has no ARM word-alignment guarantee. Keep the payload in
	// words and expose exactly its byte extent, including when disabled.
	b.line(fmt.Sprintf("//go:section .ewram\nvar mapPackedWordsRAM [mapPackedEnabled*%d]uint32", pixels/4))
	b.line(fmt.Sprintf("//go:section .ewram\nvar mapPackedSpansRAM [mapPackedEnabled*%d]rt.MapPackedSpan", spans))
	b.line("")
}

func (g *generator) renderPackedMapBindings(b *sourceBuilder) {
	if !g.hasPackedMap() {
		return
	}
	b.line("\tcartData.MapPackedPixels = unsafe.Slice((*byte)(unsafe.Pointer(&mapPackedWordsRAM)), len(mapPackedWordsRAM)*4)")
	b.line("\tcartData.MapPackedSpans = mapPackedSpansRAM[:]")
	b.line("\tcartData.MapPackedLayers = [4]rt.MapPackedLayer{")
	pixels, spans := 0, 0
	for bg, plane := range g.packedMap {
		if plane.spans != 0 {
			b.line(fmt.Sprintf("\t\t%d: {PixelStart: uint16(mapPackedEnabled*%d), PixelEnd: uint16(mapPackedEnabled*%d), SpanStart: uint16(mapPackedEnabled*%d), SpanEnd: uint16(mapPackedEnabled*%d)},", bg, pixels, pixels+plane.pixels, spans, spans+plane.spans))
		}
		pixels += plane.pixels
		spans += plane.spans
	}
	b.line("\t}")
}

func (g *generator) planPackedMap() [4]packedMapPlane {
	var plan [4]packedMapPlane
	if g.opts.DisableMapRasterCache || g.opts.Renderer == RendererMode0 ||
		g.sheetMutable || g.opts.MapStreaming || g.opts.MapDrawMask == 0 || g.mapRasterEligible() ||
		g.opts.MapW < 1 || g.opts.MapW > 64 || g.opts.MapH < 1 || g.opts.MapH > 32 {
		return plan
	}
	w, h := g.opts.MapW, g.opts.MapH
	storage := 0
	totalPixels, totalSpans := 0, 0
	for bg := 0; bg < 4; bg++ {
		if g.opts.MapPlaneMask&(1<<uint(bg)) == 0 {
			continue
		}
		start := storage * w * h
		storage++
		if g.opts.MapDrawMask&(1<<uint(bg)) == 0 || start+w*h > len(g.mapCompact) {
			continue
		}
		opaquePixels := 0
		for y := 0; y < h*8; y++ {
			run := 0
			var runColor byte
			uniform := true
			for x := 0; x <= w*8; x++ {
				var pixel byte
				if x < w*8 {
					id := int(g.mapCompact[start+(y/8)*w+x/8])
					if id > 0 && id < len(g.sheetIndex) {
						slot := int(g.sheetIndex[id])
						if slot > 0 {
							pixel = g.sheetCompact[(slot-1)*64+(y%8)*8+x%8]
						}
					}
				}
				if pixel != 0 {
					if run == 0 {
						runColor, uniform = pixel, true
					} else if pixel != runColor {
						uniform = false
					}
					run++
					opaquePixels++
				} else if run != 0 {
					if !uniform {
						plan[bg].pixels += (run + 3) &^ 3
					}
					plan[bg].spans++
					run = 0
				}
			}
		}
		// Short runs spend more time decoding/clipping spans than copying
		// pixels. Target comparisons favor the native cell path below this
		// conservative eight-word average; assess each plane independently.
		if opaquePixels < plan[bg].spans*32 {
			plan[bg] = packedMapPlane{}
			continue
		}
		if plan[bg].spans != 0 {
			plan[bg].pixels += 256
			plan[bg].spans += 16
		}
		totalPixels += plan[bg].pixels
		totalSpans += plan[bg].spans
	}
	// Both packed offsets and partition ends are uint16. Every pixel start
	// is word-aligned, including the starts of subsequent layer partitions.
	if totalPixels >= 65536 || totalSpans >= 65536 {
		return [4]packedMapPlane{}
	}
	return plan
}
