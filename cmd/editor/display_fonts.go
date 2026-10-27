package main

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type displayFontSource struct {
	parsed *opentype.Font
	size   float64
	base   font.Face
	scale  float64
	face   *text.GoXFace
}

var displayFontSources = map[*text.GoXFace]*displayFontSource{}

func registerDisplayFont(face *text.GoXFace, parsed *opentype.Font, size float64, base font.Face) *text.GoXFace {
	displayFontSources[face] = &displayFontSource{parsed: parsed, size: size, base: base}
	return face
}

func displayFont(face text.Face, scale float64) text.Face {
	if scale == 1 {
		return face
	}
	logical, ok := face.(*text.GoXFace)
	if !ok {
		return face
	}
	source := displayFontSources[logical]
	if source == nil {
		return face
	}
	if source.face == nil || source.scale != scale {
		raster, err := opentype.NewFace(source.parsed, &opentype.FaceOptions{
			Size: source.size * scale, DPI: 72, Hinting: font.HintingFull,
		})
		if err != nil {
			panic("create display font face: " + err.Error())
		}
		// Retain only the current density per face. Moving between monitors
		// replaces the raster cache without accumulating every visited scale.
		source.face = text.NewGoXFace(&displayFontFace{Face: raster, base: source.base, scale: scale})
		source.scale = scale
	}
	return source.face
}

// Rasterize at physical resolution, but preserve logical advances and
// baselines. Hinting a larger font can otherwise change character widths and
// make the code drift away from its caret, selections, and highlighted spans.
type displayFontFace struct {
	font.Face
	base  font.Face
	scale float64
}

func (f *displayFontFace) scaled(value fixed.Int26_6) fixed.Int26_6 {
	return fixed.Int26_6(math.Round(float64(value) * f.scale))
}

func (f *displayFontFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	rect, mask, point, _, ok := f.Face.Glyph(dot, r)
	advance, _ := f.GlyphAdvance(r)
	return rect, mask, point, advance, ok
}

func (f *displayFontFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	bounds, _, ok := f.Face.GlyphBounds(r)
	advance, _ := f.GlyphAdvance(r)
	return bounds, advance, ok
}

func (f *displayFontFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	advance, ok := f.base.GlyphAdvance(r)
	return f.scaled(advance), ok
}

func (f *displayFontFace) Kern(r0, r1 rune) fixed.Int26_6 {
	return f.scaled(f.base.Kern(r0, r1))
}

func (f *displayFontFace) Metrics() font.Metrics {
	m := f.base.Metrics()
	m.Height, m.Ascent, m.Descent = f.scaled(m.Height), f.scaled(m.Ascent), f.scaled(m.Descent)
	m.XHeight, m.CapHeight = f.scaled(m.XHeight), f.scaled(m.CapHeight)
	return m
}
