#!/usr/bin/env python3
"""Compare two local Android v0 boot backups without touching a device.

This does not establish device identity, recovery, or permission to flash.
Trailing partition padding is intentionally ignored by component comparisons.
"""
import argparse
import json
from pathlib import Path
from verify_diagnostic_boot import digest, parse


def compare(owner, reference):
    a, b = parse(owner), parse(reference)
    checks = {
        "whole_file_identical": a["sha256"] == b["sha256"],
        "header_fields_identical": a["fields"] == b["fields"],
        "command_line_identical": a["cmdline"] == b["cmdline"],
        "kernel_and_dtb_identical": a["kernel"] == b["kernel"],
        "ramdisk_identical": a["ramdisk"] == b["ramdisk"],
        "second_stage_identical": a["second"] == b["second"],
        "boot_id_identical": a["id"] == b["id"],
    }
    return {"checks": checks, "owner_sha256": a["sha256"],
            "reference_sha256": b["sha256"],
            "owner_kernel_sha256": digest(a["kernel"]),
            "reference_kernel_sha256": digest(b["kernel"]),
            "note": "Read-only comparison; even identical contents do not prove device identity, backup completeness, restore capability or flash safety."}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("owner_boot", type=Path, help="offline boot backup from the owner's device")
    p.add_argument("reference_boot", type=Path, help="offline boot image used as a research reference")
    args = p.parse_args()
    try:
        result = compare(args.owner_boot, args.reference_boot)
    except (OSError, ValueError, UnicodeError) as exc:
        p.exit(2, f"comparison failed: {exc}\n")
    print(json.dumps(result, indent=2))
    if not all(result["checks"].values()):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
