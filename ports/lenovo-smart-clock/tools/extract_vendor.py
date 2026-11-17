#!/usr/bin/env python3
"""Inventory and copy firmware/modules from an owner's mounted stock filesystem.

This is an inspection aid, not a complete vendor tree or a ROM builder.
"""
import argparse
import hashlib
import json
import shutil
from pathlib import Path


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('source', type=Path, help='directory containing an extracted stock partition')
    p.add_argument('output', type=Path, help='new private output directory')
    args = p.parse_args()
    source = args.source.resolve(strict=True)
    output = args.output.resolve()
    if not source.is_dir() or output == source or source in output.parents or output.exists():
        p.error('source must be a directory; output must be a new directory outside source')
    candidates = []
    for path in sorted(source.rglob('*')):
        if path.is_symlink() or not path.is_file():
            continue
        rel = path.relative_to(source)
        parts = tuple(x.lower() for x in rel.parts)
        if path.suffix.lower() == '.ko' or 'firmware' in parts:
            candidates.append((path, rel))
    if not candidates:
        p.error('no .ko files or firmware directories found; check the extracted partition')
    output.mkdir(parents=True)
    records = []
    for path, rel in candidates:
        dest = output / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, dest)
        digest = hashlib.sha256()
        with dest.open('rb') as f:
            for block in iter(lambda: f.read(1024 * 1024), b''):
                digest.update(block)
        records.append({'path': rel.as_posix(), 'size': dest.stat().st_size, 'sha256': digest.hexdigest()})
    (output / 'manifest.json').write_text(json.dumps({'files': records}, indent=2) + '\n', encoding='utf-8')
    print(f'Copied {len(records)} files to {output}; treat this directory as private.')


if __name__ == '__main__':
    main()
