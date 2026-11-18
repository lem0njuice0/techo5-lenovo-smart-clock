#!/usr/bin/env python3
"""Prepare a separate, marked data directory on an already mounted userdata FS.

Called before services start. Never formats userdata or removes the old data.
The caller must establish that userdata is the intended mounted partition.
"""
import argparse
import os
from pathlib import Path
import re
import shutil
import stat
import tempfile

MARKER = b"TECHO5-CUBE-SHARED-DATA-V1\n"


def find_userdata(sys_block, dev_root, stat_device=os.stat):
    found = []
    for entry in sys_block.iterdir():
        if not re.fullmatch(r"mmcblk[0-9]+p[0-9]+", entry.name):
            continue
        try:
            lines = (entry / "uevent").read_text().splitlines()
        except OSError:
            continue
        pairs = [line.split("=", 1) for line in lines if "=" in line]
        values = {}
        for key, value in pairs:
            if key in values:
                raise ValueError("duplicate sysfs field")
            values[key] = value
        if values.get("PARTNAME") != "userdata":
            continue
        if values.get("DEVNAME") != entry.name:
            raise ValueError("userdata device name mismatch")
        device = dev_root / entry.name
        info = stat_device(device)
        if not stat.S_ISBLK(info.st_mode) or os.major(info.st_rdev) != int(values["MAJOR"]) or os.minor(info.st_rdev) != int(values["MINOR"]):
            raise ValueError("userdata device number mismatch")
        found.append(device)
    if len(found) != 1:
        raise ValueError("expected exactly one userdata eMMC partition")
    return found[0]


def directory(path):
    try:
        return stat.S_ISDIR(path.lstat().st_mode)
    except FileNotFoundError:
        return False


def checked_store(store):
    if not directory(store) or not directory(store / "data"):
        raise ValueError("shared store or data is not a real directory")
    marker = store / "marker"
    info = marker.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_size != len(MARKER) or marker.read_bytes() != MARKER:
        raise ValueError("shared store marker is invalid")
    return store / "data"


def prepare(userdata, old_data, bluetooth=None, bluealsa=None):
    if not directory(userdata) or not directory(old_data):
        raise ValueError("userdata and existing data must be real directories")
    store = userdata / "techo5-cube"
    if store.exists() or store.is_symlink():
        return checked_store(store), False
    stage = Path(tempfile.mkdtemp(prefix=".techo5-cube-migrate-", dir=userdata))
    os.chmod(stage, 0o700)
    # The stage is left for diagnosis if copying fails. Original data is intact.
    data = stage / "data"
    shutil.copytree(old_data, data, symlinks=True)
    for source, name in ((bluetooth, "bluetooth"), (bluealsa, "bluealsa")):
        if source is None or not directory(source):
            continue
        parent = data / "techo5-linux"
        if parent.is_symlink() or (parent.exists() and not directory(parent)):
            raise ValueError("Bluetooth state parent is not a real directory")
        parent.mkdir(exist_ok=True)
        target = parent / name
        if target.exists() or target.is_symlink():
            raise ValueError("Bluetooth state target already exists")
        shutil.copytree(source, target, symlinks=True)
    with (stage / "marker").open("xb") as f:
        f.write(MARKER)
        f.flush()
        os.fsync(f.fileno())
    os.sync()
    if store.exists() or store.is_symlink():
        raise FileExistsError("shared store appeared during migration")
    os.rename(stage, store)
    os.sync()
    return checked_store(store), True


def main():
    p = argparse.ArgumentParser(description=__doc__)
    sub = p.add_subparsers(dest="action", required=True)
    find = sub.add_parser("find")
    find.add_argument("sys_block", type=Path)
    find.add_argument("dev_root", type=Path)
    prep = sub.add_parser("prepare")
    prep.add_argument("userdata", type=Path)
    prep.add_argument("old_data", type=Path)
    prep.add_argument("--bluetooth", type=Path)
    prep.add_argument("--bluealsa", type=Path)
    a = p.parse_args()
    if a.action == "find":
        print(find_userdata(a.sys_block, a.dev_root))
    else:
        data, created = prepare(a.userdata, a.old_data, a.bluetooth, a.bluealsa)
        print(("created " if created else "reused ") + str(data))


if __name__ == "__main__":
    main()
