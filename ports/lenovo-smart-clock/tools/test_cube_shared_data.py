import importlib.util
import os
from pathlib import Path
import tempfile
import stat
from types import SimpleNamespace
import unittest
from unittest.mock import patch

MODULE = Path(__file__).resolve().parents[1] / "overlay/usr/local/libexec/cube_shared_data.py"
spec = importlib.util.spec_from_file_location("cube_shared_data", MODULE)
shared = importlib.util.module_from_spec(spec)
spec.loader.exec_module(shared)


class SharedDataTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        self.userdata, self.old, self.bt = root / "userdata", root / "old-data", root / "bt"
        for d in (self.userdata, self.old, self.bt):
            d.mkdir()
        (self.old / "techo5-linux").mkdir()
        (self.old / "techo5-linux" / "wifi").write_text("retained")
        (self.bt / "bond").write_text("retained")
        sync = patch.object(shared.os, "sync", create=True)
        sync.start()
        self.addCleanup(sync.stop)

    def test_migration_preserves_original_and_reuses_shared_data(self):
        data, created = shared.prepare(self.userdata, self.old, self.bt)
        self.assertTrue(created)
        self.assertEqual((data / "techo5-linux/wifi").read_text(), "retained")
        self.assertEqual((data / "techo5-linux/bluetooth/bond").read_text(), "retained")
        self.assertEqual((self.old / "techo5-linux/wifi").read_text(), "retained")
        (self.old / "techo5-linux/wifi").write_text("stale")
        again, created = shared.prepare(self.userdata, self.old, self.bt)
        self.assertFalse(created)
        self.assertEqual(again, data)
        self.assertEqual((data / "techo5-linux/wifi").read_text(), "retained")

    def test_unmarked_existing_directory_is_not_replaced(self):
        existing = self.userdata / "techo5-cube"
        existing.mkdir()
        (existing / "keep").write_text("safe")
        with self.assertRaises((ValueError, FileNotFoundError)):
            shared.prepare(self.userdata, self.old)
        self.assertEqual((existing / "keep").read_text(), "safe")

    def test_failed_copy_does_not_publish_store_or_change_old_data(self):
        with patch.object(shared.shutil, "copytree", side_effect=OSError("disk full")):
            with self.assertRaises(OSError):
                shared.prepare(self.userdata, self.old)
        self.assertFalse((self.userdata / "techo5-cube").exists())
        self.assertEqual((self.old / "techo5-linux/wifi").read_text(), "retained")

    @unittest.skipIf(os.name == "nt", "device numbers are checked on Linux")
    def test_userdata_must_be_unique_and_match_device_number(self):
        sysblock = self.userdata / "sys"
        sysblock.mkdir()
        def add(name, minor):
            part = sysblock / name
            part.mkdir()
            (part / "uevent").write_text(f"PARTNAME=userdata\nDEVNAME={name}\nMAJOR=179\nMINOR={minor}\n")
        add("mmcblk0p29", 29)
        devinfo = SimpleNamespace(st_mode=stat.S_IFBLK | 0o600, st_rdev=os.makedev(179, 29))
        self.assertEqual(shared.find_userdata(sysblock, Path("/dev"), lambda _p: devinfo), Path("/dev/mmcblk0p29"))
        devinfo.st_rdev = os.makedev(179, 30)
        with self.assertRaises(ValueError):
            shared.find_userdata(sysblock, Path("/dev"), lambda _p: devinfo)
        devinfo.st_rdev = os.makedev(179, 29)
        add("mmcblk1p29", 29)
        with self.assertRaises(ValueError):
            shared.find_userdata(sysblock, Path("/dev"), lambda _p: devinfo)

    @unittest.skipIf(os.name == "nt", "symlink creation needs Windows privileges")
    def test_symlink_bluetooth_parent_is_rejected(self):
        (self.old / "techo5-linux/wifi").unlink()
        (self.old / "techo5-linux").rmdir()
        (self.old / "techo5-linux").symlink_to(self.bt, target_is_directory=True)
        with self.assertRaises(ValueError):
            shared.prepare(self.userdata, self.old, self.bt)
        self.assertFalse((self.userdata / "techo5-cube").exists())
        self.assertFalse((self.bt / "bluetooth").exists())


if __name__ == "__main__":
    unittest.main()
