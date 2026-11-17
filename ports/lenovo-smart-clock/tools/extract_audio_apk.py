#!/usr/bin/env python3
"""Run JADX locally and extract 2019 Lenovo audio register tables from an OEM APK.

For Linux/macOS. The APK and generated tables remain private and are not executed.
"""
import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


def sha256(p):
    h = hashlib.sha256()
    with p.open("rb") as f:
        for b in iter(lambda: f.read(1024 * 1024), b""):
            h.update(b)
    return h.hexdigest()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("apk", type=Path)
    p.add_argument("output", type=Path, help="new private output directory")
    p.add_argument("--jadx", default="jadx", help="path to the JADX command")
    a = p.parse_args()
    if os.name == "nt":
        p.error("run this helper in WSL/Linux or macOS; see README for the two-step Windows method")
    apk = a.apk.resolve(strict=True)
    output = a.output.resolve()
    if not apk.is_file() or output.exists():
        p.error("APK must be a file and output must be a new directory")
    jadx = shutil.which(a.jadx)
    if not jadx:
        p.error("JADX was not found; install it or pass --jadx")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".cube-decompile-", dir=output.parent) as tmp:
        work = Path(tmp)
        subprocess.run([jadx, "--no-res", "--threads-count", "2", "-d", str(work / "decompiled"), str(apk)], check=True)
        java = work / "decompiled/sources/com/google/android/things/sparrow/oemcustom/Lenovo.java"
        if not java.is_file():
            raise FileNotFoundError("JADX did not produce the expected Lenovo.java; check device and firmware")
        stage = work / "tables"
        subprocess.run([sys.executable, str(Path(__file__).with_name("extract_audio_tables.py")), str(java), str(stage)], check=True)
        manifest_path = stage / "manifest.json"
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        manifest["source_apk_sha256"] = sha256(apk)
        manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        stage.rename(output)
    print(f"Audio tables extracted to {output}; hardware use remains unverified.")


if __name__ == "__main__":
    main()
