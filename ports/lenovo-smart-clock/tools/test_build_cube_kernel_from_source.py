import gzip
from pathlib import Path
import tempfile
import unittest

from build_cube_kernel_from_source import decompress_stock_kernel, fetch


class KernelSourceTests(unittest.TestCase):
    def test_kernel_gzip_with_appended_dtb(self):
        payload = b'kernel IKCFG_ST fixture'
        self.assertEqual(decompress_stock_kernel(gzip.compress(payload) + b'fake-dtb'), payload)

    def test_rejects_wrong_or_truncated_kernel(self):
        with self.assertRaises(ValueError):
            decompress_stock_kernel(b'not-gzip')
        with self.assertRaises(ValueError):
            decompress_stock_kernel(gzip.compress(b'kernel')[:-3])

    def test_rejects_archive_with_wrong_hash(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / 'source'
            source.write_bytes(b'not Lenovo source')
            with self.assertRaisesRegex(ValueError, 'SHA-256 mismatch'):
                fetch('https://example.invalid/source', '0' * 64, root / 'out', source)


if __name__ == '__main__':
    unittest.main()
