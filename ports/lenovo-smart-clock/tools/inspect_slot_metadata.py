#!/usr/bin/env python3
"""Inspect a saved Smart Clock misc partition or its 32-byte A/B record.

Read-only. It never opens a device or changes boot selection.
"""
import argparse
import json
import struct
import zlib
from pathlib import Path


def inspect(data):
    if len(data) == 32:
        block = data
    elif len(data) >= 0x800 + 32:
        block = data[0x800:0x800+32]
    else:
        raise ValueError("expected a 32-byte record or a full misc backup")
    if block[:4] != b"\0AB0" or block[4:6] != b"\x01\x00":
        raise ValueError("not a supported libavb A/B record")
    if struct.unpack(">I", block[28:32])[0] != zlib.crc32(block[:28]):
        raise ValueError("A/B record CRC mismatch")
    slots = {}
    for name, off in (("a", 8), ("b", 12)):
        priority, tries, successful, _ = block[off:off+4]
        if priority > 15 or tries > 7 or successful > 1:
            raise ValueError("invalid slot fields")
        slots[name] = {"priority": priority, "tries_remaining": tries, "successful": bool(successful)}
    return {"slots": slots, "note": "Offline metadata only; not proof of a working restore or boot."}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("misc_backup", type=Path)
    a = p.parse_args()
    try:
        if a.misc_backup.stat().st_size > 16*1024*1024:
            raise ValueError("misc image exceeds inspection limit")
        result = inspect(a.misc_backup.read_bytes())
    except (OSError, ValueError) as exc:
        p.exit(2, f"inspection failed: {exc}\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
