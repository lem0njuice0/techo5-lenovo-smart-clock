import unittest
from extract_audio_tables import extract


def source(first="new short[]{0, 255}"):
    return f"short[][] registers = {{{first},new short[]{{0x7f,0x00}}}}; short[][] muteRegisters = {{new short[]{{3,2}}}}; short[][] AUDIO_MIC_COMMAND = {{new short[]{{1,4}}}};"


class ExtractAudioTests(unittest.TestCase):
    def test_order_and_values(self):
        tables = extract(source())
        self.assertEqual(tables["amp-init.regs"], [(0, 255), (127, 0)])
        self.assertEqual(tables["mic-init.regs"], [(1, 4)])

    def test_rejects_truncated_or_unexpected_input(self):
        for first in ("new short[]{256, 0}", "foo()", "new short[]{1}"):
            with self.subTest(first=first), self.assertRaises(ValueError):
                extract(source(first))
        with self.assertRaises(ValueError):
            extract(source() + "short[][] registers = {new short[]{0,0}};")


if __name__ == "__main__":
    unittest.main()
