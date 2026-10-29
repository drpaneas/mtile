# Cartridge fonts

mTile renders text using a tiny monochrome bitmap font. Existing projects use
**PICO-8 wide** by RhythmLynx, rasterized at 4×5 pixels. The editor's interface
fonts (Go Sans and JetBrains Mono) are separate from the cartridge font.

## Choose a font and preview it

Open **FONT** in the editor's top navigation. Choose from the **ROM font**
dropdown and type or paste into **Preview text**. The preview uses the actual
cartridge bitmaps, enlarged with integer pixels; it shows the first two lines.
Unsupported characters appear as `?` and are counted beneath the preview.

| Preset | Character coverage and appearance |
| --- | --- |
| `pico8` | Original PICO-8 wide ASCII; unchanged default |
| `arcade` | New square, four-column capital letters; ASCII |
| `greek` | Arcade Latin plus modern Greek capitals, lowercase, final sigma, tonos and dialytika |
| `german` | Arcade Latin plus ÄÖÜ, äöü, ß and ẞ |
| `international` | Both Greek and German repertoires in one font |

Choose **SAVE PROJECT**, then build again. Only the selection is compiled into
the cartridge. Changing fonts is a build-time operation; there is no Lua font
switch and no alternative-font library loaded into the ROM.

![Font selection and preview](media/fonts/editor.png)

The selection lives in `project.json`, for example:

```json
{
  "activePalette": "aurora",
  "renderer": "auto",
  "font": "international"
}
```

Omitting `font` preserves the original `pico8` font. Built-in presets do not
need files in your project. Custom fonts are stored in
`assets/fonts/<name>.json`. Share that file along with the project.

## Make your own

1. Enter a lowercase **New font name**, such as `my-font`.
2. Choose **COPY** to duplicate the selected font, or **BLANK** to start with
   empty ASCII, Greek and German glyphs. A visible `?` is retained as fallback.
3. Click a character in the glyph browser. Blank cells show a host-font label
   so you can identify them; `SP` denotes space. Scroll for more characters.
4. Paint inside the highlighted **4×5** area. A stroke adds or erases pixels.
   The remaining area shows the locked spacing inside the **6×8** cell.
5. To select a character directly, type/paste it into the small character box
   beneath the canvas and choose **SELECT / ADD**, or press Enter. A new
   printable, precomposed character is added if it is absent.
6. Preview your text and choose **SAVE PROJECT**. Cmd/Ctrl+S also saves;
   Cmd/Ctrl+Z and Cmd/Ctrl+Shift+Z undo/redo font selection, creation and strokes.
   Tab moves between the three text fields; Escape closes the dropdown/dialog.

Built-in fonts are read-only. Copies are independent, so editing a copy never
changes its source. BLANK intentionally leaves letters empty: draw the glyphs
needed by your game, and the ASCII letters used by the shared pause overlay.
The overlay uses the same selected font to avoid shipping a second font bank.

## Compatibility contract

| Property | Required value |
| --- | --- |
| Glyph bitmap | Exactly 4 columns × 5 rows |
| Cell/advance | Exactly 6×8 pixels, fixed width |
| Pixel depth | 1 bit: ink or transparent |
| Spacing | Two columns right, three rows below the bitmap |
| Screen grid | 40 columns × 20 rows on the native 240×160 screen |
| Required repertoire | Every ASCII codepoint 32–126, exactly once |
| Extension capacity | At most 128 additional Unicode glyphs (223 total) |
| Space | Always blank |
| Missing character | Visible `?`, one 6×8 cell per unsupported codepoint |

The editor, save/load path, compiler and runtime enforce the structural
contract. Editing JSON cannot enable larger glyphs, wider advances, extra
bitmap rows, negative pixels, duplicate glyphs or invalid codepoints.

