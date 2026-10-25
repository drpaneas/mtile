#!/usr/bin/env python3
"""Build the mTile learning site from repository guides and a curated API index."""
from __future__ import annotations

import argparse
import html
from html.parser import HTMLParser
import json
from pathlib import Path
import posixpath
import re
import shutil
from urllib.parse import quote, unquote, urlsplit

import markdown

ROOT = Path(__file__).resolve().parents[1]
SITE = ROOT / "website"
REPO = "https://github.com/drpaneas/mtile"
CATEGORIES = json.loads((SITE / "categories.json").read_text())
APIS = json.loads((SITE / "api.json").read_text())
# Existing maintained guides remain the source of truth, with no duplicate copies.
GUIDES = [
    ("install", "Install mTile", "start", "docs/getting-started.md", "Set up your tools and build a native GBA ROM."),
    ("first-game", "Your first game: Coin Quest", "start", "internal/build/templates/coin-quest/README.md", "Play a finished game, change one value, then learn movement, collision, sound, and restart."),
    ("small-games", "Choose one small game", "start", "docs/small-games.md", "Pick one feature, open a ready project, and try one visible checkpoint."),
    ("tidewalk", "Tidewalk: maps, camera & parallax", "maps", "examples/tidewalk/README.md", "Walk a pastel coast; add one map, then a camera, then a slower distant shore."),
    ("moth-study", "Moth Study: poses & looping clips", "sprites", "examples/moth-study/README.md", "Draw one still moth, then play a saved sequence and choose its facing."),
    ("moon-door", "Moon Door: start and finish an action", "sprites", "examples/moon-door/README.md", "Open a brass garden gate using its own elapsed clock; add pause separately."),
    ("lantern-rooms", "Lantern Rooms: change rooms & spawn", "maps", "examples/lantern-rooms/README.md", "Visit a hanging garden and return; learn how Lua selects a room and its entry position."),
    ("game-loop", "The game loop", "loop", "website/content/game-loop.md", "Set up once, update your world, and draw each frame."),
    ("reference", "Language & commands", "loop", "docs/reference.md", "The Lua dialect, project files, commands, and controls."),
    ("poses-and-animations", "Shared tiles, poses & animation", "sprites", "docs/poses-and-animations.md", "Choose shared tiles, spr, pspr, or anim for your artwork, and give each action the right timing."),
    ("graphics", "Maps & drawing order", "maps", "docs/graphics-model.md", "Tile maps, layers, scrolling, and sprite placement."),
    ("tile-links", "Tile links: objects from the map", "maps", "docs/tile-links.md", "Link a shared tile to a pose or clip, then place objects by painting Map Lab."),
    ("renderers", "Renderer compatibility", "debug", "docs/render-modes.md", "Understand hardware tiles, bitmap drawing, and AUTO selection."),
    ("performance", "Frame budgets", "debug", "docs/performance.md", "Measure your cartridge and keep it within its frame budget."),
    ("lua-api", "Lua API walkthrough", "loop", "docs/lua-api.md", "Common signatures, examples, and behavior notes for every supported builtin."),
    ("audio-editor", "Music Lab walkthrough", "audio", "docs/audio/editor.md", "Compose, arrange, perform, record, and create gameplay effects."),
    ("editor", "Using the editor", "editor", "docs/ide.md", "Sprites, maps, sound, and code in one workspace."),
    ("audio", "Getting started with sound", "audio", "docs/audio/README.md", "Meet the GBA’s four Game Boy hardware sound channels."),
    ("audio-api", "Audio commands", "audio", "docs/audio/api-v2.md", "Play sound effects, songs, and direct hardware notes."),
    ("sfx", "Make a sound effect", "audio", "docs/audio/sfx.md", "Create short clips for jumps, pickups, and collisions."),
    ("music", "Compose a song", "audio", "docs/audio/music.md", "Arrange clips into parts and a complete soundtrack."),
    ("foreach", "Working with foreach", "data", "docs/loops/foreach.md", "Callbacks, lists, and safe mutation patterns."),
]
DOC_ROUTES = {(ROOT / src).resolve(): f"guides/{slug}.html" for slug, _, _, src, _ in GUIDES}
DOC_ROUTES[(ROOT / "docs/README.md").resolve()] = "guide.html"
DOC_ROUTES[(ROOT / "examples/README.md").resolve()] = "examples.html"
DOC_ROUTES[(ROOT / "docs/first-game.md").resolve()] = "guides/first-game.html"
CATEGORY_GUIDES = {
    "start": ["install", "first-game", "small-games"], "loop": ["game-loop", "reference", "lua-api"], "input": [],
    "sprites": ["moth-study", "moon-door", "poses-and-animations"], "maps": ["tidewalk", "lantern-rooms", "graphics", "tile-links"], "drawing": [], "audio": ["audio", "sfx", "music", "audio-api", "audio-editor"],
    "math": [], "data": ["foreach"], "debug": ["performance", "renderers"], "editor": ["editor"],
}
ICON_PATHS = {
    "flag": '<path d="M5 21V3h13l-3 5 3 5H5"/>',
    "loop": '<path d="M19 8a8 8 0 1 0 1 8M19 3v6h-6"/>',
    "pad": '<rect x="2" y="6" width="20" height="13" rx="4"/><path d="M5 12h6M8 9v6M16 11h.01M19 14h.01"/>',
    "sprite": '<path d="M8 3h8v3h3v12h-3v3h-3v-5h-2v5H8v-3H5V6h3zM8 9h2M14 9h2"/>',
    "map": '<path d="m3 5 6-2 6 2 6-2v16l-6 2-6-2-6 2zM9 3v16M15 5v16"/>',
    "palette": '<rect x="3" y="3" width="18" height="18" rx="3"/><path d="M3 12h18M12 3v18M7 7h1M16 16h1"/>',
    "note": '<path d="M9 18V5l11-2v13M9 9l11-2"/><ellipse cx="6" cy="18" rx="3" ry="2"/><ellipse cx="17" cy="16" rx="3" ry="2"/>',
    "math": '<path d="M3 7h7M6.5 3v8M15 7h6M3 16l7 6M10 16l-7 6M15 16h6M15 21h6"/>',
    "code": '<path d="m8 5-6 7 6 7M16 5l6 7-6 7M14 3l-4 18"/>',
    "clock": '<circle cx="12" cy="12" r="9"/><path d="M12 6v6l4 3"/>',
    "screen": '<rect x="2" y="3" width="20" height="15" rx="2"/><path d="M8 22h8M12 18v4M6 7h12"/>',
    "search": '<circle cx="10" cy="10" r="6"/><path d="m15 15 6 6"/>',
}


