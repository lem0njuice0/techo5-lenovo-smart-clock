#!/usr/bin/env python3
"""Experimental 2019 CD-24501F audio initialization, offline plan by default.

--apply accesses GPIO, I2C and ALSA on the target. This sequence and pin mapping
have not been validated on the user's clock. It is not an installer.
"""
import argparse
import hashlib
import json
import re
import shutil
import subprocess
import time
from pathlib import Path

EXPECTED = {"amp-init.regs": 1677, "amp-prelude.regs": 6, "mic-init.regs": 32}
PAIR = re.compile(r"0x([0-9a-fA-F]{2}) 0x([0-9a-fA-F]{2})\n")


def load_tables(directory):
    manifest = json.loads((directory / "manifest.json").read_text(encoding="utf-8"))
    entries = {e["name"]: e for e in manifest.get("files", [])}
    if len(manifest.get("files", [])) != len(EXPECTED) or set(entries) != set(EXPECTED):
        raise ValueError("unexpected audio table manifest")
    result = {}
    for name, count in EXPECTED.items():
        data = (directory / name).read_bytes()
        e = entries[name]
        if hashlib.sha256(data).hexdigest() != e.get("sha256") or e.get("pairs") != count:
            raise ValueError(f"hash or count mismatch: {name}")
        text = data.decode("ascii")
        matches = PAIR.findall(text)
        if len(matches) != count or PAIR.sub("", text):
            raise ValueError(f"invalid table: {name}")
        result[name] = [(int(a, 16), int(b, 16)) for a, b in matches]
    return result


class Hardware:
    def __init__(self, gpio_base):
        self.base = gpio_base
        self.audio = None
        for binary in ("aplay", "i2ctransfer"):
            if not shutil.which(binary):
                raise RuntimeError(f"missing {binary}")

    def pin(self, offset, direction):
        number = self.base + offset
        gpio = Path("/sys/class/gpio")
        path = gpio / f"gpio{number}"
        if not path.exists():
            (gpio / "export").write_text(str(number), encoding="ascii")
            for _ in range(100):
                if path.exists():
                    break
                time.sleep(.01)
        (path / "direction").write_text(direction, encoding="ascii")

    def write_pairs(self, bus, address, pairs):
        for start in range(0, len(pairs), 40):
            args = ["i2ctransfer", "-y", str(bus)]
            for reg, val in pairs[start:start + 40]:
                args.extend((f"w2@0x{address:02x}", f"0x{reg:02x}", f"0x{val:02x}"))
            subprocess.run(args, check=True)

    def start_clock(self):
        self.audio = subprocess.Popen(["aplay", "-q", "-D", "hw:0,0", "-f", "S16_LE", "-c", "2", "-r", "48000", "/dev/zero"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        time.sleep(1)
        if self.audio.poll() is not None:
            raise RuntimeError("silence playback did not start")

    def close(self):
        if self.audio is not None and self.audio.poll() is None:
            self.audio.terminate()
            try:
                self.audio.wait(timeout=3)
            except subprocess.TimeoutExpired:
                self.audio.kill()
                self.audio.wait()


def apply_sequence(tables, hw, sleep=time.sleep):
    try:
        # GPIO 19 enables the amplifier; GPIO 24 resets the microphone ADC.
        hw.pin(19, "high")
        hw.start_clock()
        hw.write_pairs(2, 0x2c, tables["amp-prelude.regs"])
        hw.pin(24, "high")
        hw.pin(24, "in")
        sleep(.02)
        hw.pin(24, "high")
        sleep(.05)
        hw.write_pairs(1, 0x1b, tables["mic-init.regs"])
        hw.write_pairs(2, 0x2c, [(0, 0), (127, 0), (3, 3)])
        sleep(.01)
        hw.write_pairs(2, 0x2c, tables["amp-init.regs"])
    finally:
        hw.close()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("tables", type=Path, help="private output from extract_audio_apk.py")
    p.add_argument("--apply", action="store_true", help="apply to hardware; unverified")
    p.add_argument("--gpio-base", type=int, default=387, help="sysfs base; reference board uses 387")
    a = p.parse_args()
    tables = load_tables(a.tables)
    if not 0 <= a.gpio_base <= 4096:
        p.error("invalid GPIO base")
    if not a.apply:
        print("Validated 2019 audio tables; no device access. Hardware sequence remains unverified.")
        return
    apply_sequence(tables, Hardware(a.gpio_base))
    print("Audio initialization commands completed; actual output must be verified on the device.")


if __name__ == "__main__":
    main()
