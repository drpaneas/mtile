package main

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
	"github.com/drpaneas/mtile/internal/luaparser"
	"github.com/drpaneas/mtile/internal/transpiler"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

var (
	codeLineHeight = uiFontStyles[textComfort].lineHeight
	codeCharWidth  = uiFontStyles[textComfort].codeCharWidth
)

var (
	codeKeywordColor = color.RGBA{R: 255, G: 174, B: 112, A: 255}
	codeStringColor  = color.RGBA{R: 184, G: 210, B: 145, A: 255}
	codeNumberColor  = color.RGBA{R: 178, G: 158, B: 255, A: 255}
	codeCommentColor = color.RGBA{R: 111, G: 135, B: 126, A: 255}
	codeBuiltinColor = color.RGBA{R: 112, G: 196, B: 224, A: 255}
	codeCursorColor  = color.RGBA{R: 255, G: 214, B: 94, A: 255}
)

type codeLayout struct {
	Header      editor.Rect
	Sidebar     editor.Rect
	File        editor.Rect
	Save        editor.Rect
	Run         editor.Rect
	Tabs        editor.Rect
	Editor      editor.Rect
	Gutter      editor.Rect
	Text        editor.Rect
	Inspector   editor.Rect
	Diagnostics editor.Rect
	Search      editor.Rect
}

type codeTabHit struct {
	Path  string
	Rect  editor.Rect
	Close editor.Rect
}

func (g *Game) visibleCodeTabs(layout codeLayout) []codeTabHit {
	if len(g.codeTabs) == 0 || layout.Tabs.W <= 0 {
		return nil
	}
	const preferredWidth = 168
	count := max(1, layout.Tabs.W/preferredWidth)
	count = min(count, len(g.codeTabs))
	start := 0
	if g.codeActiveTab >= count {
		start = g.codeActiveTab - count + 1
	}
	start = min(start, len(g.codeTabs)-count)
	width := layout.Tabs.W / count
	hits := make([]codeTabHit, 0, count)
	for index := 0; index < count; index++ {
		path := g.codeTabs[start+index]
		r := editor.Rect{X: layout.Tabs.X + index*width, Y: layout.Tabs.Y, W: width, H: layout.Tabs.H}
		hits = append(hits, codeTabHit{Path: path, Rect: r, Close: editor.Rect{X: r.X + r.W - 28, Y: r.Y + 5, W: 22, H: r.H - 10}})
	}
	return hits
}

func (g *Game) updateCodeTabClick(layout codeLayout, x, y int) bool {
	if !rectContains(layout.Tabs, x, y) {
		return false
	}
	for _, hit := range g.visibleCodeTabs(layout) {
		if !rectContains(hit.Rect, x, y) {
			continue
		}
		if rectContains(hit.Close, x, y) {
			g.closeCodeDocument(hit.Path)
		} else {
			g.activateCodeDocument(hit.Path)
		}
		return true
	}
	return true
}

func (g *Game) updateProjectExplorerClick(layout codeLayout, x, y int) bool {
	if !rectContains(layout.Sidebar, x, y) {
		return false
	}
	visible := g.visibleDevelopProjectEntries()
	row := (y - (layout.Sidebar.Y + 76)) / 27
	if row < 0 {
		return true
	}
	index := g.codeProjectScroll + row
	if index >= 0 && index < len(visible) {
		g.activateProjectEntry(visible[index])
	}
	return true
}

func (g *Game) updateCodeQuickOpenInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.codeQuickOpen = false
		return
	}
	if codeKeyRepeat(ebiten.KeyBackspace) && g.codeQuickDraft != "" {
		runes := []rune(g.codeQuickDraft)
		g.codeQuickDraft = string(runes[:len(runes)-1])
		g.codeQuickIndex = 0
		g.updateQuickOpenMatches()
	}
	if input := ebiten.AppendInputChars(nil); len(input) != 0 {
		g.codeQuickDraft += string(input)
		g.codeQuickIndex = 0
		g.updateQuickOpenMatches()
	}
	if len(g.codeQuickMatches) != 0 {
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
			g.codeQuickIndex = (g.codeQuickIndex - 1 + len(g.codeQuickMatches)) % len(g.codeQuickMatches)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
			g.codeQuickIndex = (g.codeQuickIndex + 1) % len(g.codeQuickMatches)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
			if err := g.openCodeDocument(g.codeQuickMatches[g.codeQuickIndex]); err != nil {
				g.statusMsg = "quick open: " + err.Error()
			}
			g.codeQuickOpen = false
		}
	}
}

