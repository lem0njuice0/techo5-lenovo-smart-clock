#!/usr/bin/env python3
"""Verify a diagnostic Android v0 boot image against its donor and inputs.

Read-only; successful verification is not proof that an image can boot a clock.
"""
import argparse
import hashlib
import json
import struct
from pathlib import Path

HEADER = struct.Struct("<8s10I16s512s32s1024s")
MAX_IMAGE = 128 * 1024 * 1024


def digest(data):
    return hashlib.sha256(data).hexdigest()


def parse(path):
    if path.stat().st_size > MAX_IMAGE:
        raise ValueError("image too large")
    data = path.read_bytes()
    if len(data) < HEADER.size:
        raise ValueError("truncated header")
    magic, ks, ka, rs, ra, ss, sa, tags, page, version, osver, name, cmd, ident, extra = HEADER.unpack_from(data)
    if magic != b"ANDROID!" or version != 0 or page < HEADER.size or page > 65536 or page & (page - 1):
        raise ValueError("not a supported Android v0 boot image")
    align = lambda n: (n + page - 1) // page * page
    koff, roff = page, page + align(ks)
    soff = roff + align(rs)
    if not ks or not rs or koff + ks > len(data) or roff + rs > len(data) or soff + ss > len(data):
        raise ValueError("invalid component size")
    return {"fields": (ka, ra, sa, tags, page, osver, name, extra), "kernel": data[koff:koff+ks], "ramdisk": data[roff:roff+rs], "second": data[soff:soff+ss], "cmdline": cmd.rstrip(b"\0").decode("ascii"), "id": ident, "sha256": digest(data)}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("donor", type=Path)
    p.add_argument("candidate", type=Path)
    p.add_argument("kernel_gz", type=Path)
    p.add_argument("dtb", type=Path)
    p.add_argument("--expect-system-b", action="store_true", help="expect the experimental marked-root flag")
    a = p.parse_args()
    donor, candidate = parse(a.donor), parse(a.candidate)
    expected = a.kernel_gz.read_bytes() + a.dtb.read_bytes()
    boot_id = hashlib.sha1()
    for part in (candidate["kernel"], candidate["ramdisk"], candidate["second"]):
        boot_id.update(part)
        boot_id.update(struct.pack("<I", len(part)))
    root_args = [x for x in candidate["cmdline"].split() if x.startswith("techo5.cube.root=")]
    expected_root = ["techo5.cube.root=system_b"] if a.expect_system_b else []
    donor_args = donor["cmdline"].split()
    expected_args = donor_args + expected_root
    if "keep_initramfs" not in expected_args:
        expected_args.append("keep_initramfs")
    checks = {"root_mode_matches": root_args == expected_root, "command_line_matches": candidate["cmdline"].split() == expected_args and not any(x.startswith("techo5.cube.root=") for x in donor_args), "boot_id_matches": candidate["id"] == boot_id.digest().ljust(32, b"\0"), "header_fields_match": donor["fields"] == candidate["fields"], "ramdisk_unchanged": donor["ramdisk"] == candidate["ramdisk"], "second_stage_unchanged": donor["second"] == candidate["second"], "kernel_and_dtb_match_inputs": candidate["kernel"] == expected, "keep_initramfs_requested": "keep_initramfs" in candidate["cmdline"].split()}
    print(json.dumps({"checks": checks, "donor_sha256": donor["sha256"], "candidate_sha256": candidate["sha256"], "cmdline": candidate["cmdline"], "note": "structural check only; no hardware verification"}, indent=2))
    if not all(checks.values()):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
