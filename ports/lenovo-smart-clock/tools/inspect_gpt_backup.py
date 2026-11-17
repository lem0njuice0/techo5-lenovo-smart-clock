#!/usr/bin/env python3
"""Inspect a raw 512-byte-sector eMMC user-area backup, without modifying it.

This does not read a device, validate eMMC boot areas, restore a backup, or
establish that any partition can safely be written.
"""
import argparse
import binascii
import hashlib
import json
import struct
import uuid
from pathlib import Path

SECTOR = 512
HEADER = struct.Struct("<8sIIIIQQQQ16sQIII")
MAX_ENTRIES_BYTES = 16 * 1024 * 1024


def crc(data):
    return binascii.crc32(data) & 0xffffffff


def read_at(f, offset, length, size):
    if offset < 0 or length < 0 or offset + length > size:
        raise ValueError("GPT data outside backup")
    f.seek(offset)
    data = f.read(length)
    if len(data) != length:
        raise ValueError("truncated backup")
    return data


def read_header(f, lba, size):
    raw = read_at(f, lba * SECTOR, SECTOR, size)
    sig, revision, length, checksum, reserved, current, backup, first, last, disk, entries, count, entry_size, array_crc = HEADER.unpack_from(raw)
    if sig != b"EFI PART" or revision != 0x10000 or length < HEADER.size or length > SECTOR or reserved or current != lba:
        raise ValueError("invalid GPT header")
    data = bytearray(raw[:length])
    data[16:20] = b"\0" * 4
    if crc(data) != checksum:
        raise ValueError("GPT header CRC mismatch")
    nbytes = count * entry_size
    if not count or entry_size < 128 or entry_size % 8 or nbytes > MAX_ENTRIES_BYTES:
        raise ValueError("invalid GPT entry table size")
    table = read_at(f, entries * SECTOR, nbytes, size)
    if crc(table) != array_crc:
        raise ValueError("GPT entry CRC mismatch")
    return {"current": current, "backup": backup, "first": first, "last": last, "disk": disk,
            "entries_lba": entries, "entry_count": count, "entry_size": entry_size, "table": table}


def inspect(path, hash_file=False):
    size = path.stat().st_size
    if size < 3 * SECTOR or size % SECTOR:
        raise ValueError("expected a sector-aligned raw disk backup")
    total = size // SECTOR
    with path.open("rb") as f:
        mbr = read_at(f, 0, SECTOR, size)
        if mbr[510:512] != b"\x55\xaa" or not any(mbr[446+i*16+4] == 0xee for i in range(4)):
            raise ValueError("missing protective MBR")
        primary = read_header(f, 1, size)
        backup = read_header(f, total - 1, size)
        if primary["backup"] != total - 1 or backup["backup"] != 1:
            raise ValueError("GPT headers do not describe this complete disk")
        for key in ("first", "last", "disk", "entry_count", "entry_size", "table"):
            if primary[key] != backup[key]:
                raise ValueError("primary and backup GPT differ: " + key)
        if not (2 <= primary["first"] <= primary["last"] < total - 1):
            raise ValueError("invalid usable LBA range")
        table_sectors = (primary["entry_count"] * primary["entry_size"] + SECTOR - 1) // SECTOR
        if (primary["entries_lba"] < 2 or primary["entries_lba"] + table_sectors > primary["first"]
                or backup["entries_lba"] <= primary["last"]
                or backup["entries_lba"] + table_sectors > total - 1
                or primary["disk"] == bytes(16)):
            raise ValueError("invalid GPT table placement or disk GUID")
        parts, names, guids = [], set(), set()
        table, step = primary["table"], primary["entry_size"]
        for i in range(primary["entry_count"]):
            entry = table[i*step:(i+1)*step]
            if entry[:16] == bytes(16):
                continue
            type_id = str(uuid.UUID(bytes_le=entry[:16]))
            part_id = str(uuid.UUID(bytes_le=entry[16:32]))
            start, end, attrs = struct.unpack_from("<QQQ", entry, 32)
            try:
                name = entry[56:128].decode("utf-16le").split("\0", 1)[0]
            except UnicodeDecodeError as exc:
                raise ValueError("invalid partition name") from exc
            if not name or name in names or part_id in guids or part_id == str(uuid.UUID(int=0)):
                raise ValueError("missing or duplicate partition name/GUID")
            if not primary["first"] <= start <= end <= primary["last"]:
                raise ValueError("partition outside usable range: " + name)
            names.add(name); guids.add(part_id)
            parts.append({"name": name, "start_lba": start, "end_lba": end,
                          "size_bytes": (end-start+1)*SECTOR, "type_guid": type_id,
                          "partition_guid": part_id, "attributes": attrs})
        parts.sort(key=lambda p: p["start_lba"])
        for a, b in zip(parts, parts[1:]):
            if a["end_lba"] >= b["start_lba"]:
                raise ValueError("overlapping partitions: " + a["name"] + ", " + b["name"])
        result = {"disk_size_bytes": size, "sector_size": SECTOR,
                  "disk_guid": str(uuid.UUID(bytes_le=primary["disk"])), "partitions": parts,
                  "required_names_present": {n: n in names for n in ("boot_a", "boot_b", "system_a", "system_b", "vendor_a", "oem_a", "misc", "seccfg")},
                  "note": "Offline structural inspection only. eMMC boot areas, device identity, backup correctness and restoration are not verified."}
        if hash_file:
            f.seek(0)
            h = hashlib.sha256()
            for chunk in iter(lambda: f.read(4*1024*1024), b""):
                h.update(chunk)
            result["sha256"] = h.hexdigest()
        return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("image", type=Path)
    p.add_argument("--sha256", action="store_true", help="hash the entire input (may take time)")
    p.add_argument("--candidate-boot", type=Path, help="compare candidate size to boot_b")
    p.add_argument("--candidate-root", type=Path, help="compare candidate size to system_b")
    a = p.parse_args()
    try:
        result = inspect(a.image, a.sha256)
        by_name = {part["name"]: part for part in result["partitions"]}
        sizes = {}
        for name, path in (("boot_b", a.candidate_boot), ("system_b", a.candidate_root)):
            if path is not None:
                length = path.stat().st_size
                capacity = by_name.get(name, {}).get("size_bytes")
                sizes[name] = {"image_size_bytes": length, "partition_size_bytes": capacity,
                               "fits": capacity is not None and 0 < length <= capacity}
        if sizes:
            result["candidate_sizes"] = sizes
    except (OSError, ValueError, struct.error) as exc:
        p.exit(2, f"inspection failed: {exc}\n")
    print(json.dumps(result, indent=2))
    if not all(result["required_names_present"].values()) or not all(x["fits"] for x in result.get("candidate_sizes", {}).values()):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
