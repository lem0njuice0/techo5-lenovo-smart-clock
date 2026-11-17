#!/usr/bin/env python3
"""Extract the Wi-Fi/BT firmware needed by the sparrow Linux reference.

Input is an owner's raw ext4 vendor partition or an extracted vendor directory.
Does not extract touch firmware: the stock driver may reflash the controller.
"""
import argparse
import hashlib
import json
import shutil
import subprocess
import tempfile
from pathlib import Path

REQUIRED = ("EEPROM_MT7668.bin", "TxPwrLimit_MT76x8.dat", "WIFI_RAM_CODE2_SDIO_MT7668.bin", "WIFI_RAM_CODE_MT7668.bin", "mt7668_patch_e2_hdr.bin", "wifi.cfg")
OPTIONAL = ("rgx.fw.signed.22.40.54.30",)


def sha256(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("source", type=Path, help="raw ext4 vendor image or extracted vendor directory")
    ap.add_argument("output", type=Path, help="new private output directory")
    args = ap.parse_args()
    source = args.source.resolve(strict=True)
    output = args.output.resolve()
    if output.exists() or (source.is_dir() and (output == source or source in output.parents)):
        ap.error("output must be a new directory outside the source")
    if not source.is_dir() and not source.is_file():
        ap.error("source must be a directory or regular file")
    if source.is_file() and not shutil.which("debugfs"):
        ap.error("debugfs is required to read a raw ext4 image")
    output.parent.mkdir(parents=True, exist_ok=True)
    # Stage alongside output so rename is atomic. An error never leaves a partial output.
    with tempfile.TemporaryDirectory(prefix=".cube-firmware-", dir=output.parent) as temp:
        stage = Path(temp) / "data"
        stage.mkdir()
        files = []
        for name in REQUIRED + OPTIONAL:
            dest = stage / name
            if source.is_dir():
                f = source / "firmware" / name
                if f.parent.is_symlink() or f.is_symlink():
                    raise ValueError(f"refusing symlink: {f}")
                if f.is_file():
                    shutil.copyfile(f, dest)
            else:
                # debugfs may exit zero even for a missing file, so check the output too.
                subprocess.run(["debugfs", "-R", f"dump /firmware/{name} {name}", str(source)], cwd=stage, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
            if not dest.is_file() or dest.stat().st_size == 0:
                if name in REQUIRED:
                    raise FileNotFoundError(f"missing or empty required firmware: {name}")
                dest.unlink(missing_ok=True)
                continue
            files.append({"name": name, "size": dest.stat().st_size, "sha256": sha256(dest)})
        manifest = {"format": 1, "source_kind": "directory" if source.is_dir() else "ext4-image", "source_image_sha256": None if source.is_dir() else sha256(source), "files": files}
        (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        stage.rename(output)
    print(f"Extracted {len(files)} files to {output}; keep vendor files private unless redistribution is permitted.")


if __name__ == "__main__":
    main()
