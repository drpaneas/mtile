import importlib.util, json, unittest
from pathlib import Path
module_path=Path(__file__).with_name('probe.py')
spec=importlib.util.spec_from_file_location('typography_probe',module_path)
probe=importlib.util.module_from_spec(spec);spec.loader.exec_module(probe)

class FontProbeTests(unittest.TestCase):
    def test_exact_same_repertoire_and_packed_bit_limits(self):
        first=None
        for name,w,h in [('5x7',5,7),('6x10',6,10)]:
            f=json.loads(Path(__file__).with_name(name+'.json').read_text())
            keys=[g['codepoint'] for g in f['glyphs']]
            self.assertEqual(len(keys),174);self.assertEqual(len(set(keys)),174)
            self.assertEqual(keys,sorted(keys))
            if first is None:first=keys
            self.assertEqual(keys,first)
            for cp in range(32,127):self.assertIn(cp,keys)
            self.assertIn(0x1e9e,keys)
            for glyph in f['glyphs']:
                self.assertEqual(len(glyph['columns']),w)
                self.assertTrue(all(0<=c<(1<<h) for c in glyph['columns']))
            self.assertEqual(f['researchSource']['originalGlyphs'],['U+1E9E'])
    def test_declaration_keeps_ten_bit_columns_without_truncation(self):
        f=json.loads(Path(__file__).with_name('6x10.json').read_text())
        self.assertTrue(any(c>255 for g in f['glyphs'] for c in g['columns']))
        declaration=probe.declaration(f)
        self.assertIn('[6]uint16',declaration)
        self.assertNotIn('[6]byte',declaration)
        self.assertEqual(declaration.count('var projectFontROM ='),1)
        self.assertIn('TestTypographyBoundsAndAPI',probe.host_tests(f))
    def test_source_hashes_are_pinned(self):
        for name in ['5x7','6x10']:
            f=json.loads(Path(__file__).with_name(name+'.json').read_text())
            self.assertEqual(f['researchSource']['sha256'],probe.SOURCES[name][0])
            self.assertIn('Public domain',f['researchSource']['license'])
if __name__=='__main__':unittest.main()
