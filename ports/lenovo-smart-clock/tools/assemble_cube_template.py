#!/usr/bin/env python3
"""Assemble a private rootfs from a trusted cube template and owner's extracted inputs.

Requires Linux and debugfs/e2fsck. This does not download, mount or flash a device.
The caller must independently authenticate the template and provide its SHA-256.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

FIRMWARE = ('EEPROM_MT7668.bin', 'TxPwrLimit_MT76x8.dat', 'WIFI_RAM_CODE2_SDIO_MT7668.bin',
            'WIFI_RAM_CODE_MT7668.bin', 'mt7668_patch_e2_hdr.bin', 'wifi.cfg')
AUDIO = ('amp-init.regs', 'amp-prelude.regs', 'mic-init.regs', 'manifest.json')
MODULES = {
    'wlan_drv_gen4_mt7668.ko': 'drivers/misc/mediatek/connectivity/wlan/gen4-mt7668/wlan_drv_gen4_mt7668.ko',
    'btmtksdio.ko': 'drivers/misc/mediatek/connectivity/bt/mt76xx/sdio/btmtksdio.ko',
}


def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def debugfs(image, command, write=False):
    result = subprocess.run(['debugfs'] + (['-w'] if write else []) + ['-R', command, str(image)],
                            capture_output=True, text=True, check=True, env={**os.environ, 'LC_ALL': 'C'})
    errors = '\n'.join(line for line in result.stderr.splitlines() if not line.startswith('debugfs '))
    if errors.strip():
        raise ValueError('debugfs failed: ' + errors.strip())
    return result.stdout


def require_absent(image, path):
    result = subprocess.run(['debugfs', '-R', 'stat ' + path, str(image)], capture_output=True, text=True, check=True,
                            env={**os.environ, 'LC_ALL': 'C'})
    errors = '\n'.join(line for line in result.stderr.splitlines() if not line.startswith('debugfs ')).strip()
    if result.stdout.strip() or 'File not found by ext2_lookup' not in errors:
        raise ValueError('template target is not absent: ' + path)


def validate_firmware(directory):
    entries = json.loads((directory / 'manifest.json').read_text(encoding='utf-8'))['files']
    by_name = {entry['name']: entry['sha256'] for entry in entries}
    if len(by_name) != len(entries) or not set(FIRMWARE).issubset(by_name):
        raise ValueError('invalid firmware manifest')
    if set(by_name) - set(FIRMWARE) - {'rgx.fw.signed.22.40.54.30'}:
        raise ValueError('unexpected firmware manifest entry')
    for name in FIRMWARE:
        path = directory / name
        if path.is_symlink() or not path.is_file() or sha(path) != by_name[name]:
            raise ValueError('firmware hash mismatch: ' + name)


def assemble(template, expected, firmware, audio, kernel_out, output):
    template = template.resolve(strict=True)
    firmware = firmware.resolve(strict=True)
    audio = audio.resolve(strict=True)
    kernel_out = kernel_out.resolve(strict=True)
    output = output.resolve()
    if output.exists() or not re.fullmatch(r'[0-9a-fA-F]{64}', expected):
        raise ValueError('output must be new and expected SHA-256 must be 64 hexadecimal digits')
    if sha(template) != expected.lower():
        raise ValueError('template SHA-256 mismatch')
    validate_firmware(firmware)
    for name in AUDIO:
        path = audio / name
        if path.is_symlink() or not path.is_file():
            raise ValueError('missing or unsafe audio input: ' + name)
    for name, relative in MODULES.items():
        path = kernel_out / relative
        if path.is_symlink() or not path.is_file():
            raise ValueError('missing or unsafe kernel module: ' + name)
        with path.open('rb') as module:
            header = module.read(20)
        if len(header) < 20 or header[:6] != b'\x7fELF\x02\x01' or header[18:20] != b'\xb7\x00':
            raise ValueError('kernel module must be little-endian ARM64 ELF: ' + name)
    subprocess.run([sys.executable, str(Path(__file__).with_name('audio_init_2019.py')), str(audio)],
                   check=True, stdout=subprocess.DEVNULL)
    if debugfs(template, 'cat /etc/techo5-cube-root').strip() != 'TECHO5-CUBE-TEMPLATE-V1':
        raise ValueError('not an unassembled cube template')
    for name in FIRMWARE + ('FT6336U_Holitech.bin', 'FT6336U_KD.bin'):
        require_absent(template, '/lib/firmware/' + name)
    for name in AUDIO:
        require_absent(template, '/etc/techo5-cube/audio/' + name)
    for name in MODULES:
        require_absent(template, '/lib/modules/cube/' + name)
    # Use a short safe temporary directory for debugfs command arguments.
    with tempfile.TemporaryDirectory(prefix='cube-assemble-') as temporary:
        stage = Path(temporary)
        output.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='.cube-output-', dir=output.parent) as destination:
            image = Path(destination) / 'system.img'
            shutil.copyfile(template, image)
            pairs = [(firmware / name, '/lib/firmware/' + name, '0100644') for name in FIRMWARE]
            pairs += [(audio / name, '/etc/techo5-cube/audio/' + name, '0100600') for name in AUDIO]
            pairs += [(kernel_out / relative, '/lib/modules/cube/' + name, '0100644') for name, relative in MODULES.items()]
            for index, (source, target, mode) in enumerate(pairs):
                item = stage / ('input' + str(index))
                shutil.copyfile(source, item)
                debugfs(image, 'write ' + str(item) + ' ' + target, True)
                debugfs(image, 'set_inode_field ' + target + ' mode ' + mode, True)
                dumped = stage / ('check' + str(index))
                debugfs(image, 'dump ' + target + ' ' + str(dumped))
                if sha(item) != sha(dumped):
                    raise ValueError('written file failed verification: ' + target)
            marker = stage / 'marker'
            marker.write_text('TECHO5-CUBE-ROOT-V1\n', encoding='ascii')
            debugfs(image, 'rm /etc/techo5-cube-root', True)
            debugfs(image, 'write ' + str(marker) + ' /etc/techo5-cube-root', True)
            debugfs(image, 'set_inode_field /etc/techo5-cube-root mode 0100644', True)
            if debugfs(image, 'cat /etc/techo5-cube-root').strip() != 'TECHO5-CUBE-ROOT-V1':
                raise ValueError('assembled root marker did not verify')
            subprocess.run(['e2fsck', '-fn', str(image)], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            # link creates the destination exclusively; never replace an existing image.
            os.link(image, output)
    return sha(output)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--template', type=Path, required=True)
    p.add_argument('--template-sha256', required=True, help='expected hash from a trusted release')
    p.add_argument('--firmware', type=Path, required=True)
    p.add_argument('--audio', type=Path, required=True)
    p.add_argument('--kernel-out', type=Path, required=True, help='matching locally built kernel output')
    p.add_argument('--output', type=Path, required=True, help='new private output image')
    a = p.parse_args()
    if sys.platform != 'linux':
        p.error('Linux or WSL is required')
    print('Private rootfs SHA-256:', assemble(a.template, a.template_sha256, a.firmware, a.audio, a.kernel_out, a.output))
    print('Run verify_private_rootfs.py before any device trial; this is not hardware validation.')


if __name__ == '__main__':
    try:
        main()
    except (OSError, KeyError, ValueError, subprocess.SubprocessError) as exc:
        print('Assembly stopped:', exc, file=sys.stderr)
        raise SystemExit(1)
