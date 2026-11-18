#!/usr/bin/env python3
"""Audit or repair known prototype overlay ownership on the running cube.

Read-only unless --apply is supplied. This touches only a fixed list of known
system paths on the active marked cube root, not settings or other filesystems.
"""
import argparse
import os
from pathlib import Path
import stat

DIRS = ("/etc", "/etc/profile.d", "/etc/techo5", "/etc/techo5-cube",
        "/usr", "/usr/local", "/usr/local/sbin", "/usr/local/libexec",
        "/usr/local/libexec/__pycache__", "/usr/share/licenses")
FILES = ("/etc/motd", "/etc/inittab", "/etc/hostname", "/etc/profile.d/techo5.sh",
         "/etc/techo5/boot.sh", "/etc/techo5/ntp-event.sh", "/etc/techo5/shutdown.sh",
         "/etc/techo5/udhcpc.sh", "/etc/techo5-cube/rcS", "/usr/local/sbin/techo5-run",
         "/usr/local/sbin/cube-run", "/usr/local/sbin/cube-update-watch", "/usr/local/sbin/cube-shared-mount",
         "/usr/local/sbin/techo5-firewall", "/usr/local/sbin/techo5-net", "/usr/local/libexec/cube_shared_data.py")
CACHE = "/usr/local/libexec/__pycache__/cube_shared_data.cpython-312.pyc"


def no_symlink_path(path, rootdev):
    current = Path("/")
    for component in Path(path).parts[1:]:
        current /= component
        info = current.lstat()
        if stat.S_ISLNK(info.st_mode) or info.st_dev != rootdev:
            raise ValueError("symlink or other filesystem in known path")


def running_cube_root():
    if os.geteuid() != 0:
        raise ValueError("run as root")
    fd = os.open("/etc/techo5-cube-root", os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    with os.fdopen(fd, "rb") as f:
        if f.read(129) != b"TECHO5-CUBE-ROOT-V1\n":
            raise ValueError("cube root marker missing")
    words = Path("/proc/cmdline").read_text().split()
    slots = [w for w in words if w.startswith("androidboot.slot_suffix=")]
    roots = [w for w in words if w.startswith("techo5.cube.root=")]
    if len(slots) != 1 or slots[0] not in ("androidboot.slot_suffix=_a", "androidboot.slot_suffix=_b"):
        raise ValueError("ambiguous running slot")
    slot = slots[0][-1]
    if roots != ["techo5.cube.root=system_" + slot]:
        raise ValueError("running root does not match slot")
    rootdev = os.stat("/").st_dev
    matches = []
    for p in Path("/sys/class/block").glob("mmcblk*p*"):
        try:
            fields = dict(line.split("=", 1) for line in (p / "uevent").read_text().splitlines() if "=" in line)
            if fields.get("PARTNAME") == "system_" + slot:
                matches.append((int(fields["MAJOR"]), int(fields["MINOR"])))
        except (OSError, KeyError, ValueError):
            continue
    if matches != [(os.major(rootdev), os.minor(rootdev))]:
        raise ValueError("active root device is not the selected system partition")
    return rootdev


def expected_mode(path, directory):
    if directory:
        return 0o755
    if path.startswith("/usr/local/sbin/") or (path.startswith("/etc/techo5/") and path.endswith(".sh")) or path == "/etc/techo5-cube/rcS":
        return 0o755
    return 0o644


def collect(rootdev):
    paths = set(DIRS + FILES)
    licenses = Path("/usr/share/licenses")
    if licenses.is_dir() and not licenses.is_symlink():
        for root, dirs, files in os.walk(licenses, followlinks=False):
            dirs[:] = [d for d in dirs if not (Path(root) / d).is_symlink()]
            paths.update(str(Path(root) / name) for name in dirs + files if not (Path(root) / name).is_symlink())
    changes = []
    for path in sorted(paths, key=lambda p: (p.count("/"), p)):
        try:
            no_symlink_path(path, rootdev)
            info = os.lstat(path)
        except FileNotFoundError:
            continue
        if not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)):
            raise ValueError("unexpected object at known system path")
        if info.st_uid not in (0, 1000):
            raise ValueError("unexpected owner at known system path")
        mode = expected_mode(path, stat.S_ISDIR(info.st_mode))
        if info.st_uid == 1000 or stat.S_IMODE(info.st_mode) & 0o022:
            changes.append((path, mode, info.st_uid, info.st_ino))
    return changes


def repair(rootdev, changes):
    for path, mode, owner, inode in changes:
        no_symlink_path(path, rootdev)
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
        try:
            info = os.fstat(fd)
            if info.st_dev != rootdev or info.st_ino != inode or info.st_uid != owner:
                raise ValueError("system path changed during repair")
            os.fchown(fd, 0, 0)
            os.fchmod(fd, mode)
        finally:
            os.close(fd)
    # A bytecode cache under a previously writable directory is regenerable.
    try:
        no_symlink_path(CACHE, rootdev)
        info = os.lstat(CACHE)
        if stat.S_ISREG(info.st_mode) and info.st_dev == rootdev and info.st_uid in (0, 1000):
            os.unlink(CACHE)
    except FileNotFoundError:
        pass
    os.sync()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--apply", action="store_true", help="repair the listed known paths")
    a = p.parse_args()
    rootdev = running_cube_root()
    changes = collect(rootdev)
    for path, mode, _, _ in changes:
        print(f"{path}: root:root {mode:04o}")
    if a.apply:
        repair(rootdev, changes)
        print(f"Repaired {len(changes)} known paths.")
    else:
        print(f"{len(changes)} known paths need repair; no changes made.")


if __name__ == "__main__":
    main()
