#!/usr/bin/env python3
"""Extract verified upstream release helpers for a private cube build."""
import argparse
import hashlib
import os
from pathlib import Path
import tarfile

SHA256 = "0ce31244162481bbc3fb88b3c01ff7f257486191756054b0c54abf44bbe9ff9c"
FILES = {
    "techo5-librespot": "techo5-librespot-arm",
    "techo5-shairport-sync": "techo5-shairport-sync-arm",
    "techo5-nqptp": "techo5-nqptp-arm",
    "techo5-ffmpeg": "techo5-ffmpeg-arm",
    "techo5-aec": "techo5-aec-arm",
    "tailscaled": "tailscaled-arm64",
}
LICENSE = "./usr/share/licenses/tailscale/LICENSE"


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("archive", type=Path, help="upstream rootfs-v1.6.44.tar.gz")
    p.add_argument("output", type=Path, help="new directory for bin/ inputs")
    a = p.parse_args()
    h = hashlib.sha256()
    with a.archive.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    if h.hexdigest() != SHA256:
        p.error("archive SHA-256 does not match the pinned upstream release")
    a.output.mkdir(parents=True, exist_ok=False)
    names = {"./usr/local/bin/" + source: dest for source, dest in FILES.items()}
    names[LICENSE] = "tailscaled-arm64.LICENSE"
    with tarfile.open(a.archive, "r:gz") as archive:
        for source, name in names.items():
            member = archive.getmember(source)
            if not member.isfile() or member.size > 32 * 1024 * 1024:
                raise ValueError("invalid archive member: " + source)
            target = a.output / name
            with target.open("xb") as f:
                f.write(archive.extractfile(member).read())
            os.chmod(target, 0o644 if name.endswith(".LICENSE") else 0o755)
    print("Extracted verified upstream helpers into", a.output)


if __name__ == "__main__":
    main()