def esc(value):
    return html.escape(str(value), quote=True)


def icon(name):
    return f'<svg class="icon" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">{ICON_PATHS[name]}</svg>'


def relative(page, target):
    return posixpath.relpath(target, posixpath.dirname(page) or ".")


def slugify(value, separator="-"):
    # Match the repository's GitHub-style heading links.
    return re.sub(r"[^\w\- ]", "", value.lower()).replace(" ", separator)


def registry():
    source = (ROOT / "internal/transpiler/transpiler.go").read_text()
    block = source.split("var Builtins = map[string]BuiltinSpec{", 1)[1].split("\n}", 1)[0]
    return set(re.findall(r'"([a-z_][a-z0-9_]*)":\s*\{', block))


def classifications():
    source = (ROOT / "internal/transpiler/render_modes.go").read_text()
    result = {}
    for names, classification in re.findall(r"case\s+(.*?):\s*(?:(?://[^\n]*\n)\s*)*return (Renderer\w+)", source, re.S):
        for name in re.findall(r'"([a-z_][a-z0-9_]*)"', names):
            result[name] = classification
    return result


class LinkRewriter(HTMLParser):
    def __init__(self, source, page, output):
        super().__init__(convert_charrefs=False)
        self.source, self.page, self.output = source, page, output
        self.parts = []

    def link(self, value, is_image=False):
        url = urlsplit(value)
        if url.scheme or url.netloc or not url.path:
            return value
        destination = (self.source.parent / unquote(url.path)).resolve()
        if not destination.is_relative_to(ROOT):
            raise ValueError(f"link leaves repository: {value}")
        suffix = ("?" + url.query if url.query else "") + ("#" + url.fragment if url.fragment else "")
        if destination in DOC_ROUTES:
            return relative(self.page, DOC_ROUTES[destination]) + suffix
        if is_image:
            if not destination.is_file():
                raise ValueError(f"missing image {destination}")
            target = "media/" + destination.relative_to(ROOT).as_posix()
            dst = self.output / target
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(destination, dst)
            return relative(self.page, target) + suffix
        # Guides not included in the curated site still lead to their source.
        kind = "tree" if destination.is_dir() else "blob"
        return f"{REPO}/{kind}/main/{quote(destination.relative_to(ROOT).as_posix())}" + suffix

    def handle_starttag(self, tag, attrs):
        fixed = []
        for key, value in attrs:
            if value is not None and key in ("href", "src"):
                value = self.link(value, key == "src")
            fixed.append(key if value is None else f'{key}="{esc(value)}"')
        self.parts.append("<" + tag + (" " + " ".join(fixed) if fixed else "") + ">")

    def handle_endtag(self, tag):
        self.parts.append(f"</{tag}>")

    def handle_data(self, data):
        self.parts.append(data)

    def handle_entityref(self, name):
        self.parts.append(f"&{name};")

    def handle_charref(self, name):
        self.parts.append(f"&#{name};")


