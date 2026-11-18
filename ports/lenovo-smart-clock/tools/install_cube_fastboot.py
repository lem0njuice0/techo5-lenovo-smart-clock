#!/usr/bin/env python3
"""Experimental two-stage installer for an unlocked CD-24501F.

Inspect by default. `trial` writes the explicitly selected inactive system slot
and boots a temporary image; `install` writes its boot slot and selects it after
the owner has checked the trial. The active stock slot is not written. The
installer has not yet been validated on a second unit.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

from verify_diagnostic_boot import parse as parse_boot


def sha256(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def call(serial, *args):
    result = subprocess.run(["fastboot", "-s", serial, *args], capture_output=True, text=True, timeout=900)
    out = result.stdout + "\n" + result.stderr
    if result.returncode or re.search(r"\bFAILED\b", out, re.I):
        raise RuntimeError("fastboot " + " ".join(args[:2]) + " failed: " + out[-1000:])
    return out


def variable(serial, name):
    out = call(serial, "getvar", name)
    match = re.search(r"(?:^|\n)(?:\(bootloader\)\s*)?" + re.escape(name) + r"\s*:\s*(\S+)", out, re.I)
    if not match:
        raise ValueError("fastboot did not report " + name)
    return match.group(1)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--serial", required=True, help="specific fastboot device serial")
    p.add_argument("--slot", choices=("a", "b"), required=True, help="inactive slot to install")
    p.add_argument("--boot", type=Path, required=True)
    p.add_argument("--boot-sha256", required=True, help="expected SHA-256 of the boot image")
    p.add_argument("--system", type=Path, required=True)
    p.add_argument("--root-report", type=Path, required=True, help="JSON from verify_private_rootfs.py")
    p.add_argument("--action", choices=("inspect", "trial", "install"), default="inspect")
    p.add_argument("--trial-confirmed", action="store_true", help="confirm this exact system slot trial was tested")
    a = p.parse_args()
    if a.action == "install" and not a.trial_confirmed:
        p.error("install requires --trial-confirmed after the same system slot trial has been checked")
    if not re.fullmatch(r"[A-Za-z0-9._-]+", a.serial) or not re.fullmatch(r"[a-fA-F0-9]{64}", a.boot_sha256):
        p.error("invalid serial or boot SHA-256")
    if sha256(a.boot) != a.boot_sha256.lower():
        p.error("boot image SHA-256 mismatch")
    boot = parse_boot(a.boot)
    root_args = [x for x in boot["cmdline"].split() if x.startswith("techo5.cube.root=")]
    if root_args != ["techo5.cube.root=system_" + a.slot]:
        p.error("boot image does not request the selected marked root")
    report = json.loads(a.root_report.read_text(encoding="utf-8"))
    checks = report.get("checks")
    if not isinstance(checks, dict) or not checks or any(value is not True for value in checks.values()) or checks.get("root_marker") is not True:
        p.error("rootfs verification report has failing or missing checks")
    if report.get("image_sha256") != sha256(a.system):
        p.error("rootfs image does not match its verification report")
    with a.system.open("rb") as f:
        f.seek(1080)
        if f.read(2) != b"\x53\xef":
            p.error("system image is not ext4")
    if variable(a.serial, "product") != "mt8167s":
        p.error("unexpected fastboot product")
    if variable(a.serial, "unlocked").lower() != "yes":
        p.error("bootloader is locked")
    active = variable(a.serial, "current-slot").lower().lstrip("_")
    if active not in ("a", "b") or active == a.slot:
        p.error("the target must be the inactive slot")
    if variable(a.serial, "slot-successful:" + active).lower() != "yes" or variable(a.serial, "slot-unbootable:" + active).lower() != "no":
        p.error("the active stock slot is not confirmed successful and bootable")
    avb = call(a.serial, "getvar", "at-vboot-state")
    if not re.search(r"\bavb-locked\s*[:=]\s*0\b", avb, re.I):
        p.error("AVB is not confirmed unlocked")
    boot_size = int(variable(a.serial, "partition-size:boot_" + a.slot), 0)
    system_size = int(variable(a.serial, "partition-size:system_" + a.slot), 0)
    if a.boot.stat().st_size > boot_size or a.system.stat().st_size != system_size:
        p.error("candidate sizes do not match the device partitions")
    print("Preflight passed; active stock slot " + active.upper() + ", target " + a.slot.upper() + ".", flush=True)
    if a.action == "trial":
        call(a.serial, "flash", "system_" + a.slot, str(a.system))
        print("Wrote the selected system slot; starting the temporary boot image.", flush=True)
        call(a.serial, "boot", str(a.boot))
        print("Trial requested. Check screen, touch, network and recovery before installing boot_" + a.slot + ".")
    elif a.action == "install":
        call(a.serial, "flash", "boot_" + a.slot, str(a.boot))
        call(a.serial, "set_active", a.slot)
        print("Wrote and selected the target boot slot. A normal-boot and slot-success check is still needed.")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as exc:
        print("Installer stopped:", exc, file=sys.stderr)
        raise SystemExit(1)
