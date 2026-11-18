#!/usr/bin/env python3
"""Read-only structural checks on a private experimental Smart Clock ext4 rootfs.

Passing checks does not establish boot, peripheral support, or recovery safety.
"""
import argparse
import hashlib
import json
import shutil
import subprocess
import tempfile
from pathlib import Path

FIRMWARE = ("EEPROM_MT7668.bin", "TxPwrLimit_MT76x8.dat", "WIFI_RAM_CODE2_SDIO_MT7668.bin", "WIFI_RAM_CODE_MT7668.bin", "mt7668_patch_e2_hdr.bin", "wifi.cfg")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("image", type=Path)
    p.add_argument("daemon", type=Path)
    p.add_argument("firmware", type=Path)
    p.add_argument("--serial", type=Path, help="expected runtime diagnostic binary")
    p.add_argument("--expect-test-audio", action="store_true", help="expect experimental audio opt-in marker")
    p.add_argument("--expect-test-bluetooth", action="store_true", help="expect Bluetooth startup opt-in marker")
    p.add_argument("--expect-japanese-font", action="store_true", help="check the pinned Japanese font and license")
    p.add_argument("--helpers", type=Path, help="directory of verified upstream helper inputs")
    p.add_argument("--audio", type=Path, help="expected private audio-table directory")
    a = p.parse_args()
    if not shutil.which("debugfs"):
        p.error("debugfs is required")
    image = a.image.resolve(strict=True)
    manifest = json.loads((a.firmware / "manifest.json").read_text(encoding="utf-8"))
    fw = {e["name"]: e["sha256"] for e in manifest["files"]}
    with tempfile.TemporaryDirectory(prefix="cube-verify-") as d:
        tmp = Path(d)
        def read(path):
            out = tmp / "payload"
            out.unlink(missing_ok=True)
            subprocess.run(["debugfs", "-R", f"dump {path} payload", str(image)], cwd=tmp, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
            return out.read_bytes() if out.is_file() else None
        def sha(b): return hashlib.sha256(b).hexdigest() if b is not None else None
        marker = read("/etc/techo5-cube-root")
        inittab = read("/etc/inittab") or b""
        daemon = read("/usr/local/bin/techo5")
        serial = read("/usr/local/sbin/cube-serial")
        checks = {
            "root_marker": marker == b"TECHO5-CUBE-ROOT-V1\n",
            "cube_startup": b"/etc/techo5-cube/rcS" in inittab and b"/usr/local/sbin/cube-run" in inittab,
            "runtime_diagnostic": b"/usr/local/sbin/cube-serial" in inittab and serial is not None and serial[:5] == b"\x7fELF\x01" and serial[18:20] == b"\x28\x00",
            "no_echo_slotctl": read("/usr/local/sbin/slotctl") is None and read("/usr/local/sbin/bootctl") is None,
            "touch_firmware_absent": read("/lib/firmware/FT6336U_Holitech.bin") is None and read("/lib/firmware/FT6336U_KD.bin") is None,
            "audio_marker_matches_requested_mode": (read("/etc/techo5-cube/enable-unverified-audio") is not None) == a.expect_test_audio,
            "bluetooth_marker_matches_requested_mode": (read("/etc/techo5-cube/enable-unverified-bluetooth") is not None) == a.expect_test_bluetooth,
            "daemon_matches": sha(daemon) == sha(a.daemon.read_bytes()),
            "daemon_arm32": daemon is not None and daemon[:5] == b"\x7fELF\x01" and daemon[18:20] == b"\x28\x00",
            "wifi_module_present": bool(read("/lib/modules/cube/wlan_drv_gen4_mt7668.ko")),
            "wifi_tools_present": bool(read("/sbin/wpa_supplicant")) and bool(read("/sbin/wpa_cli")) and bool(read("/etc/techo5/udhcpc.sh")),
            "bluetooth_module_present": bool(read("/lib/modules/cube/btmtksdio.ko")),
            "wake_model_present": bool(read("/usr/share/techo5/models/okay_nabu.tflite")),
        }
        if a.serial is not None:
            checks["runtime_diagnostic_matches"] = sha(serial) == sha(a.serial.read_bytes())
        if a.expect_test_bluetooth:
            checks["bluetooth_bridge_present"] = bool(read("/usr/local/bin/btbridge"))
        if a.expect_japanese_font:
            checks["japanese_font_matches"] = sha(read("/data/misc/techo5/fonts/zz-noto-sans-jp.ttf")) == "d5f4601b405aa7db4bbbf9dfe6c21aae00f104a0d74ac1e69b2a219194650c6a"
            checks["japanese_font_license_present"] = bool(read("/data/misc/techo5/licenses/noto-sans-jp-OFL.txt"))
        if a.helpers is not None:
            for name in ("techo5-librespot", "techo5-shairport-sync", "techo5-nqptp", "techo5-ffmpeg", "techo5-aec"):
                checks["helper:" + name] = sha(read("/usr/local/bin/" + name)) == sha((a.helpers / (name + "-arm")).read_bytes())
            checks["helper:tailscaled"] = sha(read("/usr/local/bin/tailscaled")) == sha((a.helpers / "tailscaled-arm64").read_bytes())
        if a.audio is not None:
            manifest_audio = json.loads((a.audio / "manifest.json").read_text(encoding="utf-8"))
            table_hashes = {e["name"]: e["sha256"] for e in manifest_audio["files"]}
            for name in ("amp-init.regs", "amp-prelude.regs", "mic-init.regs"):
                checks["audio:" + name] = name in table_hashes and sha(read("/etc/techo5-cube/audio/" + name)) == table_hashes[name]
            checks["audio_init_program_present"] = bool(read("/usr/local/sbin/cube-audio-init.py"))
            checks["audio_tools_present"] = bool(read("/usr/sbin/i2ctransfer")) and bool(read("/usr/bin/aplay"))
        for name in FIRMWARE:
            checks["firmware:" + name] = name in fw and sha(read("/lib/firmware/" + name)) == fw[name]
        print(json.dumps({"checks": checks, "note": "structural checks only; hardware and startup are unverified"}, indent=2))
        if not all(checks.values()):
            raise SystemExit(1)


if __name__ == "__main__":
    main()