def render_markdown(text, source, page, output):
    md = markdown.Markdown(extensions=["fenced_code", "tables", "toc", "sane_lists", "md_in_html"],
                           extension_configs={"toc": {"slugify": slugify, "toc_depth": "2-3"}})
    rendered = md.convert(text)
    rewriter = LinkRewriter(source, page, output)
    rewriter.feed(rendered)
    return "".join(rewriter.parts), md.toc


def layout(page, title, description, body, active="", toc="", section="guide"):
    url = lambda target: relative(page, target)
    nav = ""
    for category in CATEGORIES:
        selected = ' aria-current="page"' if active == category["id"] else ""
        nav += f'<a href="{url("topics/" + category["id"] + ".html")}"{selected}>{icon(category["icon"])}<span>{esc(category["title"])}</span></a>'
    top = "".join(f'<a href="{url(route)}"' + (' aria-current="page"' if name == section else '') + f'>{label}</a>'
                  for name, route, label in [("guide", "guide.html", "Guide"), ("learn", "learn.html", "Learn"), ("api", "api.html", "API"), ("examples", "examples.html", "Examples")])
    rail = f'<aside class="on-this-page" aria-label="On this page"><p>On this page</p>{toc}</aside>' if toc and '<li>' in toc else ''
    return f'''<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{esc(title)} · mTile Guide</title><meta name="description" content="{esc(description)}">
<meta name="theme-color" content="#71609b"><meta property="og:title" content="{esc(title)} · mTile Guide"><meta property="og:description" content="{esc(description)}"><meta property="og:type" content="website">
<link rel="icon" href="{url('assets/mtile-logo.png')}" type="image/png"><link rel="stylesheet" href="{url('assets/site.css')}">
<script src="{url('search-index.js')}" defer></script><script src="{url('assets/site.js')}" defer></script></head>
<body data-root="{url('index.html')[:-10]}"><a class="skip-link" href="#main">Skip to content</a>
<header class="topbar"><a class="brand" href="{url('index.html')}" aria-label="mTile title screen"><img class="brand-logo" src="{url('assets/mtile-logo.png')}" width="220" height="48" alt="mTile"></a>
<button class="menu-toggle" type="button" aria-controls="sidebar" aria-expanded="false">Topics</button>
<nav class="topnav" aria-label="Main navigation">{top}</nav><a class="title-return" href="{url('index.html')}">Back to title</a><a class="github" href="{REPO}">GitHub <span aria-hidden="true">↗</span></a></header>
<aside id="sidebar" class="sidebar"><a class="home-link" href="{url('guide.html')}">{icon('screen')} Guide home</a><p class="nav-label">Explore the guide</p><nav aria-label="Guide topics">{nav}</nav>
<div class="sidebar-bottom"><span class="status-dot" aria-hidden="true"></span> Built for real GBA games<p>Lua in. Cartridge out.</p><a href="{url('guides/install.html')}">Set up mTile</a></div></aside>
<div class="workspace"><div class="searchbar"><div class="breadcrumb"><a href="{url('guide.html')}">Guide</a><span aria-hidden="true">/</span><span>{esc(title)}</span></div>
<div class="search-wrap"><label for="guide-search" class="sr-only">Search the guide</label>{icon('search')}<input id="guide-search" type="search" placeholder="Search a topic or function…" autocomplete="off" aria-controls="search-results"><kbd aria-hidden="true">/</kbd></div></div>
<section id="search-results" class="search-results" hidden aria-label="Search results"><div class="search-heading"><h2>Search results</h2><button id="close-search" type="button">Close</button></div><p id="search-status" role="status" aria-live="polite"></p><ul id="search-list"></ul></section>
<div class="page-layout{' with-toc' if rail else ''}"><main id="main" tabindex="-1">{body}</main>{rail}</div>
<footer class="footer"><span>Made for curious game makers.</span><span>mTile is in early development. <a href="{url('guides/renderers.html')}">Know the limits.</a></span><a href="{REPO}/blob/main/LICENSE">MIT license</a><a href="{url('art-credits.html')}">Artwork credits</a></footer><p class="artwork-notice">mTile is independent and is not affiliated with, sponsored by, or endorsed by Nintendo. Game Boy Advance is a trademark of Nintendo. Third-party artwork belongs to its respective owners and is excluded from the MIT license. <a href="{url('art-credits.html')}">Sources &amp; rights</a>.</p></div>
</body></html>'''