func newCodeLayout() codeLayout {
	contentY := editorTabsHeight
	headerH := 68
	sidebarW := 244
	inspectorW := 330
	if screenWidth < 1400 {
		sidebarW = 200
		inspectorW = 248
	}
	bottomH := 184
	padding := 12
	header := editor.Rect{X: 0, Y: contentY, W: screenWidth, H: headerH}
	sidebar := editor.Rect{X: 0, Y: contentY + headerH, W: sidebarW, H: statusBarRect.Y - contentY - headerH}
	tabs := editor.Rect{X: sidebarW + padding, Y: contentY + headerH + padding, W: screenWidth - sidebarW - inspectorW - padding*3, H: 34}
	editorRect := editor.Rect{X: tabs.X, Y: tabs.Y + tabs.H, W: tabs.W, H: statusBarRect.Y - (tabs.Y + tabs.H) - bottomH - padding}
	gutterWidth := codeCharWidth*4 + 20
	gutter := editor.Rect{X: editorRect.X, Y: editorRect.Y, W: gutterWidth, H: editorRect.H}
	textRect := editor.Rect{X: gutter.X + gutter.W, Y: editorRect.Y, W: editorRect.W - gutter.W, H: editorRect.H}
	inspector := editor.Rect{X: editorRect.X + editorRect.W + padding, Y: editorRect.Y, W: inspectorW, H: editorRect.H}
	diagnostics := editor.Rect{X: editorRect.X, Y: editorRect.Y + editorRect.H + padding, W: screenWidth - editorRect.X - padding, H: bottomH - padding}
	fileRect := editor.Rect{X: 178, Y: contentY + 18, W: min(560, screenWidth/3), H: 34}
	saveRect := editor.Rect{X: screenWidth - 302, Y: contentY + 15, W: 126, H: 38}
	runRect := editor.Rect{X: screenWidth - 164, Y: contentY + 15, W: 146, H: 38}
	search := editor.Rect{X: textRect.X + textRect.W - 390, Y: textRect.Y + 8, W: 370, H: 34}
	return codeLayout{Header: header, Sidebar: sidebar, File: fileRect, Save: saveRect, Run: runRect, Tabs: tabs, Editor: editorRect, Gutter: gutter, Text: textRect, Inspector: inspector, Diagnostics: diagnostics, Search: search}
}

func codeKeyRepeat(key ebiten.Key) bool {
	duration := inpututil.KeyPressDuration(key)
	return duration == 1 || duration > 24 && duration%3 == 0
}

func codeModifier() bool {
	return ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
}

func (g *Game) updateCodeMode() {
	g.updateCodeWorkspace(newCodeLayout())
}

func (g *Game) updateCodeWorkspace(layout codeLayout) {
	if g.code == nil {
		return
	}
	mx, my := ebiten.CursorPosition()
	if g.codeQuickOpen {
		g.updateCodeQuickOpenInput()
		return
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		switch {
		case g.updateCodeTabClick(layout, mx, my):
			return
		case g.updateProjectExplorerClick(layout, mx, my):
			return
		case rectContains(layout.Save, mx, my):
			g.saveCode()
		case rectContains(layout.Run, mx, my):
			g.startRun()
		case rectContains(layout.Text, mx, my):
			g.code.SetCursor(g.codeOffsetAt(layout, mx, my), ebiten.IsKeyPressed(ebiten.KeyShift))
			g.codeDragging = true
			g.codeCompletionOpen = false
		}
	}
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.codeDragging = false
	} else if g.codeDragging {
		g.code.SetCursor(g.codeOffsetAt(layout, mx, my), true)
	}
	if rectContains(layout.Text, mx, my) {
		_, wheelY := ebiten.Wheel()
		if wheelY != 0 {
			g.codeScrollLine = max(0, g.codeScrollLine-int(wheelY*3))
		}
	} else if rectContains(layout.Sidebar, mx, my) {
		_, wheelY := ebiten.Wheel()
		if wheelY != 0 {
			visible := g.visibleDevelopProjectEntries()
			rows := max(1, (layout.Sidebar.H-84)/27)
			g.codeProjectScroll = min(max(0, g.codeProjectScroll-int(wheelY*3)), max(0, len(visible)-rows))
		}
	}

	mod := codeModifier()
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	alt := ebiten.IsKeyPressed(ebiten.KeyAlt)
	if mod && shift && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		g.saveAllCode()
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		g.saveCode()
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.scanDevelopProject()
		g.codeQuickOpen = true
		g.codeQuickDraft = ""
		g.codeQuickIndex = 0
		g.updateQuickOpenMatches()
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyW) {
		g.closeCodeDocument(g.codePath)
		return
	}
	if mod && shift && inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.reloadActiveCodeDocument()
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.startRun()
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyF) {
		g.codeSearchOpen = true
		g.codeSearchDraft = g.code.SelectedText()
		g.updateCodeSearch()
		return
	}
	if g.codeSearchOpen {
		g.updateCodeSearchInput()
		return
	}
	if mod && ebiten.IsKeyPressed(ebiten.KeySpace) {
		g.openCodeCompletions()
		return
	}
	if g.codeCompletionOpen && g.updateCodeCompletionInput() {
		return
	}
	if mod {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyA):
			g.code.SelectAll()
			return
		case inpututil.IsKeyJustPressed(ebiten.KeyC):
			g.copyCodeSelection(false)
			return
		case inpututil.IsKeyJustPressed(ebiten.KeyX):
			g.copyCodeSelection(true)
			return
		case inpututil.IsKeyJustPressed(ebiten.KeyV):
			g.pasteCode()
			return
		case inpututil.IsKeyJustPressed(ebiten.KeyZ) && shift:
			g.code.Redo()
			return
		case inpututil.IsKeyJustPressed(ebiten.KeyZ):
			g.code.Undo()
			return
		}
	}

	word := mod || alt
	switch {
	case codeKeyRepeat(ebiten.KeyArrowLeft):
		g.code.MoveHorizontal(-1, shift, word)
	case codeKeyRepeat(ebiten.KeyArrowRight):
		g.code.MoveHorizontal(1, shift, word)
	case codeKeyRepeat(ebiten.KeyArrowUp):
		g.code.MoveVertical(-1, shift)
	case codeKeyRepeat(ebiten.KeyArrowDown):
		g.code.MoveVertical(1, shift)
	case codeKeyRepeat(ebiten.KeyPageUp):
		g.code.MoveVertical(-max(1, layout.Text.H/codeLineHeight-2), shift)
	case codeKeyRepeat(ebiten.KeyPageDown):
		g.code.MoveVertical(max(1, layout.Text.H/codeLineHeight-2), shift)
	case codeKeyRepeat(ebiten.KeyHome):
		g.code.MoveLineBoundary(false, shift)
	case codeKeyRepeat(ebiten.KeyEnd):
		g.code.MoveLineBoundary(true, shift)
	case codeKeyRepeat(ebiten.KeyBackspace):
		g.code.DeleteBackward(word)
	case codeKeyRepeat(ebiten.KeyDelete):
		g.code.DeleteForward(word)
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter):
		g.code.Newline()
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		g.code.Indent(shift)
	default:
		if !mod && !alt {
			if input := ebiten.AppendInputChars(nil); len(input) != 0 {
				g.code.Insert(string(input))
			}
		}
	}
	g.refreshCodeDiagnostics()
	g.keepCodeCursorVisible(layout)
}

