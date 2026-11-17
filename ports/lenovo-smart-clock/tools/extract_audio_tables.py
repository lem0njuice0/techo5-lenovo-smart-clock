#!/usr/bin/env python3
"""Extract register tables from the 2019 sparrow OEM app's JADX Lenovo.java.

Produces data for inspection only; never accesses or configures hardware.
"""
import argparse
import hashlib
import json
import re
import tempfile
from pathlib import Path

NAMES = {"registers": "amp-init.regs", "muteRegisters": "amp-prelude.regs", "AUDIO_MIC_COMMAND": "mic-init.regs"}
PAIR = re.compile(r"new\s+short\s*\[\s*\]\s*\{\s*(0[xX][0-9a-fA-F]+|\d+)\s*,\s*(0[xX][0-9a-fA-F]+|\d+)\s*\}")


def extract(source):
    out = {}
    for name, filename in NAMES.items():
        declaration = re.compile(r"\bshort\s*\[\s*\]\s*\[\s*\]\s*" + re.escape(name) + r"\s*=\s*\{(.*?)\}\s*;", re.S)
        matches = declaration.findall(source)
        if len(matches) != 1:
            raise ValueError(f"expected exactly one declaration for {name}")
        body = matches[0]
        pairs = [(int(a, 0), int(b, 0)) for a, b in PAIR.findall(body)]
        if not pairs or len(pairs) > 4096 or re.sub(r"[\s,]", "", PAIR.sub("", body)):
            raise ValueError(f"unsupported or empty register data in {name}")
        if any(not (0 <= a <= 255 and 0 <= b <= 255) for a, b in pairs):
            raise ValueError(f"register outside byte range in {name}")
        out[filename] = pairs
    return out


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("java", type=Path, help="Lenovo.java produced by JADX from the owner's OEM APK")
    p.add_argument("output", type=Path, help="new private output directory")
    a = p.parse_args()
    java = a.java.resolve(strict=True)
    output = a.output.resolve()
    if output.exists():
        p.error("output must be a new directory")
    data = java.read_bytes()
    tables = extract(data.decode("utf-8"))
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".cube-audio-", dir=output.parent) as temp:
        stage = Path(temp) / "data"
        stage.mkdir()
        files = []
        for name, pairs in tables.items():
            payload = "".join(f"0x{x:02x} 0x{y:02x}\n" for x, y in pairs).encode("ascii")
            (stage / name).write_bytes(payload)
            files.append({"name": name, "pairs": len(pairs), "sha256": hashlib.sha256(payload).hexdigest()})
        manifest = {"format": 1, "source_java_sha256": hashlib.sha256(data).hexdigest(), "files": files, "status": "inspection-only; hardware initialization sequence is unverified"}
        (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        stage.rename(output)
    print(f"Extracted {len(files)} tables to {output}; do not use on hardware before validation.")


if __name__ == "__main__":
    main()
