#!/usr/bin/env python3
"""Inspect, trial, or install an experimental bundle produced by build_cube_bundle.py.

Trial overwrites the inactive system partition; install writes its boot partition
and selects it. This tool does not unlock the device or mark the boot successful.
"""
import argparse
import hashlib
import os
from pathlib import Path
import subprocess
import sys


def text(en, ja):
    return ja if os.environ.get('CUBE_LANGUAGE') == 'ja' else en


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--work', type=Path, required=True, help='private bundle directory')
    p.add_argument('--serial', required=True, help='exact fastboot device serial')
    p.add_argument('--slot', choices=('a', 'b'), required=True, help='inactive target slot')
    p.add_argument('--action', choices=('inspect', 'trial', 'install'), default='inspect')
    trial = p.add_mutually_exclusive_group()
    trial.add_argument('--trial-confirmed', action='store_true', help='confirm that this exact image passed a device trial')
    trial.add_argument('--trial-unverified', action='store_true', help='proceed after an unverified trial')
    a = p.parse_args()
    if a.action == 'install' and not (a.trial_confirmed or a.trial_unverified):
        p.error('install requires --trial-confirmed or --trial-unverified')
    work = a.work.resolve(strict=True)
    boot = work / ('boot-' + a.slot + '.img')
    system = work / 'rootfs/system_b-experimental-DO-NOT-FLASH.img'
    report = work / 'root-report.json'
    h = hashlib.sha256()
    with boot.open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            h.update(chunk)
    cmd = [sys.executable, str(Path(__file__).with_name('install_cube_fastboot.py')),
           '--serial', a.serial, '--slot', a.slot, '--boot', str(boot), '--boot-sha256', h.hexdigest(),
           '--system', str(system), '--root-report', str(report), '--action', a.action]
    if a.trial_confirmed:
        cmd.append('--trial-confirmed')
    if a.trial_unverified:
        cmd.append('--trial-unverified')
    if a.action == 'trial':
        print(text('WARNING: this writes the inactive system partition before temporary boot.', '注意: 一時起動の前に、非アクティブ側の system に書き込みます。'), flush=True)
    elif a.action == 'install':
        print(text('WARNING: this writes the inactive boot partition and changes the active slot.', '注意: 非アクティブ側の boot に書き込み、起動スロットを変更します。'), flush=True)
    subprocess.run(cmd, check=True)


if __name__ == '__main__':
    try:
        main()
    except (OSError, subprocess.SubprocessError) as exc:
        print(text('Installer stopped:', 'インストールを停止しました:'), exc, file=sys.stderr)
        raise SystemExit(1)