func (g *Game) codeOffsetAt(layout codeLayout, x, y int) int {
	line := g.codeScrollLine + max(0, y-layout.Text.Y-4)/codeLineHeight
	column := g.codeScrollColumn + max(0, x-layout.Text.X-8)/codeCharWidth
	return g.code.Offset(line, column)
}

func (g *Game) keepCodeCursorVisible(layout codeLayout) {
	line, column := g.code.LineColumn(g.code.Cursor())
	visibleLines := max(1, (layout.Text.H-8)/codeLineHeight)
	visibleColumns := max(1, (layout.Text.W-16)/codeCharWidth)
	if line < g.codeScrollLine {
		g.codeScrollLine = line
	} else if line >= g.codeScrollLine+visibleLines {
		g.codeScrollLine = line - visibleLines + 1
	}
	if column < g.codeScrollColumn {
		g.codeScrollColumn = column
	} else if column >= g.codeScrollColumn+visibleColumns {
		g.codeScrollColumn = column - visibleColumns + 1
	}
	g.codeScrollLine = min(g.codeScrollLine, max(0, g.code.LineCount()-1))
}

func (g *Game) saveCode() bool {
	if g.code == nil {
		return true
	}
	g.syncActiveCodeDocument()
	document := g.codeDocuments[g.codePath]
	if document == nil || !document.Editor.Dirty() {
		g.statusMsg = filepath.Base(g.codePath) + " already saved"
		return true
	}
	if !g.saveCodeDocument(document) {
		return false
	}
	g.codeClosePending = ""
	g.statusMsg = document.Relative + " saved"
	return true
}

func (g *Game) refreshCodeDiagnostics() {
	if g.code == nil || g.code.Text() == g.codeLastParsed {
		return
	}
	g.codeLastParsed = g.code.Text()
	g.codeDiagnostic = "No problems detected. Ready to build."
	g.codeDiagnosticLine, g.codeDiagnosticColumn = 0, 0
	sourceName := filepath.ToSlash(g.codePath)
	if relative, relErr := filepath.Rel(g.model.Dir, g.codePath); relErr == nil {
		sourceName = filepath.ToSlash(relative)
	}
	_, err := luaparser.ParseString(sourceName, g.code.Text())
	if err == nil {
		return
	}
	g.codeDiagnostic = err.Error()
	var parseError *luaparser.Error
	if errors.As(err, &parseError) {
		g.codeDiagnosticLine = parseError.Pos.Line
		g.codeDiagnosticColumn = parseError.Pos.Column
	}
	g.syncActiveCodeDocument()
}

func (g *Game) copyCodeSelection(cut bool) {
	value := g.code.SelectedText()
	if value == "" {
		return
	}
	g.codeClipboard = value
	_ = hostclipboard.Write(value)
	if cut {
		g.code.DeleteSelection()
	}
}

