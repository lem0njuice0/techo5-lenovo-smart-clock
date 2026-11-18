#!/usr/bin/env python3
"""Construct an unsigned diagnostic Android v0 boot image from an owner's donor.

This only writes a new file; it never communicates with or flashes a device.
Compatibility and recovery must be established on the actual clock first.
"""
import argparse
import hashlib
import struct
from pathlib import Path
from verify_diagnostic_boot import HEADER, parse


def aligned(data, page):
    return data + b"\0" * (-len(data) % page)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("donor", type=Path)
    p.add_argument("kernel_gz", type=Path)
    p.add_argument("dtb", type=Path)
    p.add_argument("output", type=Path, help="new file; will not overwrite")
    modes = p.add_mutually_exclusive_group()
    modes.add_argument("--experimental-system-b", action="store_true", help="request boot of a marked system_b root")
    modes.add_argument("--experimental-system-a", action="store_true", help="request boot of a marked system_a root")
    a = p.parse_args()
    donor = parse(a.donor)
    if a.output.exists():
        p.error("output already exists")
    kernel, dtb = a.kernel_gz.read_bytes(), a.dtb.read_bytes()
    if not kernel.startswith(b"\x1f\x8b") or not dtb.startswith(b"\xd0\x0d\xfe\xed"):
        p.error("expected a gzip kernel and flattened device tree")
    # A non-empty second command-line field would make appending ambiguous.
    ka, ra, sa, tags, page, osver, name, extra = donor["fields"]
    if any(extra):
        p.error("donor uses extended command line; inspect manually")
    parts = donor["cmdline"].split()
    if any(part.startswith("techo5.cube.root=") for part in parts):
        p.error("donor already contains a cube root argument; inspect manually")
    if a.experimental_system_b:
        parts.append("techo5.cube.root=system_b")
    elif a.experimental_system_a:
        parts.append("techo5.cube.root=system_a")
    if "keep_initramfs" not in parts:
        parts.append("keep_initramfs")
    cmdline = " ".join(parts).encode("ascii")
    if len(cmdline) >= 512:
        p.error("command line too long")
    kernel += dtb
    ramdisk, second = donor["ramdisk"], donor["second"]
    sha = hashlib.sha1()
    for part in (kernel, ramdisk, second):
        sha.update(part)
        sha.update(struct.pack("<I", len(part)))
    ident = sha.digest().ljust(32, b"\0")
    header = HEADER.pack(b"ANDROID!", len(kernel), ka, len(ramdisk), ra, len(second), sa, tags, page, 0, osver, name, cmdline, ident, extra)
    img = aligned(header, page) + aligned(kernel, page) + aligned(ramdisk, page) + (aligned(second, page) if second else b"")
    # Exclusive creation protects the donor and any existing image.
    with a.output.open("xb") as f:
        f.write(img)
    print(f"Created unsigned experimental image {a.output} ({len(img)} bytes); no device was written.")


if __name__ == "__main__":
    main()
