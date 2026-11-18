#!/usr/bin/env python3
import importlib.util
import os
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("cube_permissions", Path(__file__).with_name("fix_cube_overlay_permissions.py"))
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


@unittest.skipUnless(os.geteuid() == 0, "ownership repair tests require root")
class PermissionsTest(unittest.TestCase):
    def test_known_paths_are_repaired_without_following_links(self):
        with tempfile.TemporaryDirectory(prefix="cube-permissions-test-") as td:
            d = Path(td) / "known"
            d.mkdir()
            f = d / "config"
            f.write_text("fixture")
            os.chown(d, 1000, 1000)
            os.chmod(d, 0o777)
            os.chown(f, 1000, 1000)
            os.chmod(f, 0o777)
            old = mod.DIRS, mod.FILES, mod.CACHE
            try:
                mod.DIRS, mod.FILES, mod.CACHE = (str(d),), (str(f),), str(d / "missing-cache")
                rootdev = os.stat(td).st_dev
                changes = mod.collect(rootdev)
                self.assertEqual(len(changes), 2)
                mod.repair(rootdev, changes)
                self.assertEqual((os.stat(d).st_uid, os.stat(d).st_mode & 0o777), (0, 0o755))
                self.assertEqual((os.stat(f).st_uid, os.stat(f).st_mode & 0o777), (0, 0o644))
                self.assertEqual(mod.collect(rootdev), [])
                link = Path(td) / "link"
                link.symlink_to(d, target_is_directory=True)
                with self.assertRaises(ValueError):
                    mod.no_symlink_path(str(link / "config"), rootdev)
            finally:
                mod.DIRS, mod.FILES, mod.CACHE = old


if __name__ == "__main__":
    unittest.main()
