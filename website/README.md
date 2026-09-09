# mTile Guide website

A static learning site with category browsing, dedicated API pages, a first-game
course, full-text search, and links back to the project's maintained guides.
The category-first navigation is inspired by the
[Nerdy Teachers guide](https://nerdyteachers.com/PICO-8/Guide/); its text,
images, icons, and CSS are not copied. mTile's art, screenshots, and identity
remain its own.

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
| `build.py` | Page routes, guide selection, templates, and link rewriting |
| `assets/site.css` | Layout, typography, responsive behavior, and colors |
| `assets/site.js` | Search, mobile navigation, and copy buttons |
| Repository Markdown | Installation, tutorial, editor, audio, performance, and renderer guides |

Long guides are rendered from the existing repository files on every build.
Renderer classifications are read from `internal/transpiler/render_modes.go`.
The build fails if the API index drifts from the public builtin registry.
`sample` is deliberately excluded because native Audio v2 projects reject it;
the API landing page explains that limitation.

API examples show common usage, not every accepted compatibility overload.
When editing them, check the runtime, public Lua registry, and native asset
validation. Update renderer documentation alongside any actual API change, as
required by [AGENTS.md](../AGENTS.md).

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
changes, and exercise search, copy, and the topic menu with the keyboard.

The **Website** GitHub Actions workflow builds and tests the site and uploads
an `mtile-guide` artifact. It does not publish it automatically.

## Hosting

Upload the **contents** of `website/_site/` to any static host, including GitHub
Pages. Keep `.nojekyll` if using GitHub Pages. The build has no host-specific
configuration, routing rewrites, or backend endpoint requirements.

A real domain, canonical URL, sitemap, and public deployment can be configured
once the publishing destination is chosen. The local preview is not a public
release, and no repository Pages settings are changed by this build.

## Design and licensing

The palette takes its cue from purple GBA shells: deep plum navigation,
muted purple surfaces, lilac controls, and small mint/gold accents. The category
index stays compact, while article pages leave room for readable code and a
section table of contents. The console illustration is CSS and the icons are
original SVG. The screen contains an actual Coin Quest ROM capture.

The bundled JetBrains Mono font is used for code and the wordmark. Its OFL
license is copied into the output as `assets/FONT-LICENSE.txt`. Body text uses
local system fonts. Original site code and game artwork follow the repository's
[MIT license](../LICENSE); palette and other component attribution remain in
the [project notices](../docs/third-party/README.md).
