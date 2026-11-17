import struct
import json
import subprocess
import sys
import tempfile
import unittest
import uuid
from pathlib import Path

from inspect_gpt_backup import HEADER, SECTOR, crc, inspect


def fixture(overlap=False):
    sectors, count, step = 128, 8, 128
    data = bytearray(sectors * SECTOR)
    data[510:512] = b'\x55\xaa'
    data[450] = 0xee
    table = bytearray(count * step)
    for i, (name, start, end) in enumerate([('boot_a', 10, 19), ('system_b', 15 if overlap else 20, 39)]):
        off = i * step
        table[off:off+16] = uuid.UUID(int=10).bytes_le
        table[off+16:off+32] = uuid.UUID(int=i+1).bytes_le
        struct.pack_into('<QQQ', table, off+32, start, end, 0)
        name_bytes = name.encode('utf-16le')
        table[off+56:off+56+len(name_bytes)] = name_bytes
    disk = uuid.UUID(int=99).bytes_le
    for here, other, table_lba in [(1, 127, 2), (127, 1, 125)]:
        header = bytearray(HEADER.pack(b'EFI PART', 0x10000, HEADER.size, 0, 0, here, other, 4, 124, disk, table_lba, count, step, crc(table)))
        struct.pack_into('<I', header, 16, crc(header))
        data[here*SECTOR:here*SECTOR+len(header)] = header
        data[table_lba*SECTOR:table_lba*SECTOR+len(table)] = table
    return data


class GPTTests(unittest.TestCase):
    def test_valid_and_hash(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'image'
            path.write_bytes(fixture())
            got = inspect(path, True)
            self.assertEqual([p['name'] for p in got['partitions']], ['boot_a', 'system_b'])
            self.assertTrue(got['required_names_present']['boot_a'])
            self.assertFalse(got['required_names_present']['boot_b'])
            self.assertEqual(len(got['sha256']), 64)

    def test_detects_corruption_and_overlap(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'image'
            data = fixture()
            data[2*SECTOR] ^= 1
            path.write_bytes(data)
            with self.assertRaisesRegex(ValueError, 'CRC'):
                inspect(path)
            path.write_bytes(fixture(overlap=True))
            with self.assertRaisesRegex(ValueError, 'overlapping'):
                inspect(path)
            path.write_bytes(fixture()[:-SECTOR])
            with self.assertRaises(ValueError):
                inspect(path)

    def test_candidate_sizes_do_not_imply_installability(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path, candidate = root / 'disk', root / 'rootfs'
            path.write_bytes(fixture())
            candidate.write_bytes(b'x' * (20 * SECTOR + 1))
            script = Path(__file__).with_name('inspect_gpt_backup.py')
            run = subprocess.run([sys.executable, str(script), str(path), '--candidate-root', str(candidate)], capture_output=True)
            self.assertEqual(run.returncode, 1)
            result = json.loads(run.stdout)
            self.assertFalse(result['candidate_sizes']['system_b']['fits'])
            self.assertFalse(result['required_names_present']['boot_b'])


if __name__ == '__main__':
    unittest.main()
