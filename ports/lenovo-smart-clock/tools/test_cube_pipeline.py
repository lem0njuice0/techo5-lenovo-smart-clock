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
    def test_device_wait_ignores_other_serials_and_times_out(self):
        with mock.patch.object(pipeline, 'fastboot_devices', return_value={'other'}), \
             mock.patch.object(pipeline.time, 'monotonic', side_effect=[0, 0, 2]), \
             mock.patch.object(pipeline.time, 'sleep'):
            with self.assertRaises(TimeoutError):
                pipeline.wait_fastboot('clock', True, 1)

    def test_guided_install_waits_for_same_device_without_claiming_trial_verified(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'bundle/rootfs').mkdir(parents=True)
            (root / 'bundle/boot-a.img').write_bytes(b'boot')
            (root / 'bundle/rootfs/system_b-experimental-DO-NOT-FLASH.img').write_bytes(b'system')
            (root / 'bundle/root-report.json').write_bytes(b'report')
            with mock.patch.object(pipeline, 'wait_fastboot') as wait, \
                 mock.patch.object(pipeline.subprocess, 'run') as run, \
                 mock.patch('builtins.input', return_value='yes') as prompt:
                pipeline.guided_install(root, 'a', 'clock', 60)
            self.assertEqual([x.args[:2] for x in wait.call_args_list],
                             [('clock', True), ('clock', False), ('clock', True)])
            self.assertEqual([x.args[0][x.args[0].index('--action') + 1] for x in run.call_args_list[:3]],
                             ['inspect', 'trial', 'install'])
            self.assertIn('--trial-unverified', run.call_args_list[2].args[0])
            self.assertNotIn('--trial-confirmed', run.call_args_list[2].args[0])
            self.assertEqual(prompt.call_count, 1)
            self.assertEqual(run.call_args_list[3].args[0], ['fastboot', '-s', 'clock', 'reboot'])

    def test_guided_install_stops_if_trial_write_cancelled(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'bundle/rootfs').mkdir(parents=True)
            (root / 'bundle/boot-a.img').write_bytes(b'boot')
            (root / 'bundle/rootfs/system_b-experimental-DO-NOT-FLASH.img').write_bytes(b'system')
            (root / 'bundle/root-report.json').write_bytes(b'report')
            with mock.patch.object(pipeline, 'wait_fastboot'), \
                 mock.patch.object(pipeline.subprocess, 'run') as run, \
                 mock.patch('builtins.input', return_value='no'):
                with self.assertRaisesRegex(ValueError, 'trial cancelled'):
                    pipeline.guided_install(root, 'a', 'clock', 60)
            self.assertEqual(run.call_count, 1)

    def test_guided_install_rejects_changed_bundle(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'bundle/rootfs').mkdir(parents=True)
            boot = root / 'bundle/boot-a.img'
            boot.write_bytes(b'boot')
            (root / 'bundle/rootfs/system_b-experimental-DO-NOT-FLASH.img').write_bytes(b'system')
            (root / 'bundle/root-report.json').write_bytes(b'report')
            def wait(_serial, present, _timeout):
                if present and run.call_count == 2:
                    boot.write_bytes(b'changed')
            with mock.patch.object(pipeline, 'wait_fastboot', side_effect=wait), \
                 mock.patch.object(pipeline.subprocess, 'run') as run, \
                 mock.patch('builtins.input', return_value='yes'):
                with self.assertRaisesRegex(ValueError, 'bundle changed'):
                    pipeline.guided_install(root, 'a', 'clock', 60)
            self.assertEqual(run.call_count, 2)

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
