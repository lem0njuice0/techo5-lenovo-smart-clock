#!/usr/bin/env python3
"""Build an experimental private bundle from the owner's stock data.

Linux x86_64 only. Does not unlock, flash, reboot, or access any device.
Builds a kernel from Lenovo's published source unless --kernel-out is provided.
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
    p.add_argument('--kernel-out', type=Path, help='optional compatible prebuilt kernel output directory')
    p.add_argument('--lenovo-archive', type=Path, help='optional local copy of the exact pinned Lenovo source archive')
    p.add_argument('--jobs', type=int, default=4, help='parallel kernel build jobs (default: 4)')
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
    for name in ('vendor', 'oem', 'donor_boot'):
        setattr(a, name, getattr(a, name).resolve(strict=True))
    if a.kernel_out:
        a.kernel_out = a.kernel_out.resolve(strict=True)
    if a.lenovo_archive:
        a.lenovo_archive = a.lenovo_archive.resolve(strict=True)
    if not a.donor_boot.is_file():
        p.error('donor boot is missing')
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
    if a.kernel_out is None:
        init = binaries / 'cube-boot-init-aarch64'
        init_env = env.copy()
        init_env.update({'GOARCH': 'arm64'})
        init_env.pop('GOARM', None)
        run(['go', '-C', PORT / 'rescue', 'build', '-tags', 'cube_boot', '-o', init, '.'], init_env)
        cmd = [sys.executable, TOOLS / 'build_cube_kernel_from_source.py', '--donor-boot', a.donor_boot,
               '--runtime-init', init, '--work', work / 'kernel-source', '--jobs', str(a.jobs)]
        if a.lenovo_archive:
            cmd += ['--lenovo-archive', a.lenovo_archive]
        run(cmd)
        a.kernel_out = work / 'kernel-source/kernel-build/out'
    kernel = a.kernel_out / 'arch/arm64/boot/Image.gz'
    dtb = a.kernel_out / 'arch/arm64/boot/dts/mediatek/mt8167s_ref.dtb'
    if not kernel.is_file() or not dtb.is_file():
        raise ValueError('kernel Image.gz or DTB is missing')
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
