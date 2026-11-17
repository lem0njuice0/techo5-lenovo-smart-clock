import struct
import tempfile
import unittest
from pathlib import Path
from verify_diagnostic_boot import HEADER, parse


class BootFormatTests(unittest.TestCase):
    def test_valid_and_truncated(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "boot.img"
            page = 2048
            header = HEADER.pack(b"ANDROID!", 4, 1, 3, 2, 0, 3, 4, page, 0, 0, b"", b"keep_initramfs", b"", b"")
            p.write_bytes(header.ljust(page, b"\0") + b"kern".ljust(page, b"\0") + b"ram".ljust(page, b"\0"))
            result = parse(p)
            self.assertEqual(result["kernel"], b"kern")
            self.assertEqual(result["ramdisk"], b"ram")
            p.write_bytes(p.read_bytes()[:page+2])
            with self.assertRaises(ValueError):
                parse(p)

    def test_invalid_header(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "bad"
            p.write_bytes(b"\0" * HEADER.size)
            with self.assertRaises(ValueError):
                parse(p)


if __name__ == "__main__":
    unittest.main()
