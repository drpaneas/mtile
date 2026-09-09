import tempfile
from pathlib import Path
import unittest

from check_links import check_file, headings


class LinksTest(unittest.TestCase):
    def test_missing_file_and_heading_are_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            (root / "target.md").write_text("# Valid heading\n")
            guide = root / "README.md"
            guide.write_text("[file](missing.md)\n[anchor](target.md#absent)\n")
            errors = check_file(guide, root)
            self.assertEqual(len(errors), 2)
            self.assertIn("does not exist", errors[0])
            self.assertIn("has no matching heading", errors[1])

    def test_local_images_fragments_and_code_examples(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            (root / "image.png").touch()
            guide = root / "README.md"
            guide.write_text("# Hello, GBA!\n[local](#hello-gba)\n![demo](image.png)\n"
                             "[external](https://example.com)\n```md\n[example](missing.md)\n```\n")
            self.assertEqual(check_file(guide, root), [])

    def test_duplicate_headings_and_escape(self):
        self.assertEqual(headings("# Repeat\n# Repeat\n"), {"repeat", "repeat-1"})
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            guide = root / "README.md"
            guide.write_text("[outside](../elsewhere.md)\n")
            self.assertIn("leaves the repository", check_file(guide, root)[0])


if __name__ == "__main__":
    unittest.main()
