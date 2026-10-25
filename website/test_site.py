"""Check generated routes, API coverage, anchors, and portable static assets."""
from html.parser import HTMLParser
import json
import importlib.util
from pathlib import Path
import unittest
from urllib.parse import unquote, urljoin, urlsplit

import build


class Page(HTMLParser):
    def __init__(self, path):
        super().__init__()
        self.links, self.ids, self.h1, self.missing_alt = [], set(), 0, []
        self.elements = []
        self.feed(path.read_text())

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
        supported = build.registry() - {'sample'}
        indexed = {api['name'] for api in build.APIS}
        self.assertEqual(supported, indexed)
        for name in indexed:
            self.assertIn(name, build.classifications())
            self.assertTrue((self.output / 'api' / f'{name}.html').is_file())
        self.assertFalse((self.output / 'api/sample.html').exists())
        self.assertIn('does not support PCM', (self.output / 'api.html').read_text())
        self.assertIn('Mode 4 drawing', (self.output / 'api/rectfill.html').read_text())
        self.assertIn('Mode 0 with conditions', (self.output / 'api/spr.html').read_text())

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
        self.assertIn('mtile doctor', (self.output / 'guides/install.html').read_text())
        self.assertTrue((self.output / 'assets/FONT-LICENSE.txt').is_file())
        self.assertTrue((self.output / '.nojekyll').is_file())


if __name__ == '__main__':
    unittest.main()
