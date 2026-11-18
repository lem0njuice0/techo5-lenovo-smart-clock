import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

import install_cube_fastboot as installer


class InstallerTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        self.boot = root / "boot.img"
        self.boot.write_bytes(b"boot")
        self.system = root / "system.img"
        data = bytearray(2048)
        data[1080:1082] = b"\x53\xef"
        self.system.write_bytes(data)
        self.report = root / "report.json"
        self.report.write_text(json.dumps({"image_sha256": hashlib.sha256(data).hexdigest(), "checks": {"root_marker": True}}))
        self.args = ["installer", "--serial", "TEST", "--slot", "a", "--boot", str(self.boot), "--boot-sha256", hashlib.sha256(b"boot").hexdigest(), "--system", str(self.system), "--root-report", str(self.report)]
        self.values = {"product": "mt8167s", "unlocked": "yes", "current-slot": "b", "slot-successful:b": "yes", "slot-unbootable:b": "no", "partition-size:boot_a": "0x1000", "partition-size:system_a": "0x800"}

    def run_installer(self, extra, calls):
        def call(_serial, *args):
            calls.append(args)
            return "(bootloader) avb-locked: 0\n"
        with patch.object(sys, "argv", self.args + extra), patch.object(installer, "parse_boot", return_value={"cmdline": "techo5.cube.root=system_a"}), patch.object(installer, "variable", side_effect=lambda _s, n: self.values[n]), patch.object(installer, "call", side_effect=call):
            installer.main()

    def test_trial_writes_inactive_a_only(self):
        calls = []
        self.run_installer(["--action", "trial"], calls)
        self.assertTrue(any(c[:2] == ("flash", "system_a") for c in calls))
        self.assertFalse(any(c[:2] in (("flash", "system_b"), ("flash", "boot_b")) for c in calls))
        self.assertTrue(any(c[0] == "boot" for c in calls))

    def test_refuses_targeting_active_slot(self):
        calls = []
        self.values["current-slot"] = "a"
        with self.assertRaises(SystemExit):
            self.run_installer(["--action", "trial"], calls)
        self.assertFalse(any(c[0] == "flash" for c in calls))

    def test_refuses_failed_rootfs_report(self):
        calls = []
        self.report.write_text(json.dumps({"checks": {"root_marker": False}}))
        with self.assertRaises(SystemExit):
            self.run_installer(["--action", "trial"], calls)
        self.assertEqual(calls, [])


if __name__ == "__main__":
    unittest.main()
