#!/usr/bin/env python3
"""Build an experimental private bundle from owner's stock data and a built kernel.

Linux x86_64 only. Does not unlock, flash, reboot, or access any device.
The kernel must be built separately and contain the matching cube runtime init.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys

TOOLS = Path(__file__).resolve().parent
PORT = TOOLS.parent
REPO = PORT.parent.parent


def run(args, env=None, stdout=None):
    print('+', ' '.join(str(x) for x in args), flush=True)
    subprocess.run([str(x) for x in args], check=True, env=env, stdout=stdout)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--vendor', type=Path, required=True, help='owner vendor raw ext4 image or directory')
    p.add_argument('--oem', type=Path, required=True, help='owner OEM raw ext4 image or directory')
    p.add_argument('--kernel-out', type=Path, required=True, help='compatible prebuilt kernel output directory')
    p.add_argument('--donor-boot', type=Path, required=True, help='owner matching stock boot image')
    p.add_argument('--slot', choices=('a', 'b'), required=True, help='target inactive slot; must match boot image')
    p.add_argument('--work', type=Path, required=True, help='new private working directory; needs several GB free')
    a = p.parse_args()
    if sys.platform != 'linux' or platform.machine() not in ('x86_64', 'amd64'):
        p.error('requires x86_64 Linux (or a Linux VM/WSL)')
    if os.geteuid() != 0:
        p.error('run as root in a dedicated Linux build environment')
    for cmd in ('go', 'jadx', 'debugfs', 'mke2fs', 'bash'):
        if not shutil.which(cmd):
            p.error('missing required tool: ' + cmd)
    if not Path('/proc/sys/fs/binfmt_misc/qemu-arm').is_file():
        p.error('qemu-arm binfmt is not registered')
    work = a.work.resolve()
    if work.exists():
        p.error('work must be a new directory')
    for name in ('vendor', 'oem', 'kernel_out', 'donor_boot'):
        setattr(a, name, getattr(a, name).resolve(strict=True))
    kernel = a.kernel_out / 'arch/arm64/boot/Image.gz'
    dtb = a.kernel_out / 'arch/arm64/boot/dts/mediatek/mt8167s_ref.dtb'
    if not kernel.is_file() or not dtb.is_file() or not a.donor_boot.is_file():
        p.error('kernel Image.gz, DTB, or donor boot is missing')
    if REPO == work or REPO in work.parents:
        p.error('work must be outside the Git worktree')
    work.mkdir(parents=True, mode=0o700)
    env = {key: value for key, value in os.environ.items() if not key.startswith('CUBE_')}
    env.update({'GOOS': 'linux', 'GOARCH': 'arm', 'GOARM': '7', 'CGO_ENABLED': '0'})
    inputs, firmware, oem, audio = (work / n for n in ('inputs', 'firmware', 'oem', 'audio'))
    run([sys.executable, TOOLS / 'prepare_cube_inputs.py', '--output', inputs])
    run([sys.executable, TOOLS / 'extract_stock_firmware.py', a.vendor, firmware])
    run([sys.executable, TOOLS / 'extract_oem_apk.py', a.oem, oem])
    run([sys.executable, TOOLS / 'extract_audio_apk.py', oem / 'app.apk', audio])
    binaries = work / 'binaries'
    binaries.mkdir()
    daemon, btbridge, serial = (binaries / n for n in ('echod-cube-arm', 'btbridge-cube-arm', 'cube-serial-arm'))
    run(['go', '-C', REPO / 'echod', 'build', '-tags', 'cube', '-o', daemon, './cmd/echod'], env)
    run(['go', '-C', REPO, 'build', '-o', btbridge, './cmd/btbridge'], env)
    run(['go', '-C', PORT / 'serial', 'build', '-tags', 'cube_slot,cube_shared', '-o', serial, '.'], env)
    env.update({'CUBE_PORT': str(PORT), 'CUBE_REPO': str(REPO), 'CUBE_INPUTS': str(inputs),
                'CUBE_HELPERS': str(inputs / 'helpers'), 'CUBE_FIRMWARE': str(firmware),
                'CUBE_AUDIO': str(audio), 'CUBE_KERNEL_OUT': str(a.kernel_out),
                'CUBE_DAEMON': str(daemon), 'CUBE_BTBRIDGE': str(btbridge), 'CUBE_SERIAL': str(serial),
                'CUBE_OUTPUT': str(work / 'rootfs'), 'CUBE_TEST_AUDIO': '1', 'CUBE_BLUETOOTH': '1'})
    run(['bash', TOOLS / 'build-private-rootfs.sh'], env)
    boot = work / ('boot-' + a.slot + '.img')
    system = work / 'rootfs/system_b-experimental-DO-NOT-FLASH.img'
    run([sys.executable, TOOLS / 'make_diagnostic_boot.py', '--experimental-system-' + a.slot, a.donor_boot, kernel, dtb, boot])
    run([sys.executable, TOOLS / 'verify_diagnostic_boot.py', '--expect-system-' + a.slot, a.donor_boot, boot, kernel, dtb])
    report = work / 'root-report.json'
    with report.open('x', encoding='utf-8') as out:
        run([sys.executable, TOOLS / 'verify_private_rootfs.py', '--serial', serial,
             '--expect-test-audio', '--expect-test-bluetooth', '--expect-japanese-font', '--audio', audio,
             system, daemon, firmware], stdout=out)
    checks = json.loads(report.read_text(encoding='utf-8'))
    if not all(checks['checks'].values()):
        raise ValueError('rootfs verification failed')
    h = hashlib.sha256()
    with boot.open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            h.update(chunk)
    print('\nExperimental bundle built; no device was accessed.')
    print('Boot:', boot, 'SHA-256:', h.hexdigest())
    print('System:', system)
    print('Report:', report)
    print('Trial and install separately using install_cube_fastboot.py; see the installation guide.')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError) as exc:
        print('Build stopped:', exc, file=sys.stderr)
        raise SystemExit(1)
