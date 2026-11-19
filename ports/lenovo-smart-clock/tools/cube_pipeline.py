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
LANGUAGE = 'en'

MESSAGES = {
    'fastboot': ('Connect the clock in fastboot: unplug power, keep USB connected, then hold Volume Up for 20–30 seconds while reconnecting power. The screen may stay dark.',
                 '時計を fastboot にしてください。USB は接続したまま電源を抜き、音量アップを押しながら電源を入れて 20〜30 秒押し続けます。画面は暗いままの場合があります。'),
    'waiting': ('Waiting for the specified device:', '指定した時計を待っています:'),
    'overwrite': ('The trial will overwrite the inactive system partition:', '試験起動では、非アクティブ側の system を上書きします:'),
    'confirm': ('Continue with trial? [yes/no]: ', '試験書き込みを続けますか？ [yes/no]: '),
    'leaving': ('Waiting for the clock to leave fastboot...', '時計が fastboot を離れるのを待っています...'),
    'return': ('Return the same clock to fastboot: unplug power, keep USB connected, then hold Volume Up for 20–30 seconds while reconnecting power.',
               '同じ時計の USB は接続したまま電源を抜き、音量アップを押しながら電源を入れて 20〜30 秒押し続け、fastboot に戻してください。'),
    'detected': ('Selected clock detected.', '指定した時計を検出しました。'),
    'unverified': ('Trial features were not verified.', '試験起動の機能確認は行っていません。'),
    'reboot': ('Reboot requested. Boot success is not marked automatically.', '再起動を開始しました。起動成功の登録は別途必要です。'),
    'cancel': ('Trial cancelled.', '試験書き込みを中止しました。'),
    'changed': ('Bundle changed after the trial; refusing to install.', '試験後にイメージが変更されたため、導入を中止しました。'),
    'stopped': ('Pipeline stopped:', '処理を停止しました:'),
    'retry': ('Choose 1 (English) or 2 (日本語).', '1（English）または 2（日本語）を選んでください。'),
}


def message(key):
    return MESSAGES[key][1 if LANGUAGE == 'ja' else 0]


def localized(en, ja):
    return ja if LANGUAGE == 'ja' else en


