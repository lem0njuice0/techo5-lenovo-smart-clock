#!/usr/bin/env python3
"""Experimental single entry point for preparing and installing a cube build.

Uses an owner's raw eMMC image or their extracted stock partitions. Does not
unlock the device, fetch a stock ROM, mark boot successful or provide recovery.
"""
import argparse
import os
from pathlib import Path
import subprocess
import sys
import tempfile

from inspect_gpt_backup import SECTOR, inspect
from verify_diagnostic_boot import parse as parse_boot

TOOLS = Path(__file__).resolve().parent
REPO = TOOLS.parent.parent.parent


def extract_stock(image, output, source_slot):
    """Copy only the three named partitions from a fully validated raw GPT image."""
    image = image.resolve(strict=True)
    output = output.resolve()
    if output.exists():
        raise ValueError('stock output already exists')
    layout = inspect(image)
    parts = {p['name']: p for p in layout['partitions']}
    needed = {name: name + '_' + source_slot for name in ('boot', 'vendor', 'oem')}
    for name in needed.values():
        if name not in parts:
            raise ValueError('stock image is missing partition: ' + name)
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.cube-stock-', dir=output.parent) as tmp:
        stage = Path(tmp) / 'partitions'
        stage.mkdir(mode=0o700)
        with image.open('rb') as source:
            for kind, name in needed.items():
                part = parts[name]
                source.seek(part['start_lba'] * SECTOR)
                remaining = part['size_bytes']
                target = stage / (kind + '.img')
                with target.open('xb') as out:
                    while remaining:
                        data = source.read(min(1024 * 1024, remaining))
                        if not data:
                            raise ValueError('truncated partition: ' + name)
                        out.write(data)
                        remaining -= len(data)
        parse_boot(stage / 'boot.img')
        for kind in ('vendor', 'oem'):
            with (stage / (kind + '.img')).open('rb') as f:
                f.seek(1080)
                if f.read(2) != b'\x53\xef':
                    raise ValueError(kind + ' is not a raw ext4 partition')
        stage.rename(output)
    return {name: output / (name + '.img') for name in needed}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=('build', 'inspect', 'trial', 'install'))
    p.add_argument('--work', type=Path, required=True, help='private pipeline directory')
    p.add_argument('--slot', choices=('a', 'b'), required=True, help='target inactive slot')
    p.add_argument('--serial', help='exact fastboot serial; required for inspect/trial/install')
    p.add_argument('--stock-image', type=Path, help='owner raw whole-eMMC user area image with complete GPT')
    p.add_argument('--source-slot', choices=('a', 'b'), default='a', help='stock image partition suffix')
    p.add_argument('--boot', type=Path, help='owner stock boot partition image')
    p.add_argument('--vendor', type=Path, help='owner vendor raw ext4 image or extracted directory')
    p.add_argument('--oem', type=Path, help='owner oem raw ext4 image or extracted directory')
    p.add_argument('--lenovo-archive', type=Path, help='optional local copy of pinned Lenovo source')
    p.add_argument('--kernel-out', type=Path, help='optional compatible prebuilt kernel output')
    p.add_argument('--jobs', type=int, default=4)
    p.add_argument('--trial-confirmed', action='store_true', help='confirm successful trial of this exact bundle')
    a = p.parse_args()
    work = a.work.resolve()
    if a.action == 'build':
        if sys.platform != 'linux':
            p.error('build requires Linux; on Windows use the WSL PowerShell wrapper')
        if os.geteuid() != 0:
            p.error('build requires root in a dedicated Linux build environment')
        if work.exists():
            p.error('work must be a new directory')
        if work == REPO or REPO in work.parents:
            p.error('work must be outside the Git worktree')
        if a.stock_image and any((a.boot, a.vendor, a.oem)):
            p.error('choose --stock-image or --boot/--vendor/--oem, not both')
        if not a.stock_image and not all((a.boot, a.vendor, a.oem)):
            p.error('provide --stock-image or all of --boot, --vendor and --oem')
        work.mkdir(parents=True, mode=0o700)
        if a.stock_image:
            paths = extract_stock(a.stock_image, work / 'stock', a.source_slot)
        else:
            paths = {name: getattr(a, name).resolve(strict=True) for name in ('boot', 'vendor', 'oem')}
        cmd = [sys.executable, str(TOOLS / 'build_cube_bundle.py'), '--vendor', str(paths['vendor']),
               '--oem', str(paths['oem']), '--donor-boot', str(paths['boot']), '--slot', a.slot,
               '--work', str(work / 'bundle'), '--jobs', str(a.jobs)]
        for name, value in (('--lenovo-archive', a.lenovo_archive), ('--kernel-out', a.kernel_out)):
            if value:
                cmd.extend([name, str(value.resolve(strict=True))])
    else:
        if not a.serial:
            p.error('--serial is required for fastboot actions')
        if a.action == 'install' and not a.trial_confirmed:
            p.error('install requires --trial-confirmed after testing this exact bundle')
        cmd = [sys.executable, str(TOOLS / 'cube_install.py'), '--work', str(work / 'bundle'),
               '--slot', a.slot, '--serial', a.serial, '--action', a.action]
        if a.trial_confirmed:
            cmd.append('--trial-confirmed')
    subprocess.run(cmd, check=True)
    if a.action == 'trial':
        print('Check display, touch, Wi-Fi and recovery; return to fastboot before install.')
    if a.action == 'install':
        print('Boot success is not automatically marked; see the installation guide.')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError) as exc:
        print('Pipeline stopped:', exc, file=sys.stderr)
        raise SystemExit(1)
