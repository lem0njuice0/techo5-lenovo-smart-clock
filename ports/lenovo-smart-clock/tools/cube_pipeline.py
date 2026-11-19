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
import time
import hashlib

from inspect_gpt_backup import SECTOR, inspect
from verify_diagnostic_boot import parse as parse_boot

TOOLS = Path(__file__).resolve().parent
REPO = TOOLS.parent.parent.parent


def fastboot_devices():
    result = subprocess.run(['fastboot', 'devices'], capture_output=True, text=True, timeout=10, check=True)
    return {line.split()[0] for line in result.stdout.splitlines()
            if len(line.split()) >= 2 and line.split()[1] == 'fastboot'}


def wait_fastboot(serial, present, timeout):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if (serial in fastboot_devices()) == present:
            return
        time.sleep(2)
    raise TimeoutError('Timed out waiting for the selected device to ' + ('enter' if present else 'leave') + ' fastboot')


def bundle_hashes(work, slot):
    paths = (work / 'bundle' / ('boot-' + slot + '.img'),
             work / 'bundle/rootfs/system_b-experimental-DO-NOT-FLASH.img',
             work / 'bundle/root-report.json')
    result = []
    for path in paths:
        h = hashlib.sha256()
        with path.open('rb') as source:
            for chunk in iter(lambda: source.read(1024 * 1024), b''):
                h.update(chunk)
        result.append(h.digest())
    return result


def guided_install(work, slot, serial, timeout):
    def install_action(action, confirmed=False):
        cmd = [sys.executable, str(TOOLS / 'cube_install.py'), '--work', str(work / 'bundle'),
               '--slot', slot, '--serial', serial, '--action', action]
        if confirmed:
            cmd.append('--trial-confirmed')
        subprocess.run(cmd, check=True)

    print('Connect the clock in fastboot (hold Volume Up while powering on).')
    print('時計を fastboot にしてください（音量アップを押しながら電源投入）。')
    print('Waiting for the specified serial / 指定した時計を待っています:', serial, flush=True)
    wait_fastboot(serial, True, timeout)
    install_action('inspect')
    hashes = bundle_hashes(work, slot)
    print('Trial writes system_' + slot + '. The inactive slot will be overwritten.')
    if input('Continue with trial? / 試験書き込みを続けますか？ [yes/no]: ').strip().lower() != 'yes':
        raise ValueError('trial cancelled')
    install_action('trial')
    print('Waiting for the device to leave fastboot / 時計が fastboot を離れるのを待っています...', flush=True)
    wait_fastboot(serial, False, timeout)
    print('Check display, touch, Wi-Fi and recovery on the temporary boot.')
    if input('Did the trial work? / 試験起動で画面・タッチ・接続は動きましたか？ [yes/no]: ').strip().lower() != 'yes':
        raise ValueError('trial was not confirmed; no boot partition was written')
    print('Return the same clock to fastboot: disconnect power, hold Volume Up, reconnect power.')
    print('同じ時計の電源を抜き、音量アップを押しながら電源を入れて fastboot に戻してください。', flush=True)
    wait_fastboot(serial, True, timeout)
    if hashes != bundle_hashes(work, slot):
        raise ValueError('bundle changed after the trial; refusing to install')
    print('Selected clock detected / 指定した時計を検出しました。')
    install_action('install', confirmed=True)
    subprocess.run(['fastboot', '-s', serial, 'reboot'], check=True, timeout=30)
    print('Reboot requested. Boot success is not marked automatically.')
    print('再起動を開始しました。起動成功の登録は別途必要です。')


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
    p.add_argument('action', choices=('build', 'inspect', 'trial', 'install', 'guided', 'build-guided'))
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
    p.add_argument('--wait-timeout', type=int, default=600, help='fastboot wait limit in seconds (1..3600)')
    a = p.parse_args()
    work = a.work.resolve()
    if not 1 <= a.wait_timeout <= 3600:
        p.error('--wait-timeout must be 1..3600 seconds')
    if a.action in ('guided', 'build-guided') and not a.serial:
        p.error('--serial is required for guided actions')
    if a.action == 'guided':
        guided_install(work, a.slot, a.serial, a.wait_timeout)
        return
    if a.action in ('build', 'build-guided'):
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
    if a.action == 'build-guided':
        guided_install(work, a.slot, a.serial, a.wait_timeout)
    if a.action == 'trial':
        print('Check display, touch, Wi-Fi and recovery; return to fastboot before install.')
    if a.action == 'install':
        print('Boot success is not automatically marked; see the installation guide.')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, EOFError, subprocess.SubprocessError) as exc:
        print('Pipeline stopped:', exc, file=sys.stderr)
        raise SystemExit(1)