def choose_language(argv, interactive):
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument('--language', choices=('en', 'ja'))
    args, _ = parser.parse_known_args(argv)
    if args.language:
        return args.language
    if not interactive or '--help' in argv or '-h' in argv:
        return 'en'
    while True:
        answer = input('Language / 言語: [1] English  [2] 日本語: ').strip().lower()
        if answer in ('1', 'en', 'english'):
            return 'en'
        if answer in ('2', 'ja', '日本語'):
            return 'ja'
        print(message('retry'))


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
    raise TimeoutError(localized('Timed out waiting for the selected device to ' + ('enter' if present else 'leave') + ' fastboot',
                                ('指定した時計が fastboot に接続されませんでした。' if present else '指定した時計が fastboot を離れませんでした。') + '時間切れのため停止します。'))


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
    def install_action(action, unverified=False):
        cmd = [sys.executable, str(TOOLS / 'cube_install.py'), '--work', str(work / 'bundle'),
               '--slot', slot, '--serial', serial, '--action', action]
        if unverified:
            cmd.append('--trial-unverified')
        subprocess.run(cmd, check=True)

    print(message('fastboot'))
    print(message('waiting'), serial, flush=True)
    wait_fastboot(serial, True, timeout)
    install_action('inspect')
    hashes = bundle_hashes(work, slot)
    print(message('overwrite'), 'system_' + slot)
    if input(message('confirm')).strip().lower() != 'yes':
        raise ValueError(message('cancel'))
    install_action('trial')
    print(message('leaving'), flush=True)
    wait_fastboot(serial, False, timeout)
    print(message('return'), flush=True)
    wait_fastboot(serial, True, timeout)
    if hashes != bundle_hashes(work, slot):
        raise ValueError(message('changed'))
    print(message('detected'))
    print(message('unverified'))
    install_action('install', unverified=True)
    subprocess.run(['fastboot', '-s', serial, 'reboot'], check=True, timeout=30)
    print(message('reboot'))


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
    global LANGUAGE
    LANGUAGE = choose_language(sys.argv[1:], sys.stdin.isatty())
    os.environ['CUBE_LANGUAGE'] = LANGUAGE
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--language', choices=('en', 'ja'), help='prompt language; defaults to English in non-interactive use')
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
    p.add_argument('--prebuilt-dir', type=Path, help='optional verified prebuilt cube binaries directory')
    p.add_argument('--template', type=Path, help='trusted cube rootfs template')
    p.add_argument('--template-sha256', help='expected template SHA-256 from a trusted source')
    p.add_argument('--jobs', type=int, default=4)
    p.add_argument('--trial-confirmed', action='store_true', help='confirm successful trial of this exact bundle')
    p.add_argument('--wait-timeout', type=int, default=600, help='fastboot wait limit in seconds (1..3600)')
    a = p.parse_args()
    work = a.work.resolve()
    if not 1 <= a.wait_timeout <= 3600:
        p.error(localized('--wait-timeout must be 1..3600 seconds', '--wait-timeout は 1〜3600 秒で指定してください'))
    if a.action in ('guided', 'build-guided') and not a.serial:
        p.error(localized('--serial is required for guided actions', '対話式操作には --serial が必要です'))
    if a.action == 'guided':
        guided_install(work, a.slot, a.serial, a.wait_timeout)
        return
    if a.action in ('build', 'build-guided'):
        if sys.platform != 'linux':
            p.error(localized('build requires Linux; on Windows use the WSL PowerShell wrapper', 'ビルドには Linux が必要です。Windows では WSL 用 PowerShell ラッパーを使ってください'))
        if os.geteuid() != 0:
            p.error(localized('build requires root in a dedicated Linux build environment', 'ビルドには専用の Linux 環境と root 権限が必要です'))
        if work.exists():
            p.error(localized('work must be a new directory', '作業先にはまだ存在しないディレクトリを指定してください'))
        if work == REPO or REPO in work.parents:
            p.error(localized('work must be outside the Git worktree', '作業先は Git の管理フォルダーの外に指定してください'))
        if a.stock_image and any((a.boot, a.vendor, a.oem)):
            p.error(localized('choose --stock-image or --boot/--vendor/--oem, not both', '--stock-image と --boot/--vendor/--oem は同時に指定できません'))
        if not a.stock_image and not all((a.boot, a.vendor, a.oem)):
            p.error(localized('provide --stock-image or all of --boot, --vendor and --oem', '--stock-image または --boot・--vendor・--oem のすべてを指定してください'))
        if bool(a.template) != bool(a.template_sha256) or (a.template and not a.prebuilt_dir):
            p.error(localized('template use requires --template, --template-sha256 and --prebuilt-dir',
                              'テンプレートには --template・--template-sha256・--prebuilt-dir が必要です'))
        work.mkdir(parents=True, mode=0o700)
        if a.stock_image:
            paths = extract_stock(a.stock_image, work / 'stock', a.source_slot)
        else:
            paths = {name: getattr(a, name).resolve(strict=True) for name in ('boot', 'vendor', 'oem')}
        cmd = [sys.executable, str(TOOLS / 'build_cube_bundle.py'), '--vendor', str(paths['vendor']),
               '--oem', str(paths['oem']), '--donor-boot', str(paths['boot']), '--slot', a.slot,
               '--work', str(work / 'bundle'), '--jobs', str(a.jobs)]
        for name, value in (('--lenovo-archive', a.lenovo_archive), ('--kernel-out', a.kernel_out), ('--prebuilt-dir', a.prebuilt_dir), ('--template', a.template)):
            if value:
                cmd.extend([name, str(value.resolve(strict=True))])
        if a.template_sha256:
            cmd.extend(['--template-sha256', a.template_sha256])
    else:
        if not a.serial:
            p.error(localized('--serial is required for fastboot actions', 'fastboot の操作には --serial が必要です'))
        if a.action == 'install' and not a.trial_confirmed:
            p.error(localized('install requires --trial-confirmed after testing this exact bundle', 'install には、このイメージの試験結果を確認したうえで --trial-confirmed を指定してください'))
        cmd = [sys.executable, str(TOOLS / 'cube_install.py'), '--work', str(work / 'bundle'),
               '--slot', a.slot, '--serial', a.serial, '--action', a.action]
        if a.trial_confirmed:
            cmd.append('--trial-confirmed')
    subprocess.run(cmd, check=True)
    if a.action == 'build-guided':
        guided_install(work, a.slot, a.serial, a.wait_timeout)
    if a.action == 'trial':
        print('Return the same clock to fastboot before install.')
    if a.action == 'install':
        print('Boot success is not automatically marked; see the installation guide.')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, EOFError, subprocess.SubprocessError) as exc:
        print(message('stopped'), exc, file=sys.stderr)
        raise SystemExit(1)
