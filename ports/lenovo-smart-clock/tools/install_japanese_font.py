#!/usr/bin/env python3
"""Stage the upstream pinned Japanese font and its license in a cube rootfs."""
import argparse
import hashlib
import io
import os
from pathlib import Path
import tarfile
import urllib.request

URL = "https://github.com/HuskerMinion/techo5/releases/download/v1.6.8/techo5-noto-sans-jp.tar.gz"
SHA256 = "85fac8fd07ca165316df85c75c251d68dd3f1519a94e127046be66b2934f4a42"
SIZE = 632659
FILES = {
    "NotoSansJP-Regular.ttf": "data/misc/techo5/fonts/zz-noto-sans-jp.ttf",
    "OFL.txt": "data/misc/techo5/licenses/noto-sans-jp-OFL.txt",
}


def install(root, data):
    if len(data) != SIZE or hashlib.sha256(data).hexdigest() != SHA256:
        raise ValueError("Japanese font archive size or SHA-256 mismatch")
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        members = archive.getmembers()
        if len(members) != len(FILES) or {m.name for m in members} != set(FILES):
            raise ValueError("unexpected Japanese font archive members")
        for member in members:
            if not member.isfile() or member.size > 4 * 1024 * 1024:
                raise ValueError("invalid Japanese font archive member")
            target = root / FILES[member.name]
            target.parent.mkdir(parents=True, exist_ok=True)
            if target.exists() or target.is_symlink():
                raise FileExistsError(target)
            with target.open("xb") as output:
                output.write(archive.extractfile(member).read())
            os.chmod(target, 0o644)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path)
    parser.add_argument("--archive", type=Path, help="use a previously downloaded archive")
    args = parser.parse_args()
    if args.archive:
        data = args.archive.read_bytes()
    else:
        with urllib.request.urlopen(URL, timeout=60) as response:
            data = response.read(SIZE + 1)
    install(args.root, data)


if __name__ == "__main__":
    main()
