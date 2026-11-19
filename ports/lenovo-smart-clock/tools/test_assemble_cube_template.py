import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

import assemble_cube_template as assembler
from audio_init_2019 import EXPECTED


@unittest.skipUnless(sys.platform == 'linux' and all(shutil.which(x) for x in ('mke2fs', 'debugfs', 'e2fsck')), 'Linux ext4 tools required')
class AssemblyTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='cube-test-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        rootfs = self.root / 'rootfs'
        (rootfs / 'etc/techo5-cube/audio').mkdir(parents=True)
        (rootfs / 'lib/firmware').mkdir(parents=True)
        (rootfs / 'lib/modules/cube').mkdir(parents=True)
        (rootfs / 'etc/techo5-cube-root').write_text('TECHO5-CUBE-TEMPLATE-V1\n')
        self.template = self.root / 'template.img'
        with self.template.open('wb') as f:
            f.truncate(32 * 1024 * 1024)
        subprocess.run(['mke2fs', '-q', '-t', 'ext4', '-F', '-d', str(rootfs), str(self.template)], check=True)
        self.firmware = self.root / 'firmware'
        self.firmware.mkdir()
        entries = []
        for name in assembler.FIRMWARE:
            content = ('fixture-' + name).encode()
            (self.firmware / name).write_bytes(content)
            entries.append({'name': name, 'sha256': hashlib.sha256(content).hexdigest()})
        (self.firmware / 'manifest.json').write_text(json.dumps({'files': entries}))
        self.audio = self.root / 'audio'
        self.audio.mkdir()
        entries = []
        for name, count in EXPECTED.items():
            content = b'0x00 0x00\n' * count
            (self.audio / name).write_bytes(content)
            entries.append({'name': name, 'pairs': count, 'sha256': hashlib.sha256(content).hexdigest()})
        (self.audio / 'manifest.json').write_text(json.dumps({'files': entries}))
        self.kernel_out = self.root / 'kernel'
        for relative in assembler.MODULES.values():
            path = self.kernel_out / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            header = bytearray(32)
            header[:6] = b'\x7fELF\x02\x01'
            header[18:20] = b'\xb7\x00'
            path.write_bytes(header)

    def test_assembly_keeps_template_unchanged(self):
        before = assembler.sha(self.template)
        output = self.root / 'result.img'
        result = assembler.assemble(self.template, before, self.firmware, self.audio, self.kernel_out, output)
        self.assertEqual(result, assembler.sha(output))
        self.assertEqual(before, assembler.sha(self.template))
        self.assertEqual(assembler.debugfs(self.template, 'cat /etc/techo5-cube-root').strip(), 'TECHO5-CUBE-TEMPLATE-V1')
        self.assertEqual(assembler.debugfs(output, 'cat /etc/techo5-cube-root').strip(), 'TECHO5-CUBE-ROOT-V1')
        self.assertEqual(assembler.debugfs(output, 'cat /lib/firmware/wifi.cfg'), 'fixture-wifi.cfg')

    def test_rejects_wrong_hash_without_output(self):
        output = self.root / 'result.img'
        with self.assertRaisesRegex(ValueError, 'SHA-256 mismatch'):
            assembler.assemble(self.template, '0' * 64, self.firmware, self.audio, self.kernel_out, output)
        self.assertFalse(output.exists())

    def test_rejects_modified_firmware_without_output(self):
        (self.firmware / 'wifi.cfg').write_bytes(b'modified')
        output = self.root / 'result.img'
        with self.assertRaisesRegex(ValueError, 'firmware hash mismatch'):
            assembler.assemble(self.template, assembler.sha(self.template), self.firmware, self.audio, self.kernel_out, output)
        self.assertFalse(output.exists())


if __name__ == '__main__':
    unittest.main()