func (g *Game) pasteCode() {
	value, err := hostclipboard.Read()
	if err != nil {
		value = g.codeClipboard
	}
	if value != "" {
		g.code.Insert(value)
	}
}

func (g *Game) updateCodeSearchInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.codeSearchOpen = false
		return
	}
	if codeKeyRepeat(ebiten.KeyBackspace) && g.codeSearchDraft != "" {
		runes := []rune(g.codeSearchDraft)
		g.codeSearchDraft = string(runes[:len(runes)-1])
		g.updateCodeSearch()
	}
	if input := ebiten.AppendInputChars(nil); len(input) != 0 {
		g.codeSearchDraft += string(input)
		g.updateCodeSearch()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		if len(g.codeSearchMatches) != 0 {
			delta := 1
			if ebiten.IsKeyPressed(ebiten.KeyShift) {
				delta = -1
			}
			g.codeSearchIndex = (g.codeSearchIndex + delta + len(g.codeSearchMatches)) % len(g.codeSearchMatches)
			g.selectCodeSearchMatch()
		}
	}
}

func (g *Game) updateCodeSearch() {
	g.codeSearchMatches = nil
	g.codeSearchIndex = 0
	needle := []rune(strings.ToLower(g.codeSearchDraft))
	if len(needle) == 0 {
		return
	}
	haystack := []rune(strings.ToLower(g.code.Text()))
	for at := 0; at+len(needle) <= len(haystack); {
		if string(haystack[at:at+len(needle)]) == string(needle) {
			g.codeSearchMatches = append(g.codeSearchMatches, at)
			at += len(needle)
		} else {
			at++
		}
	}
	g.selectCodeSearchMatch()
}

func (g *Game) selectCodeSearchMatch() {
	if len(g.codeSearchMatches) == 0 {
		return
	}
	at := g.codeSearchMatches[g.codeSearchIndex]
	g.code.SetCursor(at, false)
	g.code.SetCursor(at+len([]rune(g.codeSearchDraft)), true)
	g.keepCodeCursorVisible(g.activeCodeLayout())
}

func (g *Game) activeCodeLayout() codeLayout {
	if g != nil && g.mode == modeCode {
		return newDevelopLayout().Code
	}
	return newCodeLayout()
}

func (g *Game) openCodeCompletions() {
	prefix := g.codePrefix()
	items := []string{"_init", "_update", "_draw"}
	for name := range transpiler.Builtins {
		items = append(items, name)
	}
	sort.Strings(items)
	g.codeCompletions = g.codeCompletions[:0]
	for _, item := range items {
		if prefix == "" || strings.HasPrefix(item, prefix) {
			g.codeCompletions = append(g.codeCompletions, item)
		}
	}
	g.codeCompletionIndex = 0
	g.codeCompletionOpen = len(g.codeCompletions) != 0
}

func (g *Game) codePrefix() string {
	text := []rune(g.code.Text())
	at := g.code.Cursor()
	start := at
	for start > 0 && (unicode.IsLetter(text[start-1]) || unicode.IsDigit(text[start-1]) || text[start-1] == '_') {
		start--
	}
	return string(text[start:at])
}

func (g *Game) updateCodeCompletionInput() bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.codeCompletionOpen = false
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		g.codeCompletionIndex = (g.codeCompletionIndex - 1 + len(g.codeCompletions)) % len(g.codeCompletions)
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		g.codeCompletionIndex = (g.codeCompletionIndex + 1) % len(g.codeCompletions)
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		prefix := []rune(g.codePrefix())
		at := g.code.Cursor()
		g.code.SetCursor(at-len(prefix), false)
		g.code.SetCursor(at, true)
		name := g.codeCompletions[g.codeCompletionIndex]
		if _, ok := transpiler.Builtins[name]; ok {
			name += "()"
		}
		g.code.Insert(name)
		g.codeCompletionOpen = false
		return true
	}
	return false
}

func (g *Game) drawCodeMode(screen *ebiten.Image) {
	layout := newCodeLayout()
	fillRect(screen, editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}, colorInk)
	fillRect(screen, layout.Header, color.RGBA{R: 30, G: 28, B: 75, A: 255})
	fillRect(screen, editor.Rect{X: 0, Y: layout.Header.Y, W: layout.Header.W, H: 3}, colorCode)
	drawTextScale(screen, "MTILE", 20, layout.Header.Y+4, 2, colorText)
	drawText(screen, "CARTRIDGE WORKSTATION  /  LUA", 20, layout.Header.Y+layout.Header.H-uiTextHeight-4, colorLCD)
	fillRect(screen, layout.File, colorBackground)
	strokeRect(screen, layout.File, colorBorder)
	name := filepath.Base(g.codePath)
	if g.code.Dirty() {
		name += "  *"
	}
	drawText(screen, name, layout.File.X+12, layout.File.Y+(layout.File.H-uiTextHeight)/2, colorText)
	drawNativeButton(screen, layout.Save, "SAVE", false, colorCode)
	drawNativeButton(screen, layout.Run, "RUN GAME  >", false, colorRun)

	g.drawCodeSidebar(screen, layout)
	g.drawCodeTabs(screen, layout)
	g.drawCodeDocument(screen, layout)
	g.drawCodeInspector(screen, layout)
	g.drawCodeDiagnostics(screen, layout)
	g.drawCodeOverlays(screen, layout)
}

