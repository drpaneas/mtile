---
name: mTile cartridge font editor
description: Scoped design reference for the native cartridge-font overlay.
colors:
  primary: "#52d9d0"
  focus: "#ffcf45"
  bitmap: "#b7d48f"
  background: "#100f1c"
  panel: "#232139"
  border: "#7771a2"
  text: "#ebe7fa"
  muted-text: "#c2bbdb"
  button: "#1c1b34"
  button-hover: "#2b2946"
  button-pressed: "#272643"
  button-active: "#323068"
typography:
  title:
    fontFamily: "Go Sans"
    fontSize: "20px"
    fontWeight: 700
  body:
    fontFamily: "Go Sans"
    fontSize: "13px"
    fontWeight: 400
  label:
    fontFamily: "Go Sans"
    fontSize: "11px"
    fontWeight: 400
rounded:
  square: "0px"
spacing:
  panel-inset: "24px"
  field-inset: "8px"
components:
  font-button:
    backgroundColor: "{colors.button}"
    textColor: "{colors.text}"
    rounded: "{rounded.square}"
  font-button-active:
    backgroundColor: "{colors.button-active}"
    textColor: "{colors.primary}"
    rounded: "{rounded.square}"
  font-field:
    backgroundColor: "{colors.background}"
    textColor: "{colors.text}"
    rounded: "{rounded.square}"
---

# Design System: mTile cartridge font editor

## Overview

This reference covers only the native Ebitengine **CARTRIDGE FONT** overlay and
its **FONT** entry in the existing editor navigation. Its mode is **Operate**:
choose the cartridge font, inspect actual pixels, create an editable font, and
save the project. Preserve the existing purple and teal editor identity and
native shell.

Implementation truth lives in `cmd/editor/font_editor.go`, its modal integration
in `game.go` and `mode.go`, and `internal/editor/font.go`. Shared appearance comes
from `draw_util.go` and `theme.go`. The usage and font-format contract remain in
[Cartridge fonts](../../fonts.md).

The completed finish review was **Ship** for the captured desktop, compact,
dropdown, blank-font and overflow states. Blank-glyph identification and preview
warning overlap were resolved. This is bounded visual evidence, not a broad
claim about live interaction or accessibility. No web detector ran: this is a Go
native surface.

## Colors

Purple surfaces carry the workspace; accents identify an operation or state.
The frontmatter records shared source colors, independent of the game palette.

- **Primary teal:** navigation, hovered button borders and selected dropdown rows.
- **Focus yellow:** active text-field border and caret, selected glyph, and the
  editable bitmap boundary.
- **LCD green:** enlarged bitmap pixels, preview output, fallback count and status.
- **Neutral roles:** panel above the dark field/canvas background; lavender
  borders, bright text and muted explanatory labels.

## Typography

Go Sans supplies interface text, including empty-glyph labels. The frontmatter
records the default Comfort preset; Compact and Large reuse the editor's existing
measured faces. Use the title face for the overlay heading, body for typed values,
and small text for buttons, labels and metadata.

The cartridge preview and nonempty glyph tiles use the selected font's actual
bitmap at integer 2× scale. Keep that raster output distinct from host UI text.

## Layout

The centered panel is at most (1000 × 730 px), inset from the window by at least
(20 px) horizontally and (35 px) vertically. The implemented compact layout is
(1024 × 768 px); the desktop capture is (1600 × 1000 px). There is no separate
mobile composition.

Order the work top to bottom: font selection and named creation, preview text and
bitmap output, editing canvas beside the glyph browser, then format metadata and
status beside **SAVE PROJECT**. The selector stays (260 px) wide; the name field
and glyph browser take the remaining width. **CLOSE** stays at the upper right.

The preview container is (72 px) high. Its bitmap area is clipped to the first
(44 px), leaving a separate lower lane for the unsupported-character count.
Preserve this boundary when changing preview text, scale or warning copy.

## Elevation & Depth

Depth uses flat tonal layers and a black modal scrim (alpha 210/255), without
shadows. Draw the overlay after the editor shell and the dropdown after all panel
content so its rows remain above the preview.

## Shapes

Fields, buttons, panel and glyph selection use square corners and thin borders.
The canvas shows a fixed (6 × 8) cell; only the upper-left (4 × 5) bitmap accepts
ink. Pixel cells have a (1 px) gap. Locked spacing uses the button-face tone.
Canvas pixels scale from (16–28 px) according to available panel height.

## Components

- **Buttons and selector:** reuse `drawTactileButton` through `drawFontButton`.
  Hover changes the face and border; pressed and active states use their existing
  face colors. Active selector rows use teal text and borders. The menu shows at
  most six (34 px) rows and supports scrolling.
- **Text fields:** dark fill, lavender border, yellow active border and caret.
  Preview, new name and single-character fields hold at most 160, 32 and 1 runes.
  Tab cycles these three fields. Keep modal input ahead of workspace shortcuts
  so typing and pixel work cannot operate the obscured editor.
- **Creation:** presets remain read-only. **COPY** duplicates the current font;
  **BLANK** creates the international repertoire with empty bitmaps and a visible
  `?` fallback. Both use the entered name and select the new font.
- **Glyph browser:** (36 × 30 px) slots, with a yellow selected outline. Nonempty
  glyphs show their bitmap; blank glyphs show a muted host-font character, with
  `SP` for space. Keep blank characters identifiable before any pixels exist.
- **Canvas and direct selection:** show the selected codepoint and character,
  plus read-only status for presets. **SELECT / ADD** selects an existing
  character or adds an absent valid precomposed character. A drag adds or erases
  pixels consistently and forms one undoable stroke.
- **Persistence and exit:** **SAVE PROJECT** and Cmd/Ctrl+S save the project;
  the next build uses the selected font. Cmd/Ctrl+Z and Shift+Z use project undo
  and redo. Escape closes the open dropdown first, then the overlay.

## Do's and Don'ts

- **Do** preserve the fixed bitmap, locked spacing, visible fallback and
  one-selected-font-per-ROM contract.
- **Do** keep status text bounded before the save button and the fallback count
  below the clipped preview pixels.
- **Do** verify desktop, compact, dropdown, blank-font and overflow captures when
  changing this layout; `font_editor_render_test.go` contains those fixtures.
- **Don't** replace actual cartridge pixels with host-font approximations in the
  preview or populated glyph tiles.
- **Don't** hide blank glyphs, admit edits to built-in presets, or let modal
  typing and painting trigger the workspace beneath it.
- **Don't** extend this scoped reference into a product-wide identity or infer
  untested platform and accessibility guarantees from screenshots.
