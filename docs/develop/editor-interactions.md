# Develop editor interactions

The native Develop editor now supports these interactions:

| Action | Input |
| --- | --- |
| Scroll vertically | Mouse wheel, trackpad, or the right scrollbar |
| Scroll horizontally | Horizontal trackpad motion, Shift + wheel, or the bottom scrollbar |
| Select a word | Double-click; keep dragging to extend by whole words |
| Select a line | Triple-click or click its line number; drag to extend by lines |
| Extend a selection beyond the viewport | Drag outside the code area's edges |
| Insert an indented line | Enter |
| Indent / unindent | Tab / Shift + Tab |
| Reindent the document | **INDENT** in the file tab bar, or Alt + Shift + F |
| Undo / redo | Cmd/Ctrl + Z / Cmd/Ctrl + Shift + Z; Cmd/Ctrl + Y also redoes |
| Go to document boundaries | Ctrl + Home/End; on macOS, Cmd + Up/Down |
| Go to line boundaries on macOS | Cmd + Left/Right |

To try these controls, open **Develop**. For an existing game, run this from
the folder containing its `project.json`:

```sh
mtile edit .
```

## How the fixes work

Previously, every idle update moved the viewport back to the caret. That undid
manual scrolling whenever the caret was outside the visible area. The editor
now reveals the caret only for editing and keyboard navigation. Manual scroll
positions persist. Fractional trackpad motion accumulates instead of turning
each small event into a full line, and both scrollbars clamp to the document.

Click tracking distinguishes single, double, and triple clicks. Word selection
uses Unicode letters, digits, and underscores; punctuation remains individually
selectable. Selection dragging retains its initial word or line anchor and
scrolls when held outside the viewport. Clicking code also returns typing focus
from Find to the document. Row hit testing now matches the rendered row origin.

Lua indentation follows block keywords and delimiters while ignoring their
appearance inside strings and comments. Enter and completed closing keywords
adjust indentation as appropriate. The editor infers the existing indentation
style, including Celeste's two-space style. Pasting inserts the supplied text
literally.

**INDENT adjusts leading whitespace only.** It preserves numeric spelling,
operators, strings, multiline literal contents, and program logic. It does not
rewrite spacing within expressions. The operation preserves the selection and
is reversible with one Undo.

Line position lookups now reuse a cache invalidated by edits and undo/redo.
Finding the longest line takes one pass through the document, avoiding repeated
full-document scans during viewport updates.

## Verification

Regression tests cover persistent manual scrolling, fractional wheel input,
scrollbar bounds, word/line selection and reverse dragging, selection beyond
the viewport, hit testing, Find focus, and layout at multiple window sizes.
Model tests cover Lua block indentation, literal/comment preservation,
indentation styles, undo/redo, Unicode selections, and line-cache invalidation.

Passed on 2026-09-06:

```sh
go test ./internal/codeeditor ./internal/editor ./cmd/editor -count=1 -timeout=2m
go test ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```

Native inspection used a temporary copy of the Celeste project. It confirmed
the scrollbar/button layout and that wheel scrolling stays in place while the
caret remains offscreen. Synthetic native clicks and keyboard input were not
delivered reliably during that inspection, so double-click and formatting
verification rests on the model/controller tests above; a complete native
input smoke test remains outstanding. The original Celeste source was not
changed by this editor work.
