import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from audio_init_2019 import EXPECTED, apply_sequence, load_tables


class FakeHardware:
    def __init__(self, fail=False):
        self.calls = []
        self.fail = fail
    def pin(self, n, direction): self.calls.append(("pin", n, direction))
    def start_clock(self): self.calls.append(("clock",))
    def write_pairs(self, bus, address, pairs):
        self.calls.append(("i2c", bus, address, len(pairs)))
        if self.fail: raise RuntimeError("simulated I2C failure")
    def close(self): self.calls.append(("close",))


class AudioInitTests(unittest.TestCase):
    def make_tables(self, root):
        entries=[]
        for name,count in EXPECTED.items():
            data=b"0x00 0x01\n" * count
            (root/name).write_bytes(data)
            entries.append({"name":name,"pairs":count,"sha256":hashlib.sha256(data).hexdigest()})
        (root/"manifest.json").write_text(json.dumps({"files":entries}),encoding="utf-8")

    def test_preflight_rejects_tampering(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d); self.make_tables(root)
            tables=load_tables(root)
            self.assertEqual(len(tables["amp-init.regs"]),1677)
            (root/"mic-init.regs").write_text("0x00 0x01\n",encoding="ascii")
            with self.assertRaises(ValueError): load_tables(root)

    def test_sequence_and_cleanup_on_failure(self):
        tables={name:[(0,1)]*count for name,count in EXPECTED.items()}
        hw=FakeHardware()
        apply_sequence(tables,hw,sleep=lambda _:None)
        self.assertEqual(hw.calls[0:3],[("pin",19,"high"),("clock",),("i2c",2,0x2c,6)])
        self.assertIn(("i2c",1,0x1b,32),hw.calls)
        self.assertEqual(hw.calls[-2:],[("i2c",2,0x2c,1677),("close",)])
        hw=FakeHardware(fail=True)
        with self.assertRaises(RuntimeError): apply_sequence(tables,hw,sleep=lambda _:None)
        self.assertEqual(hw.calls[-1],("close",))


if __name__ == "__main__": unittest.main()
