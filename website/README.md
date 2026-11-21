# mTile Guide website

A static learning site with category browsing, dedicated API pages, a first-game
course, full-text search, and links back to the project's maintained guides.
The category-first navigation is inspired by the
[Nerdy Teachers guide](https://nerdyteachers.com/PICO-8/Guide/); its text,
images, icons, and CSS are not copied. mTile keeps its own identity and actual
game/editor captures. An indigo handheld title screen opens into the guide.
The credits page identifies the artwork, palette and fonts used on the site.

## Build and preview

From the repository root, with Python 3.11 or newer:

```sh
python3 -m venv website/.venv
website/.venv/bin/python -m pip install -r website/requirements.txt
website/.venv/bin/python website/build.py
python3 -m http.server 4173 --bind 127.0.0.1 --directory website/_site
```

Open **http://127.0.0.1:4173/**. On Windows, use
`website\.venv\Scripts\python.exe` in place of `website/.venv/bin/python`.
Rebuild and refresh after changing source files. No Node, Go, database, CDN,
external font service, or production server is needed to serve the result.

The generated directory is `website/_site/`. Do not edit it: source changes
belong in the files below. It is ignored by Git.

## Where content lives

| Source | Purpose |
| --- | --- |
| `categories.json` | Topic names, descriptions, and ordering |
| `content/` | Original introductory lessons |
| `api.json` | Common call shapes, short examples, and beginner explanations |
| `build.py` | Page routes, guide selection, shared markup, asset copying, and link rewriting |
| `templates/menu.html` | Handheld title screen and its navigation links |
| `assets/menu.css` | Title-screen console illustration and responsive styling |
| `assets/menu.js` | Focus and arrow-key selection in the title-screen menu |
| `assets/site.css` | Guide layout, typography, responsive behavior, and colors |
| `assets/site.js` | Search, mobile navigation, and copy buttons |
| Repository Markdown | Installation, tutorial, editor, audio, performance, and renderer guides |

Long guides are rendered from the existing repository files on every build.
Renderer classifications are read from `internal/transpiler/render_modes.go`.
The build fails if the API index drifts from the public builtin registry.
`sample` is deliberately excluded because native Audio v2 projects reject it;
the API landing page explains that limitation.

The memory and GC feature comes from `docs/garbage-collection.md`, rendered by
`build.py` at `guides/garbage-collection.html` and styled only there by
`assets/gc.css`. It extends the existing design with a magazine-style article
and an actual Coin Quest capture. Keep diagram labels as live, selectable HTML
and preserve their schematic meaning: object boxes do not represent equal
allocation sizes, reusable memory need not be erased, and timing stages are
not measured durations. Keep the reuse sequence clear that it shows one table
at different times.

`docs/lua-api.md` is generated from the same `api.json`, so the repository and
website share API explanations. After editing API text, run:

```sh
python3 scripts/docs/generate_lua_api.py
python3 scripts/docs/generate_lua_api.py --check
```

The site tests reject a stale generated guide. Editor screenshots are copied
from the Markdown image references during each build; see the
[capture instructions](../docs/media/README.md#editor-screenshots).

API examples show common usage, not every accepted compatibility overload.
The `TestWebsiteAPIExamples` Go test parses/transpiles every API snippet with
fixture assets. It checks compilation, not pixel output or performance. When
editing examples, also check runtime behavior and native asset validation. Update renderer documentation alongside any actual API change, as
required by [AGENTS.md](../AGENTS.md).

`index.html` is the title/menu screen; `guide.html` is the guide landing page.
The menu leads with the latest desktop release, followed by installation,
Coin Quest and the examples. The header links directly to the guide.
The guide keeps the existing lesson, category, API, example, and long-guide
routes. Its navigation includes a return to the title screen.

All site routes use relative URLs, including search results. The output can
live at a domain root or a subdirectory such as `/mtile/`. Search is a local
JavaScript index and makes no network requests. Text and navigation are static
HTML; they remain available without JavaScript. Links to additional research
or example source open the repository on GitHub.

## Verify

```sh
website/.venv/bin/python -m unittest discover -s website -p 'test_*.py'
```

The checks build the entire site and verify every local link and heading
fragment, API coverage, renderer labels, search routes, image alternatives,
and page structure. Also inspect a desktop and mobile browser after layout
changes, and exercise title-menu selection, direct guide entry, search, copy,
and the topic menu with the keyboard. Menu links work without JavaScript; its
script adds Arrow Up/Down and Home/End focus navigation.

## Hosting

The [live mTile guide](https://drpaneas.com/mtile/) is hosted on GitHub Pages.
The **Website** workflow builds and tests changes to the site and its inputs.
Pull requests produce a downloadable `mtile-guide` preview. On `main`, a
successful build also publishes `website/_site/` to the `github-pages`
environment. To republish, run the workflow manually on `main`.

## Design and licensing

The selected direction evokes an indigo GBA and the instruction booklet in its
box. The title screen uses a CSS console shell, a dark screen, a green power
light, and four real navigation choices. The guide carries that identity into
cream reading surfaces, indigo chapter navigation, soft corners, and readable prose.
See [DESIGN.md](DESIGN.md) for the design system and maintenance guidance.

There is one logo: the application asset at
`cmd/editor/assets/mtile-header.png`, copied into the website at build time. Do
not create a separate menu or website wordmark. The console illustration is
CSS, the topic icons are original SVG, and the demo screen contains an actual
Coin Quest ROM capture. The Sprite Lab
captures use the original [clockwork surveyor generator](../examples/nerdo/gen_assets.py).

Rubik supplies title-screen body text and guide display headings; guide prose
and controls use Trebuchet MS / Segoe UI / system sans-serif. JetBrains Mono
supplies code, keyboard hints, API names, and the title-screen menu. Rubik is
bundled as `assets/Rubik.ttf` with `assets/RUBIK-OFL.txt`; JetBrains Mono and its
SIL Open Font License notice are copied from the repository during the build.
No external font service is required.

Original site code and original project artwork follow the repository's
[MIT license](../LICENSE). The editor uses [DawnBringer's Aurora palette](https://lospec.com/palette-list/aurora).
The [capture notes](../docs/media/README.md) identify the projects shown in the editor.
The title screen and guide pages link to credits and identify Game Boy Advance as a Nintendo trademark.
The [project notices](../docs/third-party/README.md) cover bundled components.

The [content review](../docs/website-content-review.md) records factual corrections,
implementation evidence, and the limits of automated verification. Run the
API snippet gate with `go test ./internal/transpiler -run TestWebsiteAPIExamples`.

Effective mTile uses `docs/performance.md` at the existing
`guides/performance.html` route. Its page-only assets are `effective-mtile.css`
and `effective-mtile.js`. The storage slider models element bytes, not CPU
speed; its initial example remains readable without JavaScript. The earlier
engineering reference remains in a disclosure and old hash links open it.
`TestEffectiveMtilePrograms` executes all 22 published Lua programs with their
stated assets and compares behavior; `test_effective_mtile.py` checks the site
structure, scales, accessibility hooks and preserved links. Advanced compiler
notes identify the optimization revision they require.