The version-1 JSON document has `version`, `name`, `cellWidth`, `cellHeight`,
`width`, `height` and `glyphs`. Dimensions must be `6, 8, 4, 5` respectively.
Each glyph stores a numeric Unicode `codepoint` and exactly four integer
`columns`, each 0–31. Bit 0 is the top pixel, bit 4 the bottom. For example,
`{"codepoint": 937, "columns": [30,17,16,30]}` describes one Ω glyph.
All required ASCII entries must also be present in a valid document. The
filename and document name must match; built-in names cannot be overwritten.
Names use lowercase ASCII letters, digits and internal hyphens, up to 32 bytes.

## Text APIs and renderers

`print()` decodes UTF-8 printable text in Mode 4. Each codepoint advances six
pixels, including unknown characters; malformed UTF-8 bytes render `?` safely.
The return value, cursor placement, newline, tab, clipping, camera, background
control and palette semantics retain their original fixed-cell behavior.
The repeat control repeats a complete UTF-8 character. Other control parameters
remain bytes. Lua string length (`#`), `sub`, `ord` and `chr` remain byte based;
font support does not change Lua's string model.

Use **precomposed Unicode (NFC)**. Greek `ά` and German `ä` are supported;
separate combining accents are not merged by the runtime and produce fallback
glyphs. This is a bounded bitmap text system: polytonic Greek, shaping, variable
width, kerning, anti-aliasing and automatic Unicode normalization are not
implemented. Custom precomposed glyphs can be added within the extension limit.
Keeping the original 4×5 size necessarily limits the detail of accents.

Mode 0 supports the existing proven static ASCII/decimal print subset with
exactly the selected font's pixels. Its atlas deduplicates `(glyph,color)`
pairs and retains the 32-tile/OAM limits. Greek and German diacritics require
Mode 4: AUTO falls back with a source-positioned `M0_PRINT_SHAPE` diagnostic;
forced Mode 0 rejects them. Selecting a Greek/German font and printing only
ASCII can still use Mode 0. See [renderer compatibility](render-modes.md).

## GBA memory and design choices

Bitmaps stay packed at 1bpp in read-only cartridge memory. ASCII lookup is
direct; additional codepoints use a bounded binary search. Rendering does not
allocate. Neither the editor's font library nor a TrueType rasterizer ships in
the cartridge, and font bitmaps are not copied into EWRAM. On ARM, font data
uses 392 bytes for the ASCII bank and descriptor plus eight bytes per extra
glyph (four for the codepoint, four for bitmap columns): at most 1,416 bytes.
The `international` preset uses 1,024 bytes total.

Mode 0 expands only the glyph/color pairs used by its static print calls into
its existing ROM/OBJ atlas, then uploads once while blanked. This is a renderer
cache of the same font, not another selectable font. ROM/VRAM expansion is
bounded by the existing 32 text tiles; no per-frame font upload is needed.

The implementation follows [Tonc's bitmap-text guidance](https://gbadev.net/tonc/text.html)
and its [UTF-8 text-engine discussion](https://gbadev.net/tonc/tte.html): bitpack
monochrome art, keep predictable metrics, and avoid retaining every glyph in
VRAM. [Pokémon Emerald's font source](https://github.com/pret/pokeemerald/blob/master/src/fonts.c)
and its small Latin font assets informed the compact menu style. Its variable
widths cannot be imported directly without violating mTile's fixed grid.
The new arcade, Greek and German patterns are original mTile artwork; no
commercial-game font assets are bundled. The inherited PICO-8 artwork remains
under its [existing CC0 license](../fonts/pico-8-wide.otf/license.txt).

## Try it and verify

```sh
go run ./cmd/mtile edit examples/font-demo
go run ./cmd/mtile build examples/font-demo
```

![Actual mGBA capture of the demo](media/fonts/demo.png)

The [demo walkthrough](../examples/font-demo/README.md) covers selection and
scratch editing. [Benchmark evidence](benchmarks/fonts/README.md) includes
real Mode 0/4 parity captures and cycle measurements. Tests cover malformed
fonts, bounds and capacity, editable pixels, persistence, undo, UTF-8 static
and heap strings, fallback, control codes, renderer diagnostics and one-font
ROM linking. Every ASCII glyph in every preset is compared pixel for pixel
between the software renderer and the compiled Mode 0 atlas.