func (g *Game) drawCodeOverlays(screen *ebiten.Image, layout codeLayout) {
	if g.codeQuickOpen {
		g.drawCodeQuickOpen(screen, layout)
		return
	}
	if g.codeSearchOpen {
		fillRect(screen, layout.Search, colorPanel)
		strokeRect(screen, layout.Search, colorCode)
		label := fmt.Sprintf("FIND  %s", g.codeSearchDraft)
		if len(g.codeSearchMatches) != 0 {
			label += fmt.Sprintf("   %d/%d", g.codeSearchIndex+1, len(g.codeSearchMatches))
		} else if g.codeSearchDraft != "" {
			label += "   no matches"
		}
		drawText(screen, truncateMusicEditText(label, max(1, layout.Search.W/codeCharWidth-3)), layout.Search.X+10, layout.Search.Y+9, colorText)
	}
	if g.codeCompletionOpen {
		g.drawCodeCompletions(screen, layout)
	}
}

func (g *Game) drawCodeSidebar(screen *ebiten.Image, layout codeLayout) {
	fillRect(screen, layout.Sidebar, colorPanel)
	drawLine(screen, layout.Sidebar.X+layout.Sidebar.W-1, layout.Sidebar.Y, layout.Sidebar.X+layout.Sidebar.W-1, layout.Sidebar.Y+layout.Sidebar.H, colorBorder)
	drawText(screen, "PROJECT", 18, layout.Sidebar.Y+20, colorDim)
	rootName := truncateMusicEditText(filepath.Base(g.model.Dir), max(1, (layout.Sidebar.W-34)/codeCharWidth))
	drawText(screen, rootName, 18, layout.Sidebar.Y+46, colorText)
	visible := g.visibleDevelopProjectEntries()
	rows := max(1, (layout.Sidebar.H-84)/27)
	g.codeProjectScroll = min(g.codeProjectScroll, max(0, len(visible)-rows))
	for row := 0; row < rows && g.codeProjectScroll+row < len(visible); row++ {
		entry := visible[g.codeProjectScroll+row]
		y := layout.Sidebar.Y + 76 + row*27
		r := editor.Rect{X: layout.Sidebar.X + 7, Y: y, W: layout.Sidebar.W - 15, H: 25}
		active := !entry.Directory && filepath.Clean(entry.Path) == filepath.Clean(g.codePath)
		if active {
			fillRect(screen, r, colorIndigo)
			fillRect(screen, editor.Rect{X: r.X, Y: r.Y, W: 3, H: r.H}, colorCode)
		}
		x := r.X + 9 + entry.Depth*13
		icon, clr := "<> ", colorText
		if entry.Directory {
			icon, clr = "+  ", colorDim
			if g.codeProjectExpanded[entry.Relative] {
				icon = "-  "
			}
		} else if filepath.Ext(entry.Path) != ".lua" {
			icon, clr = "·  ", colorDim
		}
		available := max(1, (r.X+r.W-x)/codeCharWidth-len(icon))
		drawText(screen, icon+truncateMusicEditText(entry.Name, available), x, r.Y+(r.H-uiTextHeight)/2, clr)
	}
	if len(visible) == 0 {
		drawText(screen, "No project files", 18, layout.Sidebar.Y+86, colorDim)
	}
	drawText(screen, "Cmd/Ctrl+P  QUICK OPEN", 16, layout.Sidebar.Y+layout.Sidebar.H-28, colorLCD)
}

func (g *Game) drawCodeTabs(screen *ebiten.Image, layout codeLayout) {
	fillRect(screen, layout.Tabs, colorInk)
	strokeRect(screen, layout.Tabs, colorBorder)
	for _, hit := range g.visibleCodeTabs(layout) {
		document := g.codeDocuments[hit.Path]
		active := filepath.Clean(hit.Path) == filepath.Clean(g.codePath)
		if active {
			fillRect(screen, hit.Rect, colorBackground)
			fillRect(screen, editor.Rect{X: hit.Rect.X, Y: hit.Rect.Y, W: hit.Rect.W, H: 3}, colorCode)
		} else {
			fillRect(screen, hit.Rect, colorPanel)
		}
		drawLine(screen, hit.Rect.X+hit.Rect.W-1, hit.Rect.Y+4, hit.Rect.X+hit.Rect.W-1, hit.Rect.Y+hit.Rect.H-4, colorBorder)
		name := filepath.Base(hit.Path)
		if document != nil && document.Editor.Dirty() {
			name += "  *"
		}
		name = truncateMusicEditText(name, max(1, (hit.Rect.W-42)/codeCharWidth))
		clr := colorDim
		if active {
			clr = colorText
		}
		drawText(screen, name, hit.Rect.X+11, hit.Rect.Y+(hit.Rect.H-uiTextHeight)/2, clr)
		drawTextCentered(screen, "x", hit.Close, map[bool]color.Color{false: colorDim, true: colorCode}[active])
	}
}

