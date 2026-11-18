#!/usr/bin/env python3
"""Build the experimental 2019 cube kernel from Lenovo's published source.

Downloads pinned Lenovo source, reference patches and a pinned toolchain.
Requires an owner's compatible stock boot with an embedded kernel configuration.
Builds in a new private Linux directory; never accesses a connected device.
"""
import argparse
import hashlib
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import urllib.request
import zlib

from verify_diagnostic_boot import parse

LENOVO = 'https://download.lenovo.com/consumer/mobiles/lenovo_smart_clock_opensource_0.114.10_201904.tar.gz'
LENOVO_SHA256 = 'cd7af6eff981327c44f611cad919b2568a5d457da6e1d8037ef9046d86bf32d1'
REFERENCE = 'https://raw.githubusercontent.com/kortregt/lenovo-smart-clock-linux/e03d33f50651cc0766492b8eb89d90a53256d777/kernel/patches/'
PATCHES = {
    '0001-dtc-fix-yylloc-multiple-definition-with-host-gcc-10.patch': 'fe92f0a8fdb6c798a58d783496df80215fa1eafcb97ae46cfebca09704b89e53',
    '0002-initramfs-keep-built-in-initramfs-and-ignore-lk-ramdisk.patch': 'a63991aee966facc96c884659fe072e441b525b6597fc4a31f08827ae40beebe',
    '0003-dispsys-add-dsi_bist-and-pq_bypass-debug-commands.patch': '0509affb3481205858e82858617835b37593ed6f9c0d05f7a7845c02a05dda50',
    '0004-mtkfb-use-the-ION-framebuffer-mapping-in-kernel-only-display-mode.patch': '00b25be3bc1530f2b2a00962937dbeb88e69b4ebfc3f79127b1ff00b18d2e3f4',
}
TOOLCHAIN = 'https://github.com/LineageOS/android_prebuilts_gcc_linux-x86_aarch64_aarch64-linux-android-4.9.git'
TOOLCHAIN_COMMIT = '5e030eafe024784a73cdf47e6936ac0dbfc763dc'


def fetch(url, expected, dest, local=None, limit=1024 * 1024 * 1024):
    h = hashlib.sha256()
    request = urllib.request.Request(url, headers={'User-Agent': 'techo5-cube-kernel-builder/1'})
    with (local.open('rb') if local else urllib.request.urlopen(request, timeout=90)) as source, dest.open('xb') as out:
        size = 0
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            size += len(chunk)
            if size > limit:
                raise ValueError('input exceeds size limit: ' + url)
            h.update(chunk)
            out.write(chunk)
    if h.hexdigest() != expected:
        raise ValueError('SHA-256 mismatch: ' + url)


def decompress_stock_kernel(data):
    # Lenovo's Android boot payload appends a DTB after the gzip stream.
    if not data.startswith(b'\x1f\x8b'):
        raise ValueError('stock kernel is not gzip-compressed')
    d = zlib.decompressobj(16 + zlib.MAX_WBITS)
    output = d.decompress(data, 128 * 1024 * 1024 + 1)
    if not d.eof or len(output) > 128 * 1024 * 1024:
        raise ValueError('invalid or oversized stock kernel')
    return output


