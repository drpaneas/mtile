"""Source decoding retained specifically for the Celeste PSG conversion."""
import unittest

from pico_source import authored_music, parse_cart_text, sfx_length


class SourceAudioTests(unittest.TestCase):
    def test_source_notes_and_pattern_timing(self):
        source = "__sfx__\n00080000" + "39070" * 32 + "\n__music__\n00 00414141\n"
        cart = parse_cart_text(source)
        self.assertEqual(cart.sfx[0].notes[0].pitch, 57)
        self.assertEqual(sfx_length(cart.sfx[0]), 32)
        pattern = authored_music(cart)["patterns"][0]
        self.assertEqual(pattern["channels"], [0, -1, -1, -1])
        self.assertEqual(pattern["durationUnits"], 256)
        self.assertEqual(pattern["next"], -1)

    def test_malformed_source_is_rejected(self):
        with self.assertRaises(ValueError):
            parse_cart_text("__sfx__\ninvalid\n")


if __name__ == "__main__":
    unittest.main()
