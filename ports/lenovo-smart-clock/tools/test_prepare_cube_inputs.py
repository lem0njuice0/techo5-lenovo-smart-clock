import hashlib
import io
from pathlib import Path
import tarfile
import tempfile
import unittest

import prepare_cube_inputs as prepare


class PrepareInputsTests(unittest.TestCase):
    def test_obtain_checks_local_archive_hash(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / 'source'
            source.write_bytes(b'public input')
            good = hashlib.sha256(b'public input').hexdigest()
            prepare.obtain('https://example.invalid/input', good, root / 'good', source)
            self.assertEqual((root / 'good').read_bytes(), b'public input')
            with self.assertRaisesRegex(ValueError, 'SHA-256 mismatch'):
                prepare.obtain('https://example.invalid/input', '0' * 64, root / 'bad', source)

    def test_archive_member_must_be_a_bounded_regular_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            archive_path = root / 'fixture.tar.gz'
            with tarfile.open(archive_path, 'w:gz') as archive:
                file = tarfile.TarInfo('model.tflite')
                file.size = 4
                archive.addfile(file, io.BytesIO(b'test'))
                link = tarfile.TarInfo('link')
                link.type = tarfile.SYMTYPE
                link.linkname = 'model.tflite'
                archive.addfile(link)
            with tarfile.open(archive_path, 'r:gz') as archive:
                prepare.copy_member(archive, archive.getmember('model.tflite'), root / 'model', 4)
                self.assertEqual((root / 'model').read_bytes(), b'test')
                with self.assertRaises(ValueError):
                    prepare.copy_member(archive, archive.getmember('model.tflite'), root / 'large', 3)
                with self.assertRaises(ValueError):
                    prepare.copy_member(archive, archive.getmember('link'), root / 'link', 10)


if __name__ == '__main__':
    unittest.main()
