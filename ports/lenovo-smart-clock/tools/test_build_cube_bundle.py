import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from build_cube_bundle import PREBUILT, verify_prebuilt


class PrebuiltTests(unittest.TestCase):
    def fixture(self, root):
        manifest = {}
        for name, machine in PREBUILT.items():
            data = bytearray(32)
            data[:6] = b'\x7fELF' + bytes((1 if machine == 40 else 2, 1))
            data[18:20] = machine.to_bytes(2, 'little')
            (root / name).write_bytes(data)
            manifest[name] = hashlib.sha256(data).hexdigest()
        (root / 'SHA256.json').write_text(json.dumps(manifest), encoding='utf-8')

    def test_valid_manifest_and_architectures(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.fixture(root)
            self.assertEqual(set(verify_prebuilt(root)), set(PREBUILT))

    def test_tampered_binary_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.fixture(root)
            with (root / 'echod-cube-arm').open('ab') as out:
                out.write(b'tampered')
            with self.assertRaisesRegex(ValueError, 'hash mismatch'):
                verify_prebuilt(root)

    def test_wrong_architecture_rejected_even_with_matching_hash(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.fixture(root)
            source, target = root / 'cube-boot-init-aarch64', root / 'cube-serial-arm'
            target.write_bytes(source.read_bytes())
            manifest = json.loads((root / 'SHA256.json').read_text())
            manifest[target.name] = hashlib.sha256(target.read_bytes()).hexdigest()
            (root / 'SHA256.json').write_text(json.dumps(manifest))
            with self.assertRaisesRegex(ValueError, 'wrong target architecture'):
                verify_prebuilt(root)


if __name__ == '__main__':
    unittest.main()
