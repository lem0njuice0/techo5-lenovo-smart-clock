#!/usr/bin/env python3
"""Extract the Smart Clock OEM APK from an owner's raw ext4 image or directory."""
import argparse
import hashlib
import json
import shutil
import subprocess
import tempfile
import zipfile
from pathlib import Path

REL = Path("app/com.google.android.things.sparrow.oem/app.apk")


def sha256(p):
    h = hashlib.sha256()
    with p.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("source", type=Path, help="raw ext4 oem partition or extracted oem directory")
    p.add_argument("output", type=Path, help="new private directory")
    a = p.parse_args()
    source = a.source.resolve(strict=True)
    output = a.output.resolve()
    if output.exists() or (source.is_dir() and source in output.parents):
        p.error("output must be a new directory outside source")
    if not source.is_dir() and not source.is_file():
        p.error("source must be an image or directory")
    if source.is_file() and not shutil.which("debugfs"):
        p.error("debugfs is required for an ext4 image")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".cube-oem-", dir=output.parent) as temp:
        stage = Path(temp) / "data"
        stage.mkdir()
        apk = stage / "app.apk"
        if source.is_dir():
            f = source
            for component in REL.parts:
                f = f / component
                if f.is_symlink():
                    raise ValueError(f"refusing symlink: {f}")
            if f.is_file():
                shutil.copyfile(f, apk)
        else:
            subprocess.run(["debugfs", "-R", f"dump /{REL.as_posix()} app.apk", str(source)], cwd=stage, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
        if not apk.is_file() or not 0 < apk.stat().st_size <= 64 * 1024 * 1024 or not zipfile.is_zipfile(apk):
            raise ValueError("OEM APK is missing, oversized or not a ZIP")
        with zipfile.ZipFile(apk) as z:
            if "classes.dex" not in z.namelist():
                raise ValueError("APK has no classes.dex")
        manifest = {"format": 1, "source_kind": "directory" if source.is_dir() else "ext4-image", "source_image_sha256": None if source.is_dir() else sha256(source), "apk_sha256": sha256(apk), "apk_size": apk.stat().st_size}
        (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        stage.rename(output)
    print(f"Extracted OEM APK to {output}; keep this file private.")


if __name__ == "__main__":
    main()