func (g *Game) drawCodeQuickOpen(screen *ebiten.Image, layout codeLayout) {
	width := min(620, layout.Editor.W-60)
	height := 64 + min(8, max(1, len(g.codeQuickMatches)))*30
	box := editor.Rect{X: layout.Editor.X + (layout.Editor.W-width)/2, Y: layout.Editor.Y + 46, W: width, H: height}
	fillRect(screen, box, colorInk)
	strokeRoundedRect(screen, box, 4, colorCode)
	drawText(screen, "QUICK OPEN", box.X+16, box.Y+14, colorCode)
	query := g.codeQuickDraft
	if query == "" {
		query = "type a Lua file name..."
	}
	drawText(screen, truncateMusicEditText(query, max(1, (box.W-32)/codeCharWidth)), box.X+16, box.Y+38, map[bool]color.Color{false: colorText, true: colorDim}[g.codeQuickDraft == ""])
	visible := min(8, len(g.codeQuickMatches))
	start := min(max(0, g.codeQuickIndex-visible+1), max(0, len(g.codeQuickMatches)-visible))
	for row := 0; row < visible; row++ {
		index := start + row
		path := g.codeQuickMatches[index]
		relative, _ := filepath.Rel(g.model.Dir, path)
		y := box.Y + 66 + row*30
		if index == g.codeQuickIndex {
			fillRect(screen, editor.Rect{X: box.X + 8, Y: y - 5, W: box.W - 16, H: 27}, colorIndigo)
		}
		drawText(screen, filepath.ToSlash(relative), box.X+18, y, map[bool]color.Color{false: colorDim, true: colorText}[index == g.codeQuickIndex])
	}
	if len(g.codeQuickMatches) == 0 {
		drawText(screen, "No matching Lua files", box.X+18, box.Y+72, colorDim)
	}
}

func (g *Game) drawCodeDocument(screen *ebiten.Image, layout codeLayout) {
	fillRect(screen, layout.Editor, colorBackground)
	strokeRect(screen, layout.Editor, colorBorder)
	fillRect(screen, layout.Gutter, colorInk)
	drawLine(screen, layout.Gutter.X+layout.Gutter.W-1, layout.Gutter.Y, layout.Gutter.X+layout.Gutter.W-1, layout.Gutter.Y+layout.Gutter.H, colorBorder)
	visible := max(1, (layout.Text.H-8)/codeLineHeight)
	cursorLine, _ := g.code.LineColumn(g.code.Cursor())
	selectionStart, selectionEnd, selected := g.code.Selection()
	for row := 0; row < visible; row++ {
		lineNumber := g.codeScrollLine + row
		if lineNumber >= g.code.LineCount() {
			break
		}
		y := layout.Text.Y + 1 + row*codeLineHeight
		line := []rune(g.code.Line(lineNumber))
		lineStart := g.code.Offset(lineNumber, 0)
		if lineNumber == cursorLine {
			fillRect(screen, editor.Rect{X: layout.Text.X, Y: y - 3, W: layout.Text.W, H: codeLineHeight}, color.RGBA{R: 46, G: 49, B: 66, A: 255})
			fillRect(screen, editor.Rect{X: layout.Text.X, Y: y - 3, W: 3, H: codeLineHeight}, rgba(colorCode, 180))
		}
		if selected {
			from := max(selectionStart, lineStart)
			to := min(selectionEnd, lineStart+len(line))
			if selectionEnd > lineStart+len(line) && selectionStart <= lineStart+len(line) {
				to = lineStart + len(line) + 1
			}
			if from < to {
				x := layout.Text.X + 8 + (from-lineStart-g.codeScrollColumn)*codeCharWidth
				w := (to - from) * codeCharWidth
				fillRect(screen, editor.Rect{X: max(layout.Text.X, x), Y: y - 2, W: min(w, layout.Text.X+layout.Text.W-max(layout.Text.X, x)), H: codeLineHeight - 2}, color.RGBA{R: 65, G: 82, B: 122, A: 255})
			}
		}
		drawCodeText(screen, fmt.Sprintf("%4d", lineNumber+1), layout.Gutter.X+10, y, colorDim)
		g.drawCodeLine(screen, string(line), layout.Text.X+8, y, layout.Text.W-16)
	}
	if cursorLine >= g.codeScrollLine && cursorLine < g.codeScrollLine+visible {
		x := int(g.theme.codeCaretX)
		y := int(g.theme.codeCaretY)
		if x >= layout.Text.X && x < layout.Text.X+layout.Text.W {
			alpha := uint8(255)
			if g.theme.level != effectsClean {
				alpha = uint8(160 + 95*(1+math.Sin(float64(g.theme.frame)*.16))/2)
			}
			fillRect(screen, editor.Rect{X: x, Y: y, W: 2, H: codeLineHeight - 3}, rgba(codeCursorColor, alpha))
		}
	}
}

