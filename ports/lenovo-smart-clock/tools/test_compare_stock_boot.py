import tempfile
import unittest
from pathlib import Path

from compare_stock_boot import compare
from verify_diagnostic_boot import HEADER


def image(kernel=b"kern", cmd=b"bootopt=test"):
    page = 2048
    header = HEADER.pack(b"ANDROID!", len(kernel), 1, 3, 2, 0, 3, 4, page, 0, 0, b"", cmd, b"", b"")
    return header.ljust(page, b"\0") + kernel.ljust(page, b"\0") + b"ram".ljust(page, b"\0")


class CompareStockBootTests(unittest.TestCase):
    def test_identical_padding_and_kernel_change(self):
        with tempfile.TemporaryDirectory() as tmp:
            a, b = Path(tmp) / "a", Path(tmp) / "b"
            a.write_bytes(image())
            b.write_bytes(image())
            self.assertTrue(all(compare(a, b)["checks"].values()))
            b.write_bytes(image() + b"\0" * 2048)
            result = compare(a, b)["checks"]
            self.assertFalse(result["whole_file_identical"])
            self.assertTrue(result["kernel_and_dtb_identical"])
            self.assertTrue(result["ramdisk_identical"])
            b.write_bytes(image(b"else"))
            self.assertFalse(compare(a, b)["checks"]["kernel_and_dtb_identical"])

    def test_invalid_image_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            a = Path(tmp) / "bad"
            a.write_bytes(b"bad")
            with self.assertRaises(ValueError):
                compare(a, a)


if __name__ == "__main__":
    unittest.main()