def run(args, **kwargs):
    print('+', ' '.join(str(x) for x in args), flush=True)
    subprocess.run([str(x) for x in args], check=True, **kwargs)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--donor-boot', type=Path, required=True)
    p.add_argument('--runtime-init', type=Path, required=True, help='ARM64 cube_boot init built from this repository')
    p.add_argument('--work', type=Path, required=True, help='new private directory on a case-sensitive Linux filesystem')
    p.add_argument('--lenovo-archive', type=Path, help='optional local copy of the exact pinned Lenovo source archive')
    p.add_argument('--jobs', type=int, default=4)
    a = p.parse_args()
    if sys.platform != 'linux':
        p.error('Linux is required')
    if not 1 <= a.jobs <= 64:
        p.error('jobs must be 1..64')
    for tool in ('git', 'tar', 'make', 'gcc', 'python3', 'gzip', 'cpio', 'bc', 'bison', 'flex'):
        if not shutil.which(tool):
            p.error('missing required build tool: ' + tool)
    work = a.work.resolve()
    if work.exists() or any(c.isspace() for c in str(work)):
        p.error('work must be a new path without whitespace')
    donor, init = a.donor_boot.resolve(strict=True), a.runtime_init.resolve(strict=True)
    kernel_bytes = parse(donor)['kernel']
    image = decompress_stock_kernel(kernel_bytes)
    if not re.search(rb'Linux version 4\.4\.95(?:\+|\s)', image):
        raise ValueError('donor kernel is not the supported 2019 4.4.95 baseline')
    if a.lenovo_archive:
        a.lenovo_archive = a.lenovo_archive.resolve(strict=True)
    work.mkdir(parents=True, mode=0o700)
    source_archive = work / 'lenovo-source.tar'
    print('Obtaining Lenovo published source...', flush=True)
    fetch(LENOVO, LENOVO_SHA256, source_archive, a.lenovo_archive)
    # The pinned Lenovo file has a .tar.gz name but is an uncompressed tar.
    run(['tar', '--no-same-owner', '--no-same-permissions', '-xf', source_archive, '-C', work])
    src = work / 'T400_kernel_OSC'
    if not (src / 'Makefile').is_file():
        raise ValueError('unexpected Lenovo source layout')
    patchdir = work / 'patches'
    patchdir.mkdir()
    for name, expected in PATCHES.items():
        patch = patchdir / name
        fetch(REFERENCE + name, expected, patch, limit=1024 * 1024)
        run(['git', '-C', src, 'apply', '--check', '--ignore-whitespace', patch])
        run(['git', '-C', src, 'apply', '--ignore-whitespace', patch])
    # The reference patch targets a later kernel and uses a symbol absent in 2019.
    fb = src / 'drivers/misc/mediatek/video/mt8167/videox/mtkfb.c'
    data = fb.read_bytes()
    old = b'if (!is_lk_show_logo) {'
    if data.count(old) != 1:
        raise ValueError('unexpected 2019 framebuffer patch context')
    fb.write_bytes(data.replace(old, b'if (fb_virtual_address != NULL) {', 1))
    stock_image = work / 'stock-kernel-uncompressed'
    stock_image.write_bytes(image)
    config = work / 'stock.config'
    with config.open('xb') as out:
        run(['sh', src / 'scripts/extract-ikconfig', stock_image], stdout=out)
    cfg = config.read_text(encoding='utf-8')
    if not re.search(r'^CONFIG_ARM64=y$', cfg, re.M) or not re.search(r'^CONFIG_MTK_PLATFORM="mt8167"$', cfg, re.M):
        raise ValueError('donor kernel configuration does not appear to be MT8167 ARM64')
    toolchain = work / 'toolchain'
    run(['git', 'init', '-q', toolchain])
    run(['git', '-C', toolchain, 'remote', 'add', 'origin', TOOLCHAIN])
    run(['git', '-C', toolchain, 'fetch', '--depth=1', 'origin', TOOLCHAIN_COMMIT])
    run(['git', '-C', toolchain, 'checkout', '--detach', 'FETCH_HEAD'])
    actual = subprocess.check_output(['git', '-C', str(toolchain), 'rev-parse', 'HEAD'], text=True).strip()
    if actual != TOOLCHAIN_COMMIT:
        raise ValueError('unexpected toolchain commit')
    for f in (toolchain / 'bin').iterdir():
        if f.is_file() and not f.is_symlink():
            data = f.read_bytes()
            if data.startswith(b'#!/usr/bin/python\n'):
                f.write_bytes(data.replace(b'#!/usr/bin/python\n', b'#!/usr/bin/env python3\n', 1))
    env = os.environ.copy()
    env.update({'CUBE_KERNEL_SRC': str(src), 'CUBE_STOCK_CONFIG': str(config), 'CUBE_RUNTIME_INIT': str(init),
                'CUBE_KERNEL_BUILD': str(work / 'kernel-build'), 'CROSS_COMPILE': str(toolchain / 'bin/aarch64-linux-android-'),
                'CUBE_JOBS': str(a.jobs)})
    run(['bash', Path(__file__).with_name('build_cube_kernel.sh')], env=env)
    print('Kernel output:', work / 'kernel-build/out')
    print('Experimental build; source provenance and hardware compatibility require verification.')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, zlib.error, subprocess.SubprocessError) as exc:
        print('Kernel build stopped:', exc, file=sys.stderr)
        raise SystemExit(1)
