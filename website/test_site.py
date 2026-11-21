"""Check generated routes, API coverage, anchors, and portable static assets."""
from html.parser import HTMLParser
import json
import importlib.util
from pathlib import Path
import re
import unittest
from urllib.parse import unquote, urljoin, urlsplit
from xml.etree import ElementTree

import build


class Page(HTMLParser):
    def __init__(self, path):
        super().__init__()
        self.links, self.ids, self.h1, self.missing_alt = [], set(), 0, []
        self.elements = []
        self.text = []
        self.feed(path.read_text())

    def handle_data(self, data):
        self.text.append(data)

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        self.elements.append((tag, attrs))
        if 'id' in attrs:
            self.ids.add(attrs['id'])
        if tag == 'h1':
            self.h1 += 1
        if tag == 'img' and 'alt' not in attrs:
            self.missing_alt.append(attrs.get('src'))
        for attribute in ('href', 'src'):
            if attribute in attrs:
                self.links.append(attrs[attribute])


class SiteTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.output = build.build(build.SITE / '_site')
        cls.pages = {path.resolve(): Page(path) for path in cls.output.rglob('*.html')}

    def test_all_static_links_and_fragments_resolve(self):
        errors = []
        for path, page in self.pages.items():
            for link in page.links:
                url = urlsplit(link)
                if url.scheme or url.netloc:
                    continue
                target = (path.parent / unquote(url.path)).resolve() if url.path else path
                if not target.is_relative_to(self.output) or not target.is_file():
                    errors.append(f'{path.name}: missing or escaping link {link}')
                elif url.fragment and target.suffix == '.html' and unquote(url.fragment) not in self.pages[target].ids:
                    errors.append(f'{path.name}: missing anchor {link}')
        self.assertEqual(errors, [])

    def test_api_coverage_and_renderer_contract(self):
        supported = build.registry()
        indexed = {api['name'] for api in build.APIS}
        self.assertEqual(supported, indexed)
        for name in indexed:
            self.assertIn(name, build.classifications())
            self.assertTrue((self.output / 'api' / f'{name}.html').is_file())
        self.assertTrue((self.output / 'api/sample.html').exists())
        self.assertTrue((self.output / 'api/pcm_music.html').exists())
        self.assertIn('imported Direct Sound audio', (self.output / 'api.html').read_text())
        self.assertIn('Mode 4 drawing', (self.output / 'api/rectfill.html').read_text())
        self.assertIn('Mode 0 with conditions', (self.output / 'api/spr.html').read_text())

    def test_syntax_highlighting_in_guide_and_api_pages(self):
        for route in ('guides/first-game.html', 'api/btn.html'):
            with self.subTest(route=route):
                page = self.pages[(self.output / route).resolve()]
                styles = {attrs.get('class') for tag, attrs in page.elements if tag == 'span'}
                self.assertTrue({'tok-keyword', 'tok-function', 'tok-number'} <= styles)

        api = (self.output / 'api/btn.html').read_text()
        self.assertIn('<span class="tok-function">btn</span>(index)', api)
        install = (self.output / 'guides/install.html').read_text()
        self.assertIn('<span class="tok-function">mtile</span> build mygame', install)

    def test_highlighting_preserves_copyable_text_and_escaping(self):
        examples = {
            'lua': ('-- compare\nif x < 2 then print("<tag>&amp;") end', 'tok-comment'),
            'sh': ('\n  echo "$HOME"\n\n', 'tok-string'),
            'json': ('{"limit": 3, "valid": true}', 'tok-key'),
            'text': ('a plain <diagram> & its labels', None),
        }
        for language, (source, style) in examples.items():
            with self.subTest(language=language):
                block = f'<pre><code class="language-{language}">{build.esc(source)}</code></pre>'
                code = ElementTree.fromstring(build.CODE_BLOCK.sub(build.highlight_block, block)).find('code')
                self.assertEqual(''.join(code.itertext()), source)
                self.assertTrue(all(child.tag == 'span' for child in code))
                if style:
                    self.assertIsNotNone(code.find(f"span[@class='{style}']"))
                else:
                    self.assertEqual(len(code), 0)

    def test_generated_lua_guide_matches_shared_api_data(self):
        script = build.ROOT / 'scripts/docs/generate_lua_api.py'
        spec = importlib.util.spec_from_file_location('generate_lua_api', script)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        self.assertEqual((build.ROOT / 'docs/lua-api.md').read_text(), module.render())
        editor = (self.output / 'guides/editor.html').read_text()
        for name in ('shared-tiles', 'poses', 'animations', 'color-variants',
                     'map-rooms', 'audio', 'develop', 'preferences'):
            self.assertIn(f'{name}.png', editor)

    def test_accessible_structure_and_no_root_absolute_assets(self):
        for path, page in self.pages.items():
            with self.subTest(page=path.name):
                self.assertEqual(page.h1, 1)
                self.assertIn('main', page.ids)
                self.assertEqual(page.missing_alt, [])
                self.assertFalse(any(link.startswith('/') for link in page.links))
                self.assertIn('lang="en"', path.read_text())

    def test_title_menu_and_guide_are_separate_portable_entrypoints(self):
        menu = self.pages[(self.output / 'index.html').resolve()]
        guide = self.pages[(self.output / 'guide.html').resolve()]
        self.assertTrue(any(tag == 'nav' and 'game-menu' in attrs.get('class', '').split()
                            for tag, attrs in menu.elements))
        self.assertNotIn('guide-search', menu.ids)
        self.assertIn('guide-search', guide.ids)
        menu_scripts = [attrs.get('src') for tag, attrs in menu.elements if tag == 'script']
        self.assertIn('assets/menu.js', menu_scripts)
        self.assertNotIn('assets/site.js', menu_scripts)
        guide_scripts = [attrs.get('src') for tag, attrs in guide.elements if tag == 'script']
        self.assertIn('assets/site.js', guide_scripts)
        for route in ('guide.html', 'guides/first-game.html', 'examples.html', 'guides/install.html'):
            self.assertIn(route, menu.links)
            for base in ('https://mtile.example/', 'https://mtile.example/projects/mtile/'):
                with self.subTest(base=base, route=route):
                    self.assertEqual(urljoin(base + 'index.html', route), base + route)
        self.assertEqual(build.DOC_ROUTES[(build.ROOT / 'docs/README.md').resolve()], 'guide.html')

    def test_download_is_primary_and_examples_use_the_installed_cli(self):
        releases = 'https://github.com/drpaneas/mtile/releases/latest'
        checksums = releases + '/download/examples.sha256'
        for route, primary_class in [('index.html', 'selected'), ('guide.html', 'primary')]:
            page = self.pages[(self.output / route).resolve()]
            actions = [attrs['href'] for tag, attrs in page.elements
                       if tag == 'a' and primary_class in attrs.get('class', '').split()]
            self.assertIn(releases, actions, route)
            self.assertIn(checksums, page.links, route)
            self.assertIn('guides/install.html#verify-the-hello-rom', page.links, route)

        install_path = (self.output / 'guides/install.html').resolve()
        install = self.pages[install_path]
        sections = [attrs['id'] for tag, attrs in install.elements if tag == 'h2' and 'id' in attrs]
        self.assertLess(sections.index('using-a-desktop-bundle'), sections.index('build-the-desktop-tools'))
        self.assertIn('use-mtile-in-a-terminal', install.ids)
        self.assertIn(releases, install.links)
        self.assertIn(checksums, install.links)
        self.assertIn('verify-the-hello-rom', sections)
        verification = install_path.read_text().split('id="verify-the-hello-rom"', 1)[1].split('<h2', 1)[0]
        snippets = [''.join(ElementTree.fromstring(block).itertext()).strip()
                    for block in re.findall(r'<pre>.*?</pre>', verification, re.DOTALL)]
        self.assertIn('mtile build examples/hello', snippets)
        bundle = (build.ROOT / 'scripts/release/BUNDLE-README.md').read_text()
        guide = bundle.split('## Verify a built example\n', 1)[1].split('\n## ', 1)[0]
        commands = re.findall(r'```(?:sh|powershell)\n(.*?)\n```', guide, re.DOTALL)
        self.assertEqual(len(commands), 3)
        for command in commands:
            self.assertIn(command, snippets)
        examples = self.pages[(self.output / 'examples.html').resolve()]
        for page in (install, examples):
            content = ''.join(page.text)
            self.assertIn('mtile edit examples/hello', content)
            self.assertIn('mtile build examples/hello', content)
            self.assertIn('examples/hello/game.gba', content)
        for path, page in self.pages.items():
            if path.name == 'examples.html' or path.parent.name == 'guides':
                self.assertNotIn('go run ./cmd/mtile', ''.join(page.text), path.name)
                self.assertNotIn('Source code (zip)', ''.join(page.text), path.name)

    def test_nested_pages_distinguish_guide_home_from_title_screen(self):
        page = self.pages[(self.output / 'api/btn.html').resolve()]
        links_by_class = {name: attrs['href'] for tag, attrs in page.elements if tag == 'a'
                          for name in attrs.get('class', '').split()}
        self.assertEqual(links_by_class['brand'], '../index.html')
        self.assertEqual(links_by_class['title-return'], '../index.html')
        self.assertEqual(links_by_class['home-link'], '../guide.html')
        self.assertIn('<a href="../guide.html">Guide</a>',
                      (self.output / 'api/btn.html').read_text())
        for base in ('https://mtile.example/', 'https://mtile.example/projects/mtile/'):
            with self.subTest(base=base):
                self.assertEqual(urljoin(base + 'api/btn.html', links_by_class['home-link']),
                                 base + 'guide.html')
                self.assertEqual(urljoin(base + 'api/btn.html', links_by_class['brand']),
                                 base + 'index.html')

    def test_shared_logo_font_licenses_and_retired_artwork(self):
        self.assertEqual((self.output / 'assets/mtile-logo.png').read_bytes(),
                         (build.ROOT / 'cmd/editor/assets/mtile-header.png').read_bytes())
        for license_name in ('FONT-LICENSE.txt', 'RUBIK-OFL.txt'):
            with self.subTest(license=license_name):
                text = (self.output / 'assets' / license_name).read_text()
                self.assertIn('SIL OPEN FONT LICENSE', text)
        self.assertFalse((self.output / 'assets/box-art').exists())

    def test_search_index_contains_every_page_and_no_build_paths(self):
        text = (self.output / 'search-index.js').read_text()
        entries = json.loads(text.removeprefix('window.MTILE_SEARCH = ').removesuffix(';\n'))
        self.assertEqual({entry['url'] for entry in entries}, {p.relative_to(self.output).as_posix() for p in self.pages})
        self.assertNotIn(str(build.ROOT), text)
        self.assertTrue(any(entry['title'] == 'btn()' for entry in entries))

    def test_generated_guides_contain_current_template_and_cli(self):
        self.assertIn('btnp(5)', (self.output / 'guides/first-game.html').read_text())
        install = self.pages[(self.output / 'guides/install.html').resolve()]
        self.assertIn('mtile doctor', ''.join(install.text))
        self.assertTrue((self.output / 'assets/FONT-LICENSE.txt').is_file())
        self.assertTrue((self.output / '.nojekyll').is_file())


if __name__ == '__main__':
    unittest.main()
