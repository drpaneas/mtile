"""Keep the beginner chapter, its diagrams and old reference links usable."""
from pathlib import Path
import re
import unittest

import build
from test_site import Page


class EffectiveMtileTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.output = build.build(build.SITE / '_site')
        cls.path = cls.output / 'guides/performance.html'
        cls.text = cls.path.read_text()
        cls.page = Page(cls.path)

    def test_guide_is_a_decision_chapter_with_progressive_details(self):
        self.assertIn('<h1>Effective mTile</h1>', self.text)
        self.assertIn('prose effective-feature', self.text)
        self.assertEqual(self.page.h1, 1)
        for anchor in ('think-in-jobs-then-in-code',
                       'reuse-storage-when-the-same-slots-will-do',
                       'keep-one-number-about-one-thing',
                       'make-a-fixed-list-easy-to-recognise',
                       'state-a-fixed-rule-only-when-it-is-fixed',
                       'ask-for-the-picture-you-mean',
                       'keep-permanent-world-data-in-the-assets',
                       'expose-wrapped-reads-when-the-world-really-wraps',
                       'change-the-colours-that-actually-change',
                       'keep-a-fixed-label-whole', 'measure-a-decision',
                       'historical-reference'):
            self.assertIn(anchor, self.page.ids)
        self.assertGreaterEqual(self.text.count('class="effective-note"'), 9)
        self.assertIn('b99eb51', self.text)
        self.assertIn('implementation conditions', self.text)
        self.assertNotIn('{{PALETTE_', self.text)
        self.assertNotIn('{{HISTORICAL_', self.text)

    def test_diagrams_use_shared_scales_and_accessible_static_fallbacks(self):
        for anchor in ('effective-storage-lab', 'storage-count',
                       'storage-feedback', 'storage-description'):
            self.assertIn(anchor, self.page.ids)
        self.assertIn('aria-labelledby="storage-title storage-description"', self.text)
        self.assertIn('aria-live="polite"', self.text)
        self.assertIn('<noscript>', self.text)
        self.assertIn('384 bytes · one shared scale', self.text)
        self.assertIn('192 bytes', self.text)
        self.assertIn('64 bytes', self.text)
        self.assertIn('not an emulator', self.text)
        self.assertIn('not a duration scale', self.text)
        self.assertEqual(self.text.count('class="effective-changed"'), 2)
        for asset in ('effective-mtile.css', 'effective-mtile.js'):
            self.assertIn('../assets/' + asset, self.page.links)
            self.assertNotIn('assets/' + asset,
                             Page(self.output / 'guide.html').links)

    def test_old_reference_anchors_survive_without_filling_main_contents(self):
        for anchor in ('frame-stability-survey', 'the-budget',
                       'measured-costs-per-call', 'how-the-runtime-spends-memory',
                       'measuring-your-own-game'):
            self.assertIn(anchor, self.page.ids)
        self.assertIn('class="effective-history"', self.text)
        self.assertIn('Open the earlier timing tables', self.text)
        # The prior h2s become raw h3s inside the disclosure; the learning path
        # owns the main table of contents while deep links remain valid.
        self.assertIn('<h3 id="the-budget">The budget</h3>', self.text)

    def test_every_authoring_example_has_a_named_partner(self):
        source = (build.ROOT / 'docs/performance.md').read_text()
        names = re.findall(r'-- effective-example: ([a-z-]+)', source)
        self.assertEqual(len(names), 22)
        self.assertEqual(len(set(names)), 22)
        for name in names:
            partner = name.removesuffix('-before') + '-after' if name.endswith('-before') else name.removesuffix('-after') + '-before'
            self.assertIn(partner, names)


if __name__ == '__main__':
    unittest.main()