def guide_links(category, page):
    result = []
    for slug, title, group, source, description in GUIDES:
        if slug in CATEGORY_GUIDES[category]:
            result.append(f'<a class="guide-link" href="{relative(page, "guides/" + slug + ".html")}"><span>{esc(title)}</span><small>{esc(description)}</small></a>')
    return "".join(result)


def category_tiles(page):
    parts = []
    for category in CATEGORIES:
        items = [api for api in APIS if api['category'] == category['id']]
        functions = ''.join(f'<a class="function-link" href="{relative(page, "api/" + api["name"] + ".html")}">{esc(api["name"])}<span aria-hidden="true">()</span></a>' for api in items)
        parts.append(f'''<section class="topic-group" id="{category['id']}"><h3><a href="{relative(page, 'topics/' + category['id'] + '.html')}"><span class="topic-icon {category['id']}">{icon(category['icon'])}</span>{esc(category['title'])}</a></h3><p>{esc(category['description'])}</p><div class="function-links">{functions}</div>{guide_links(category['id'], page)}</section>''')
    return '<div class="topic-grid">' + ''.join(parts) + '</div>'


def page_heading(title, description):
    return f'<div class="page-heading"><h1>{esc(title)}</h1><p>{esc(description)}</p></div>'


def build(output):
    output = output.resolve()
    # Refuse arbitrary destinations: rebuilding may replace files, but never source.
    if output != (SITE / '_site').resolve():
        raise ValueError('build output must be website/_site')
    output.mkdir(parents=True, exist_ok=True)
    names = [api['name'] for api in APIS]
    if len(names) != len(set(names)) or set(names) != registry() - {'sample'}:
        raise ValueError('API index must cover every project-supported builtin; sample is explicitly unsupported in Audio v2')
    classes = classifications()
    if set(names) - classes.keys():
        raise ValueError('missing renderer classifications')
    # Assets are fully generated: remove retired artwork/fonts on every rebuild.
    if (output / 'assets').exists():
        shutil.rmtree(output / 'assets')
    shutil.copytree(SITE / 'assets', output / 'assets')
    shutil.copyfile(ROOT / 'cmd/editor/assets/mtile-header.png', output / 'assets/mtile-logo.png')
    shutil.copyfile(ROOT / 'fonts/JetBrainsMono/JetBrainsMono-Regular.ttf', output / 'assets/JetBrainsMono-Regular.ttf')
    shutil.copyfile(ROOT / 'fonts/JetBrainsMono/OFL.txt', output / 'assets/FONT-LICENSE.txt')
    media = output / 'assets'
    shutil.copyfile(ROOT / 'docs/media/coin-quest/preview.png', media / 'coin-quest.png')
    shutil.copyfile(ROOT / 'docs/media/background-layers.png', media / 'background-layers.png')
    search = []
    written = set()

    def write(page, title, description, body, active='', toc='', section='guide'):
        target = output / page
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(layout(page, title, description, body, active, toc, section), encoding='utf-8')
        written.add(page)
        searchable = re.sub(r'<[^>]+>', ' ', body)
        search.append({'title': title, 'description': description, 'url': page, 'category': active,
                       'text': re.sub(r'\s+', ' ', html.unescape(searchable))})

    menu = (SITE / 'templates/menu.html').read_text()
    (output / 'index.html').write_text(menu, encoding='utf-8')
    written.add('index.html')
    search.append({'title': 'mTile title screen', 'description': 'Start your own GBA game, open the guide, browse examples, or install mTile.', 'url': 'index.html', 'category': '', 'text': 'New game The guide Cartridge shelf Getting ready'})

    hero = '''<section class="welcome"><div class="welcome-copy"><h1>Make the game <br>you wish you had.</h1><p>Make Game Boy Advance games with Lua. Start with a playable game and change one thing. You do not need to learn the whole API first.</p><div class="start-scene"><span class="character-cue">Your first cartridge starts here</span><a class="button primary" href="guides/first-game.html">Start with Coin Quest <span aria-hidden="true">↗</span></a></div><a class="text-link" href="guides/install.html">Need to install mTile?</a></div><div class="cartridge-demo"><div class="console-shell"><span class="console-label">mTile / Game Boy Advance</span><div class="console-screen"><img src="assets/coin-quest.png" alt="Coin Quest running on the GBA: a little explorer, a gold coin, and a score of one out of five." width="720" height="480"></div><div class="console-controls"><span class="dpad" aria-hidden="true"></span><span class="demo-label">Your first adventure: Coin Quest</span><span class="game-buttons" aria-hidden="true"><i>B</i><i>A</i></span></div></div><span class="capture-label">A real ROM. Original art. Four small lessons.</span></div></section>'''
    steps = '''<div class="section-heading"><div><h2>Start here</h2><p>Each step gives you something to try. Stop with a working game.</p></div></div><section class="learning-strip" aria-label="Getting started steps"><a href="guides/install.html"><span>1</span><div><strong>Get set up</strong><small>Install mTile and check your tools with <code>mtile doctor</code>.</small></div></a><a href="guides/first-game.html"><span>2</span><div><strong>Play, then change one value</strong><small>Move around Coin Quest. Change the explorer’s speed and play again.</small></div></a><a href="guides/moth-study.html"><span>3</span><div><strong>Try a pose, then a clip</strong><small>Open Moth Study. Draw one still picture, then give its wings a saved sequence.</small></div></a></section>'''
    body = hero + steps + '<div class="section-heading"><div><h2>Look things up as you need them</h2><p>The topics below are a reference, not a reading list. Return when your game needs something new.</p></div><a href="api.html">Full API index</a></div><details><summary>Browse topics and functions</summary>' + category_tiles('guide.html') + '</details>'
    write('guide.html', 'Home', 'Learn to make Game Boy Advance games with Lua. Beginner tutorials, working examples, and a searchable mTile API reference.', body)
    write('api.html', 'API reference', 'Find a mTile function by topic. Learn its common use and renderer requirements.', page_heading('A function for that.', 'Find what you need, see it in context, and get back to making your game.') + category_tiles('api.html') + '<p class="notice">Native Audio v2 does not support PCM playback or <code>sample()</code>. Use SFX clips, songs, and hardware notes.</p>', section='api')
    for cat in CATEGORIES:
        page = f'topics/{cat["id"]}.html'
        body = page_heading(cat['title'], cat['description']) + '<div class="guide-list">' + guide_links(cat['id'], page) + '</div>'
        items = [api for api in APIS if api['category'] == cat['id']]
        if items:
            body += '<div class="api-list">' + ''.join(f'<a href="../api/{api["name"]}.html"><code>{esc(api["name"])}()</code><span>{esc(api["summary"])}</span><span aria-hidden="true">↗</span></a>' for api in items) + '</div>'
        if cat['id'] == 'input':
            body += '<div class="notice"><strong>Know your buttons:</strong> 0 left, 1 right, 2 up, 3 down, 4 B, 5 A, 6 Start, 7 Select, 8 L, 9 R. In the preview, Z is A and X is B.</div>'
        if cat['id'] == 'drawing':
            body += '<div class="notice">A GBA screen is 240×160 pixels. The top-left is (0, 0); X goes right and Y goes down. Colors are palette indices, not RGB values.</div>'
        write(page, cat['title'], cat['description'], body, cat['id'])
    for slug, title, group, source, description in GUIDES:
        page = f'guides/{slug}.html'
        text = (ROOT / source).read_text()
        text = re.sub(r'^# .+\n', '', text, count=1)
        article, toc = render_markdown(text, ROOT / source, page, output)
        body = page_heading(title, description) + f'<article class="prose">{article}</article><a class="source-link" href="{REPO}/blob/main/{source}">View this guide on GitHub</a>'
        write(page, title, description, body, group, toc, 'learn')
    for api in APIS:
        page = f'api/{api["name"]}.html'
        classification = classes[api['name']]
        artwork_guide = '<p><a href="../guides/poses-and-animations.html">When should I use spr, pspr, or anim?</a></p>' if api['name'] in ('spr', 'pspr', 'anim') else ''
        label, detail = {
            'RendererIndependent': ('Renderer independent', 'This function does not select a renderer. Its CPU and memory costs still count toward your frame budget.'),
            'RendererBoth': ('Both renderers', 'Implemented in both renderers. Mode 0 and AUTO can still impose placement, mutation, and resource checks.'),
            'RendererMode0Conditional': ('Mode 0 with conditions', 'Hardware rendering is available only for proven call shapes within the resource limits. AUTO falls back to Mode 4 when those checks fail.'),
            'RendererMode4Only': ('Mode 4 drawing', 'This needs the bitmap renderer. AUTO selects Mode 4 when this function is reachable. A forced Mode 0 build reports an error.'),
        }[classification]
        body = f'''<div class="api-heading"><a class="back-topic" href="../topics/{api['category']}.html">{next(c['title'] for c in CATEGORIES if c['id']==api['category'])}</a><h1>{api['name']}<span>()</span></h1><p>{esc(api['summary'])}</p></div><article class="prose"><h2 id="common-use">Common use</h2><pre><code class="language-lua">{esc(api['signature'])}</code></pre><h2 id="example">Example</h2><p>This is a snippet to use in your game. Drawing belongs in <code>_draw()</code>; input and game logic usually belong in <code>_update()</code>. Create any referenced assets first.</p><pre><code class="language-lua">{esc(api['example'])}</code></pre><h2 id="remember">Keep in mind</h2><p>{esc(api['note'])}</p><div class="compatibility"><span class="compat-label">{label}</span><p>{detail}</p><a href="../guides/renderers.html#implemented-mode-0-subset">Read the exact renderer restrictions</a></div><h2 id="next">Keep exploring</h2>{artwork_guide}<div class="related">{''.join(f'<a href="{other["name"]}.html">{other["name"]}()</a>' for other in APIS if other['category']==api['category'] and other['name']!=api['name'])}</div></article>'''
        toc = '<ul><li><a href="#common-use">Common use</a></li><li><a href="#example">Example</a></li><li><a href="#remember">Keep in mind</a></li><li><a href="#next">Keep exploring</a></li></ul>'
        write(page, api['name'] + '()', api['summary'], body, api['category'], toc, 'api')
    credits = page_heading('Credits & rights', 'The tools, type, captures, and names behind this little world.')
    credits += '<article class="prose"><h2 id="mtile">mTile</h2><p>The application and website use the same mTile logo. The title-screen handheld is a CSS illustration; Coin Quest captures show an actual mTile ROM. Original project code and artwork are covered by the <a href="' + REPO + '/blob/main/LICENSE">project MIT license</a>, except where separate notices identify third-party material.</p><h2 id="fonts">Fonts</h2><ul><li><a href="https://github.com/google/fonts/tree/main/ofl/rubik">Rubik</a>: <a href="assets/RUBIK-OFL.txt">SIL Open Font License</a>.</li><li><a href="https://www.jetbrains.com/lp/mono/">JetBrains Mono</a>: <a href="assets/FONT-LICENSE.txt">SIL Open Font License</a>.</li></ul><h2 id="third-party">Nintendo &amp; other third-party material</h2><p>mTile is an independent project and is not affiliated with, sponsored by, or endorsed by Nintendo. Nintendo and Game Boy Advance are trademarks of Nintendo. Nintendo characters and artwork visible within documented example games or editor captures remain © Nintendo; other third-party artwork belongs to its respective owners. These materials are excluded from mTile’s MIT license.</p><p>The title screen and guide decoration do not use Nintendo character illustrations. Attribution is not a license or permission to reuse third-party artwork. No permission from Nintendo is claimed.</p><p>See the <a href="' + REPO + '/blob/main/docs/third-party/README.md">project third-party notices</a> and the source documentation accompanying individual examples for additional ownership and license information.</p></article>'
    write('art-credits.html', 'Credits & rights', 'Sources and licenses for mTile artwork, fonts, captures, and third-party names.', credits)

    learn = page_heading('Small steps. A real game.', 'Start with something playable, then learn how each piece works.')
    learn += '<div class="course-feature"><img src="assets/coin-quest.png" alt="Coin Quest running on the GBA" width="720" height="480"><div><span class="welcome-note">Your first game</span><h2>Coin Quest</h2><p>A tiny explorer, five coins, and your first finished game. Four working checkpoints take you from movement to a win screen.</p><a class="button primary" href="guides/first-game.html">Start the walkthrough</a></div></div>'
    learn += '<div class="lesson-list">' + ''.join(f'<a href="guides/first-game.html#{anchor}"><span>{n}</span><div><h3>{title}</h3><p>{desc}</p></div></a>' for n, title, desc, anchor in [('1', 'Move your explorer', 'Read buttons and stay inside the play area.', '3-learn-movement-from-a-working-checkpoint'), ('2', 'Collect your first coin', 'Detect an overlap and increase the score.', '4-add-coins-and-collision'), ('3', 'Make a little noise', 'Play a two-note hardware sound effect.', '5-give-pickups-a-sound'), ('4', 'Win. Restart. Make it yours.', 'Add a goal and a way to play again.', '6-add-a-finish-and-restart')]) + '</div>'
    learn += '<div class="notice">Allow about 15 minutes after setup. First-time builds can take longer. New here? <a href="guides/install.html">Install mTile first.</a></div>'
    learn += '<div class="notice"><strong>Ready for one new feature?</strong> <a href="guides/small-games.html">Choose one small game</a> for maps and parallax, poses and clips, an action clock, or room transitions. Each has ready artwork and complete runnable lessons; choose the feature you want.</div>'
    write('learn.html', 'Learn', 'Build your first GBA game with four working Coin Quest lessons.', learn, 'start', section='learn')
    text = (ROOT / 'examples/README.md').read_text()
    article, toc = render_markdown(re.sub(r'^# .+\n', '', text, count=1), ROOT / 'examples/README.md', 'examples.html', output)
    body = page_heading('Learn by opening a game.', 'Small, working projects you can read, run, and change.') + '<figure class="example-capture"><img src="assets/background-layers.png" alt="Four scrolling backgrounds and a sprite from the background-layers example" width="720" height="480"><figcaption>Background layers — captured from a real mTile ROM.</figcaption></figure><article class="prose">' + article + '</article>'
    write('examples.html', 'Examples', 'Explore mTile game examples, from sprites and input to parallax and hardware audio.', body, section='examples')
    (output / 'search-index.js').write_text('window.MTILE_SEARCH = ' + json.dumps(search, ensure_ascii=True).replace('<', '\\u003c') + ';\n')
    (output / '.nojekyll').touch()
    # Remove only stale generated HTML from an earlier build, never user source.
    for old in output.rglob('*.html'):
        if old.relative_to(output).as_posix() not in written:
            old.unlink()
    print(f'Built {len(written)} static pages and {len(APIS)} API entries in {output}')
    return output


if __name__ == '__main__':
    argparse.ArgumentParser(description=__doc__).parse_args()
    build(SITE / '_site')
