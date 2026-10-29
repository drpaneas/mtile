package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type fontEditorState struct {
	strokeChanged                      bool
	open, menu                         bool
	focus                              int // 1 preview, 2 new font name, 3 glyph character
	preview, name, glyphInput, message string
	selected, firstRow                 int
	stroke                             bool
	strokeOn                           bool
}

type fontEditorLayout struct {
	Pixel                                                                                                int
	Panel, Close, Select, Preview, PreviewText, Name, Copy, Blank, Canvas, Glyphs, GlyphInput, Add, Save editor.Rect
}

func newFontEditorLayout() fontEditorLayout {
	p := editor.Rect{W: min(1000, screenWidth-40), H: min(730, screenHeight-70)}
	p.X = (screenWidth - p.W) / 2
	p.Y = (screenHeight - p.H) / 2
	left := p.X + 24
	top := p.Y + 76
	right := p.X + p.W - 24
	l := fontEditorLayout{Panel: p, Close: editor.Rect{X: right - 84, Y: p.Y + 16, W: 84, H: 34}}
	l.Select = editor.Rect{X: left, Y: top, W: 260, H: 36}
	l.Name = editor.Rect{X: left + 278, Y: top, W: p.W - 526, H: 36}
	l.Copy = editor.Rect{X: right - 196, Y: top, W: 92, H: 36}
	l.Blank = editor.Rect{X: right - 96, Y: top, W: 96, H: 36}
	l.PreviewText = editor.Rect{X: left, Y: top + 58, W: p.W - 48, H: 36}
	l.Preview = editor.Rect{X: left, Y: top + 106, W: p.W - 48, H: 84}
	l.Pixel = min(28, max(16, (p.H-466)/10))
	l.Canvas = editor.Rect{X: left, Y: top + 226, W: 6 * l.Pixel, H: 10 * l.Pixel}
	l.GlyphInput = editor.Rect{X: left, Y: l.Canvas.Y + l.Canvas.H + 14, W: 60, H: 34}
	l.Add = editor.Rect{X: left + 68, Y: l.GlyphInput.Y, W: 100, H: 34}
	l.Glyphs = editor.Rect{X: left + 196, Y: top + 218, W: p.W - 244, H: p.H - 394}
	l.Save = editor.Rect{X: right - 126, Y: p.Y + p.H - 50, W: 126, H: 34}
	return l
}
func fontButtonRect() editor.Rect {
	last := editorTabRect(modeCode)
	return editor.Rect{X: last.X + last.W + 12, Y: 5, W: 86, H: editorTabsHeight - 10}
}
func (g *Game) openFontEditor() {
	g.preferencesOpen = false
	g.fontEditor.open = true
	g.fontEditor.menu = false
	g.fontEditor.focus = 1
	if g.fontEditor.preview == "" {
		g.fontEditor.preview = "Hello GBA!  Ελλάδα  Grüße"
	}
	g.fontEditor.selected = 'A'
	g.fontEditor.message = "Choose a font. COPY to edit, or BLANK to start your own."
	g.developGameFocus = false
	g.requestRender()
}
func (g *Game) fontChanged() {
	found := false
	for _, glyph := range g.model.ActiveFont().Glyphs {
		if glyph.Codepoint == g.fontEditor.selected {
			found = true
			break
		}
	}
	if !found {
		g.fontEditor.selected = 'A'
	}
	g.fontEditor.message = "Font changed. SAVE writes the project; the next build uses this font."
	g.undo.forcePush(g.snapshot())
	g.requestRender()
}
func (g *Game) fontAction(action string) {
	var err error
	switch action {
	case "copy", "blank":
		err = g.model.CreateFont(g.fontEditor.name, action == "blank")
	case "add":
		runes := []rune(g.fontEditor.glyphInput)
		if len(runes) != 1 {
			err = fmt.Errorf("type exactly one precomposed character")
		} else {
			for _, glyph := range g.model.ActiveFont().Glyphs {
				if glyph.Codepoint == int(runes[0]) {
					g.fontEditor.selected = int(runes[0])
					g.fontEditor.message = "Character selected. Paint inside the highlighted bitmap."
					g.requestRender()
					return
				}
			}
			err = g.model.AddFontGlyph(int(runes[0]))
			if err == nil {
				g.fontEditor.selected = int(runes[0])
			}
		}
	case "save":
		err = g.model.Save()
		if err == nil {
			g.queueProjectCheck()
			g.fontEditor.message = "Saved. Only this selected font will be linked into the cartridge."
			return
		}
	}
	if err != nil {
		g.fontEditor.message = err.Error()
		return
	}
	g.fontChanged()
}
func (g *Game) updateFontEditor() {
	s := &g.fontEditor
	l := newFontEditorLayout()
	mx, my := editorCursorPosition()
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if s.menu {
			s.menu = false
		} else {
			s.open = false
		}
		return
	}
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		g.fontAction("save")
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		g.handleUndoRedoKeys()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		s.focus = s.focus%3 + 1
	}
	if s.focus != 0 {
		value := &s.preview
		if s.focus == 2 {
			value = &s.name
		}
		if s.focus == 3 {
			value = &s.glyphInput
		}
		typed := string(ebiten.AppendInputChars(nil))
		if mod && inpututil.IsKeyJustPressed(ebiten.KeyV) {
			if text, err := hostclipboard.Read(); err == nil {
				typed = text
			}
		}
		if mod && inpututil.IsKeyJustPressed(ebiten.KeyA) {
			*value = ""
		}
		for _, ch := range typed {
			if !unicode.IsControl(ch) {
				*value += string(ch)
			}
		}
		limit := 160
		if s.focus == 2 {
			limit = 32
		}
		if s.focus == 3 {
			limit = 1
		}
		runes := []rune(*value)
		if len(runes) > limit {
			*value = string(runes[:limit])
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
			_, n := utf8.DecodeLastRuneInString(*value)
			*value = (*value)[:len(*value)-n]
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) && s.focus == 3 {
			g.fontAction("add")
		}
	}
	names := g.model.FontNames()
	if s.menu {
		_, wheel := ebiten.Wheel()
		s.firstRow = max(0, min(max(0, len(names)-6), s.firstRow-int(wheel)))
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			for i := s.firstRow; i < min(len(names), s.firstRow+6); i++ {
				r := l.Select
				r.Y += r.H + (i-s.firstRow)*34
				r.H = 34
				if rectContains(r, mx, my) {
					if err := g.model.SelectFont(names[i]); err != nil {
						s.message = err.Error()
					} else {
						g.fontChanged()
					}
					s.menu = false
					return
				}
			}
			s.menu = false
		}
		return
	}
	if rectContains(l.Glyphs, mx, my) {
		_, wheel := ebiten.Wheel()
		f := g.model.ActiveFont()
		cols := max(1, l.Glyphs.W/36)
		rows := max(1, l.Glyphs.H/30)
		s.firstRow = max(0, min(max(0, (len(f.Glyphs)+cols-1)/cols-rows), s.firstRow-int(wheel)))
	}
	pressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	if pressed {
		s.focus = 0
		switch {
		case rectContains(l.Close, mx, my):
			s.open = false
			return
		case rectContains(l.Select, mx, my):
			s.menu = true
			s.firstRow = 0
			return
		case rectContains(l.PreviewText, mx, my):
			s.focus = 1
		case rectContains(l.Name, mx, my):
			s.focus = 2
		case rectContains(l.GlyphInput, mx, my):
			s.focus = 3
		case rectContains(l.Copy, mx, my):
			g.fontAction("copy")
		case rectContains(l.Blank, mx, my):
			g.fontAction("blank")
		case rectContains(l.Add, mx, my):
			g.fontAction("add")
		case rectContains(l.Save, mx, my):
			g.fontAction("save")
		case rectContains(l.Glyphs, mx, my):
			f := g.model.ActiveFont()
			cols := max(1, l.Glyphs.W/36)
			idx := (s.firstRow+(my-l.Glyphs.Y)/30)*cols + (mx-l.Glyphs.X)/36
			if idx < len(f.Glyphs) {
				s.selected = f.Glyphs[idx].Codepoint
			}
		}
	}
	down := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if down && rectContains(l.Canvas, mx, my) {
		x, y := (mx-l.Canvas.X)/l.Pixel, (my-l.Canvas.Y)/l.Pixel
		if x < 6 && y < 10 {
			f := g.model.ActiveFont()
			font, _ := assetsync.CompileFont(f)
			cols := font.Columns(rune(s.selected))
			if pressed {
				s.strokeChanged = false
				s.stroke = true
				s.strokeOn = cols[x]&(1<<uint(y)) == 0
			}
			if s.stroke {
				if (cols[x]&(1<<uint(y)) != 0) != s.strokeOn {
					if err := g.model.SetFontPixel(s.selected, x, y, s.strokeOn); err != nil {
						s.message = err.Error()
					} else {
						s.strokeChanged = true
						g.requestRender()
					}
				}
			}
		} else if pressed {
			s.message = "Spacing is locked: draw only inside the 6 x 10 bitmap."
		}
	}
	if !down && s.stroke {
		s.stroke = false
		if s.strokeChanged {
			g.fontChanged()
		}
	}
}
func drawFontPixels(screen *ebiten.Image, font *rt.Font, text string, x, y, scale int, clr color.Color, maxWidth int) {
	start := x
	for _, cp := range text {
		if cp == '\n' || x+6*scale > start+maxWidth {
			x = start
			y += 10 * scale
			if cp == '\n' {
				continue
			}
		}
		columns := font.Columns(cp)
		for col, bits := range columns {
			for row := 0; row < 10; row++ {
				if bits&(1<<uint(row)) != 0 {
					fillRect(screen, editor.Rect{X: x + col*scale, Y: y + row*scale, W: scale, H: scale}, clr)
				}
			}
		}
		x += 6 * scale
	}
}
func fontField(screen *ebiten.Image, r editor.Rect, value string, active bool) {
	fillRect(screen, r, colorBackground)
	strokeRect(screen, r, colorBorder)
	if active {
		strokeRect(screen, r, colorHighlight)
	}
	shown := truncateUIText(value, r.W-20)
	drawText(screen, shown, r.X+8, r.Y+(r.H-uiTextHeight)/2, colorText)
	if active {
		cx := min(r.X+r.W-8, r.X+8+uiTextWidth(shown)+2)
		drawLine(screen, cx, r.Y+7, cx, r.Y+r.H-7, colorHighlight)
	}
}
func (g *Game) drawFontEditor(screen *ebiten.Image) {
	s := &g.fontEditor
	l := newFontEditorLayout()
	p := l.Panel
	f := g.model.ActiveFont()
	if f == nil {
		return
	}
	font, _ := assetsync.CompileFont(f)
	fillRect(screen, editor.Rect{W: screenWidth, H: screenHeight}, color.RGBA{0, 0, 0, 210})
	fillRect(screen, p, colorPanel)
	strokeRect(screen, p, colorBorder)
	drawTextScale(screen, "CARTRIDGE FONT", p.X+24, p.Y+18, 2, colorText)
	drawFontButton(screen, l.Close, "CLOSE", false, colorSprite)
	drawSmallText(screen, "ROM font", l.Select.X, l.Select.Y-20, colorDim)
	drawSmallText(screen, "New font name", l.Name.X, l.Name.Y-20, colorDim)
	drawFontButton(screen, l.Select, strings.ToUpper(f.Name)+"  v", s.menu, colorSprite)
	name := s.name
	if name == "" {
		name = "new-font-name"
	}
	fontField(screen, l.Name, name, s.focus == 2)
	drawFontButton(screen, l.Copy, "COPY", false, colorSprite)
	drawFontButton(screen, l.Blank, "BLANK", false, colorSprite)
	drawSmallText(screen, "Preview text / first two lines", l.PreviewText.X, l.PreviewText.Y-20, colorDim)
	fontField(screen, l.PreviewText, s.preview, s.focus == 1)
	fillRect(screen, l.Preview, colorBackground)
	strokeRect(screen, l.Preview, colorBorder)
	// Two rows at native 2x scale; clipping prevents long samples leaking into tools.
	clip := screen.SubImage(image.Rect(l.Preview.X, l.Preview.Y, l.Preview.X+l.Preview.W, l.Preview.Y+52)).(*ebiten.Image)
	drawFontPixels(clip, font, s.preview, l.Preview.X+12, l.Preview.Y+12, 2, colorLCD, l.Preview.W-24)
	missing := 0
	for _, cp := range s.preview {
		found := cp >= 32 && cp <= 126
		for _, extra := range font.Extra {
			if cp == extra.Codepoint {
				found = true
				break
			}
		}
		if !found {
			missing++
		}
	}
	if missing > 0 {
		drawSmallText(screen, fmt.Sprintf("%d unsupported characters shown as ? / select another font or add glyphs", missing), l.Preview.X+12, l.Preview.Y+l.Preview.H-22, colorLCD)
	}
	glyphLabel := fmt.Sprintf("U+%04X  %c", s.selected, s.selected)
	if assetsync.IsBuiltinFont(f.Name) {
		glyphLabel += " / read-only"
	}
	drawSmallText(screen, glyphLabel, l.Canvas.X, l.Canvas.Y-24, colorText)
	columns := font.Columns(rune(s.selected))
	for y := 0; y < 10; y++ {
		for x := 0; x < 6; x++ {
			r := editor.Rect{X: l.Canvas.X + x*l.Pixel, Y: l.Canvas.Y + y*l.Pixel, W: l.Pixel - 1, H: l.Pixel - 1}
			clr := colorBackground
			if x >= 6 || y >= 10 {
				clr = colorButtonFace
			}
			if x < 6 && y < 10 && columns[x]&(1<<uint(y)) != 0 {
				clr = colorLCD
			}
			fillRect(screen, r, clr)
		}
	}
	strokeRect(screen, editor.Rect{X: l.Canvas.X, Y: l.Canvas.Y, W: 6 * l.Pixel, H: 10 * l.Pixel}, colorHighlight)
	fontField(screen, l.GlyphInput, s.glyphInput, s.focus == 3)
	drawFontButton(screen, l.Add, "SELECT / ADD", false, colorSprite)
	fillRect(screen, l.Glyphs, colorBackground)
	strokeRect(screen, l.Glyphs, colorBorder)
	cols := max(1, l.Glyphs.W/36)
	rows := max(1, l.Glyphs.H/30)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			idx := (s.firstRow+row)*cols + col
			if idx >= len(f.Glyphs) {
				continue
			}
			glyph := f.Glyphs[idx]
			r := editor.Rect{X: l.Glyphs.X + col*36, Y: l.Glyphs.Y + row*30, W: 35, H: 29}
			if glyph.Codepoint == s.selected {
				fillRect(screen, r, colorButtonFace)
				strokeRect(screen, r, colorHighlight)
			}
			if *font.Columns(rune(glyph.Codepoint)) == [6]uint16{} {
				label := string(rune(glyph.Codepoint))
				if glyph.Codepoint == 32 {
					label = "SP"
				}
				drawSmallTextCentered(screen, label, r, colorDim)
			} else {
				drawFontPixels(screen, font, string(rune(glyph.Codepoint)), r.X+12, r.Y+8, 2, colorText, 12)
			}
		}
	}
	drawSmallText(screen, "6 x 10 bitmap / 6 x 10 cell / 1-bit / 40 x 16 screen grid", p.X+24, p.Y+p.H-98, colorText)
	drawSmallText(screen, fmt.Sprintf("%d glyphs / bitmap %d bytes / one font per ROM", len(f.Glyphs), len(f.Glyphs)*12), p.X+24, p.Y+p.H-76, colorDim)
	drawSmallText(screen, truncateUIText(s.message, p.W-206), p.X+24, p.Y+p.H-42, colorLCD)
	drawFontButton(screen, l.Save, "SAVE PROJECT", false, colorSprite)
	if s.menu {
		for i := s.firstRow; i < min(len(g.model.FontNames()), s.firstRow+6); i++ {
			name := g.model.FontNames()[i]
			r := l.Select
			r.Y += r.H + (i-s.firstRow)*34
			r.H = 34
			drawFontButton(screen, r, name, name == f.Name, colorSprite)
		}
	}
}

func drawFontButton(screen *ebiten.Image, r editor.Rect, label string, active bool, accent color.Color) {
	drawTactileButton(screen, r, label, active, accent, true)
}
