#!/usr/bin/env python3
"""Download and verify public build inputs into a new, private directory.

This does not obtain stock firmware, build a kernel, or access a device.
The pinned apk.static runs on x86_64 Linux; no packages are installed on the host.
"""
import argparse
import hashlib
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request

UPSTREAM_URL = 'https://github.com/HuskerMinion/techo5/releases/download/v1.6.44/rootfs-v1.6.44.tar.gz'
UPSTREAM_HASH = '0ce31244162481bbc3fb88b3c01ff7f257486191756054b0c54abf44bbe9ff9c'
ALPINE_URL = 'https://dl-cdn.alpinelinux.org/alpine/v3.24/releases/armv7/alpine-minirootfs-3.24.2-armv7.tar.gz'
ALPINE_HASH = '5552f1ef2398cee46ef2cf733f8f1258f39a77ad065dfd47282fc0df9fe1976c'
APK_URL = 'https://dl-cdn.alpinelinux.org/alpine/v3.24/main/x86_64/apk-tools-static-3.0.9-r0.apk'
APK_HASH = '0a1a0682375f66a83b7b903f6e4638ccfe312547d50d336b5f2be9fc391281be'


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def obtain(url, expected, target, local=None):
    if local:
        shutil.copyfile(local, target)
    else:
        request = urllib.request.Request(url, headers={'User-Agent': 'techo5-cube-input-preparer/1'})
        with urllib.request.urlopen(request, timeout=60) as response, target.open('xb') as out:
            size = 0
            while True:
                chunk = response.read(1024 * 1024)
                if not chunk:
                    break
                size += len(chunk)
                if size > 128 * 1024 * 1024:
                    raise ValueError('download exceeds size limit')
                out.write(chunk)
    if digest(target) != expected:
        raise ValueError('SHA-256 mismatch: ' + url)


def copy_member(archive, member, target, limit):
    if not member.isfile() or member.size <= 0 or member.size > limit:
        raise ValueError('invalid archive member: ' + member.name)
    with archive.extractfile(member) as source, target.open('xb') as out:
        shutil.copyfileobj(source, out)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--output', type=Path, required=True, help='new private output directory')
    p.add_argument('--upstream-archive', type=Path, help='optional local copy of the pinned upstream rootfs')
    p.add_argument('--alpine-archive', type=Path, help='optional local copy of the pinned Alpine rootfs')
    p.add_argument('--apk-package', type=Path, help='optional local copy of the pinned x86_64 apk-tools package')
    a = p.parse_args()
    output = a.output.resolve()
    if output.exists():
        p.error('output must not exist')
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.cube-inputs-', dir=output.parent) as tmp:
        stage = Path(tmp) / 'prepared'
        stage.mkdir()
        downloads = Path(tmp) / 'downloads'
        downloads.mkdir()
        upstream = downloads / 'rootfs.tar.gz'
        apk = downloads / 'apk-tools.apk'
        print('Fetching pinned upstream rootfs...', flush=True)
        obtain(UPSTREAM_URL, UPSTREAM_HASH, upstream, a.upstream_archive)
        print('Fetching pinned Alpine rootfs...', flush=True)
        obtain(ALPINE_URL, ALPINE_HASH, stage / 'alpine-minirootfs-3.24.2-armv7.tar.gz', a.alpine_archive)
        print('Fetching pinned Alpine apk-tools...', flush=True)
        obtain(APK_URL, APK_HASH, apk, a.apk_package)
        with tarfile.open(apk, 'r:gz') as archive:
            copy_member(archive, archive.getmember('sbin/apk.static'), stage / 'apk.static', 32 * 1024 * 1024)
        os.chmod(stage / 'apk.static', 0o755)
        models = stage / 'models'
        models.mkdir()
        found = set()
        with tarfile.open(upstream, 'r:gz') as archive:
            for member in archive:
                match = re.fullmatch(r'\./usr/share/techo5/models/([a-z0-9_]+\.(?:json|tflite))', member.name)
                if match:
                    name = match.group(1)
                    if name in found:
                        raise ValueError('duplicate model: ' + name)
                    copy_member(archive, member, models / name, 16 * 1024 * 1024)
                    found.add(name)
        if not {'okay_nabu.json', 'okay_nabu.tflite'}.issubset(found):
            raise ValueError('required wake-word model missing')
        helpers = stage / 'helpers'
        subprocess.run([sys.executable, str(Path(__file__).with_name('extract_upstream_helpers.py')), str(upstream), str(helpers)], check=True)
        (stage / 'SOURCES.txt').write_text(
            'Pinned inputs (SHA-256):\n' + UPSTREAM_URL + ' ' + UPSTREAM_HASH + '\n' +
            ALPINE_URL + ' ' + ALPINE_HASH + '\n' + APK_URL + ' ' + APK_HASH + '\n'
            'Upstream helpers retain their own license terms. Do not assume redistribution rights.\n', encoding='utf-8')
        stage.rename(output)
    print('Prepared public inputs in', output)
    print('Stock vendor/OEM data and a compatible kernel are still required.')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError, tarfile.TarError) as exc:
        print('Preparation stopped:', exc, file=sys.stderr)
        raise SystemExit(1)