type codeSegment struct {
	text  string
	color color.Color
}

func (g *Game) drawCodeLine(screen *ebiten.Image, source string, x, y, width int) {
	runes := []rune(source)
	if g.codeScrollColumn >= len(runes) {
		return
	}
	runes = runes[g.codeScrollColumn:]
	limit := max(0, width/codeCharWidth)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	for _, segment := range highlightLuaLine(string(runes)) {
		drawCodeText(screen, segment.text, x, y, segment.color)
		x += len([]rune(segment.text)) * codeCharWidth
	}
}

func highlightLuaLine(line string) []codeSegment {
	runes := []rune(line)
	var result []codeSegment
	appendSegment := func(value string, clr color.Color) {
		if value != "" {
			result = append(result, codeSegment{text: value, color: clr})
		}
	}
	keywords := map[string]bool{"and": true, "break": true, "do": true, "else": true, "elseif": true, "end": true, "false": true, "for": true, "function": true, "if": true, "in": true, "local": true, "nil": true, "not": true, "or": true, "repeat": true, "return": true, "then": true, "true": true, "until": true, "while": true}
	for at := 0; at < len(runes); {
		if at+1 < len(runes) && runes[at] == '-' && runes[at+1] == '-' {
			appendSegment(string(runes[at:]), codeCommentColor)
			break
		}
		if runes[at] == '\'' || runes[at] == '"' {
			quote := runes[at]
			end := at + 1
			for end < len(runes) {
				if runes[end] == '\\' {
					end += 2
					continue
				}
				end++
				if runes[end-1] == quote {
					break
				}
			}
			appendSegment(string(runes[at:min(end, len(runes))]), codeStringColor)
			at = min(end, len(runes))
			continue
		}
		if unicode.IsLetter(runes[at]) || runes[at] == '_' {
			end := at + 1
			for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_') {
				end++
			}
			word := string(runes[at:end])
			clr := colorText
			if keywords[word] {
				clr = codeKeywordColor
			} else if _, ok := transpiler.Builtins[word]; ok {
				clr = codeBuiltinColor
			}
			appendSegment(word, clr)
			at = end
			continue
		}
		if unicode.IsDigit(runes[at]) {
			end := at + 1
			for end < len(runes) && (unicode.IsDigit(runes[end]) || strings.ContainsRune(".xabcdefABCDEF", runes[end])) {
				end++
			}
			appendSegment(string(runes[at:end]), codeNumberColor)
			at = end
			continue
		}
		appendSegment(string(runes[at]), colorText)
		at++
	}
	return result
}

var goHighlightKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

var goHighlightBuiltins = map[string]bool{
	"any": true, "append": true, "bool": true, "byte": true, "cap": true,
	"close": true, "complex64": true, "complex128": true, "copy": true,
	"delete": true, "error": true, "false": true, "float32": true, "float64": true,
	"imag": true, "int": true, "int8": true, "int16": true, "int32": true,
	"int64": true, "len": true, "make": true, "max": true, "min": true,
	"new": true, "nil": true, "panic": true, "print": true, "println": true,
	"real": true, "recover": true, "rune": true, "string": true, "true": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"uintptr": true,
}

func highlightGoLine(line string) []codeSegment {
	runes := []rune(line)
	result := make([]codeSegment, 0, 8)
	appendSegment := func(value string, clr color.Color) {
		if value != "" {
			result = append(result, codeSegment{text: value, color: clr})
		}
	}
	for at := 0; at < len(runes); {
		if at+1 < len(runes) && runes[at] == '/' && runes[at+1] == '/' {
			appendSegment(string(runes[at:]), codeCommentColor)
			break
		}
		if runes[at] == '\'' || runes[at] == '"' || runes[at] == '`' {
			quote := runes[at]
			end := at + 1
			for end < len(runes) {
				if quote != '`' && runes[end] == '\\' {
					end += 2
					continue
				}
				end++
				if runes[end-1] == quote {
					break
				}
			}
			appendSegment(string(runes[at:min(end, len(runes))]), codeStringColor)
			at = min(end, len(runes))
			continue
		}
		if unicode.IsLetter(runes[at]) || runes[at] == '_' {
			end := at + 1
			for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_') {
				end++
			}
			word := string(runes[at:end])
			clr := color.Color(colorText)
			if goHighlightKeywords[word] {
				clr = codeKeywordColor
			} else if goHighlightBuiltins[word] {
				clr = codeBuiltinColor
			}
			appendSegment(word, clr)
			at = end
			continue
		}
		if unicode.IsDigit(runes[at]) {
			end := at + 1
			for end < len(runes) && (unicode.IsDigit(runes[end]) || strings.ContainsRune(".xabcdefABCDEF", runes[end])) {
				end++
			}
			appendSegment(string(runes[at:end]), codeNumberColor)
			at = end
			continue
		}
		appendSegment(string(runes[at]), colorText)
		at++
	}
	return result
}

