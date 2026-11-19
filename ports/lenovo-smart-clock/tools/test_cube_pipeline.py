from pathlib import Path
import struct
import tempfile
import unittest
import uuid
from unittest import mock

import cube_pipeline as pipeline
from inspect_gpt_backup import HEADER, SECTOR, crc


def stock_gpt_fixture():
    sectors, count, step = 128, 8, 128
    data = bytearray(sectors * SECTOR)
    data[510:512] = b'\x55\xaa'
    data[450] = 0xee
    table = bytearray(count * step)
    for i, (name, start) in enumerate([('boot_a', 10), ('vendor_a', 20), ('oem_a', 30)]):
        off = i * step
        table[off:off + 16] = uuid.UUID(int=10).bytes_le
        table[off + 16:off + 32] = uuid.UUID(int=i + 1).bytes_le
        struct.pack_into('<QQQ', table, off + 32, start, start + 7, 0)
        encoded = name.encode('utf-16le')
        table[off + 56:off + 56 + len(encoded)] = encoded
        if name != 'boot_a':
            data[start * SECTOR + 1080:start * SECTOR + 1082] = b'\x53\xef'
    disk = uuid.UUID(int=99).bytes_le
    for here, other, table_lba in [(1, 127, 2), (127, 1, 125)]:
        header = bytearray(HEADER.pack(b'EFI PART', 0x10000, HEADER.size, 0, 0, here, other,
                                       4, 124, disk, table_lba, count, step, crc(table)))
        struct.pack_into('<I', header, 16, crc(header))
        data[here * SECTOR:here * SECTOR + len(header)] = header
        data[table_lba * SECTOR:table_lba * SECTOR + len(table)] = table
    return data


class PipelineTests(unittest.TestCase):
    def test_extracts_validated_gpt_image(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            image = root / 'disk.img'
            image.write_bytes(stock_gpt_fixture())
            with mock.patch.object(pipeline, 'parse_boot'):
                result = pipeline.extract_stock(image, root / 'stock', 'a')
            self.assertEqual(result['boot'].stat().st_size, 4096)
            self.assertEqual(result['oem'].read_bytes()[1080:1082], b'\x53\xef')

    def test_extract_only_requested_slot(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            image = root / 'disk.img'
            data = bytearray(512 * 40)
            for first in (8, 16):
                data[first * 512 + 1080:first * 512 + 1082] = b'\x53\xef'
            data[0:4] = b'BOOT'
            image.write_bytes(data)
            parts = [{'name': n, 'start_lba': start, 'size_bytes': 4096}
                     for n, start in [('boot_b', 0), ('vendor_b', 8), ('oem_b', 16)]]
            with mock.patch.object(pipeline, 'inspect', return_value={'partitions': parts}), \
                 mock.patch.object(pipeline, 'parse_boot'):
                result = pipeline.extract_stock(image, root / 'stock', 'b')
            self.assertEqual(result['boot'].read_bytes()[:4], b'BOOT')
            self.assertEqual(result['vendor'].stat().st_size, 4096)
            self.assertEqual(set(p.name for p in (root / 'stock').iterdir()), {'boot.img', 'vendor.img', 'oem.img'})

    def test_missing_partition_fails_before_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            image = root / 'disk.img'
            image.write_bytes(b'fixture')
            with mock.patch.object(pipeline, 'inspect', return_value={'partitions': []}):
                with self.assertRaisesRegex(ValueError, 'missing partition'):
                    pipeline.extract_stock(image, root / 'stock', 'a')
            self.assertFalse((root / 'stock').exists())

    def test_rejects_invalid_ext4_and_leaves_no_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            image = root / 'disk.img'
            image.write_bytes(bytes(512 * 24))
            parts = [{'name': n, 'start_lba': start, 'size_bytes': 4096}
                     for n, start in [('boot_a', 0), ('vendor_a', 8), ('oem_a', 16)]]
            with mock.patch.object(pipeline, 'inspect', return_value={'partitions': parts}), \
                 mock.patch.object(pipeline, 'parse_boot'):
                with self.assertRaisesRegex(ValueError, 'not a raw ext4'):
                    pipeline.extract_stock(image, root / 'stock', 'a')
            self.assertFalse((root / 'stock').exists())


if __name__ == '__main__':
    unittest.main()