func (g *Game) drawCodeInspector(screen *ebiten.Image, layout codeLayout) {
	fillRect(screen, layout.Inspector, colorPanel)
	strokeRect(screen, layout.Inspector, colorBorder)
	drawText(screen, "OUTLINE", layout.Inspector.X+16, layout.Inspector.Y+18, colorCode)
	functions := codeFunctions(g.code.Text())
	if len(functions) == 0 {
		drawText(screen, "No functions yet", layout.Inspector.X+16, layout.Inspector.Y+50, colorDim)
	}
	functionLimit := max(1, (350-50-codeTextHeight)/codeLineHeight)
	for index, item := range functions {
		if index >= functionLimit {
			break
		}
		drawCodeText(screen, item, layout.Inspector.X+18, layout.Inspector.Y+50+index*codeLineHeight, colorText)
	}
	drawLine(screen, layout.Inspector.X+12, layout.Inspector.Y+350, layout.Inspector.X+layout.Inspector.W-12, layout.Inspector.Y+350, colorBorder)
	drawText(screen, "MTILE API", layout.Inspector.X+16, layout.Inspector.Y+370, colorLCD)
	for index, item := range []string{"spr(id, x, y)", "map()", "btn(id)", "sfx(id)", "music(id)", "gba_note(ch, note, vol)"} {
		drawCodeText(screen, item, layout.Inspector.X+18, layout.Inspector.Y+402+index*codeLineHeight, codeBuiltinColor)
	}
	drawText(screen, "Ctrl+Space  autocomplete", layout.Inspector.X+16, layout.Inspector.Y+layout.Inspector.H-32, colorDim)
}

func codeFunctions(source string) []string {
	var functions []string
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "local ")
		if !strings.HasPrefix(line, "function ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "function "))
		if at := strings.IndexRune(name, '('); at >= 0 {
			name = name[:at]
		}
		if name != "" {
			functions = append(functions, name+"()")
		}
	}
	return functions
}

func (g *Game) drawCodeDiagnostics(screen *ebiten.Image, layout codeLayout) {
	fillRect(screen, layout.Diagnostics, colorPanel)
	strokeRect(screen, layout.Diagnostics, colorBorder)
	drawText(screen, "PROBLEMS", layout.Diagnostics.X+16, layout.Diagnostics.Y+16, colorDim)
	clr := colorRun
	icon := "OK"
	if g.codeDiagnosticLine != 0 {
		clr = colorDanger
		icon = "ERROR"
	}
	drawText(screen, icon, layout.Diagnostics.X+16, layout.Diagnostics.Y+48, clr)
	message := g.codeDiagnostic
	if g.codeDiagnosticLine != 0 {
		message = fmt.Sprintf("Ln %d, Col %d  %s", g.codeDiagnosticLine, g.codeDiagnosticColumn, message)
	}
	drawText(screen, truncateUIText(message, layout.Diagnostics.W-110), layout.Diagnostics.X+94, layout.Diagnostics.Y+48, colorText)
	drawText(screen, "Lua is parsed and transpiled inside Mtile. No Lua installation is needed.", layout.Diagnostics.X+16, layout.Diagnostics.Y+84, colorLCD)
}

func (g *Game) drawCodeCompletions(screen *ebiten.Image, layout codeLayout) {
	line, column := g.code.LineColumn(g.code.Cursor())
	x := layout.Text.X + 8 + (column-g.codeScrollColumn)*codeCharWidth
	y := layout.Text.Y + 7 + (line-g.codeScrollLine+1)*codeLineHeight
	visible := min(8, len(g.codeCompletions))
	box := editor.Rect{X: min(x, layout.Text.X+layout.Text.W-230), Y: min(y, layout.Text.Y+layout.Text.H-visible*24-8) + max(0, 8-g.theme.completionFrame), W: 222, H: visible * 24}
	fillRect(screen, box, colorInk)
	strokeRect(screen, box, colorCode)
	start := min(max(0, g.codeCompletionIndex-visible+1), max(0, len(g.codeCompletions)-visible))
	for row := 0; row < visible; row++ {
		index := start + row
		if index == g.codeCompletionIndex {
			fillRect(screen, editor.Rect{X: box.X + 1, Y: box.Y + row*24 + 1, W: box.W - 2, H: 22}, colorIndigo)
		}
		drawText(screen, g.codeCompletions[index], box.X+10, box.Y+6+row*24, colorText)
	}
}
