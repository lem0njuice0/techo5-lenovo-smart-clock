#!/usr/bin/env python3
"""Replace a cube daemon over SSH, retaining the prior binary and device settings.

This is a manual developer update, not a signed release or OTA updater.
"""
import argparse
import hashlib
import ipaddress
from pathlib import Path
import secrets
import subprocess
import time


def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("address", help="clock IP address")
    p.add_argument("key", type=Path, help="SSH private key for the clock")
    p.add_argument("daemon", type=Path, help="Linux ARM32 cube daemon")
    a = p.parse_args()
    address = str(ipaddress.IPv4Address(a.address))
    data = a.daemon.read_bytes()
    if len(data) < 20 or len(data) > 100 * 1024 * 1024 or data[:6] != b"\x7fELF\x01\x01" or data[18:20] != b"\x28\x00":
        p.error("daemon must be a Linux ARM32 little-endian ELF no larger than 100 MiB")
    digest = hashlib.sha256(data).hexdigest()
    base = ["-i", str(a.key), "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=8"]
    host = "root@" + address
    ssh = ["ssh", *base, host]
    candidate = "/usr/local/bin/.techo5-new-" + secrets.token_hex(8)
    # All shell values below are constants, hex digests, or this generated path.
    run(ssh + ["test \"$(cat /etc/techo5-cube-root)\" = TECHO5-CUBE-ROOT-V1 && test -f /usr/local/bin/techo5 && test ! -e " + candidate])
    try:
        run(["scp", "-O", *base, str(a.daemon), host + ":" + candidate])
        command = (
            "set -eu; cd /usr/local/bin; "
            f"test \"$(sha256sum {candidate} | cut -d ' ' -f1)\" = {digest}; "
            "old=$(sha256sum techo5 | cut -d ' ' -f1); "
            "backup=techo5.before-$old; "
            "if [ -e \"$backup\" ]; then test \"$(sha256sum \"$backup\" | cut -d ' ' -f1)\" = \"$old\"; "
            "else ln techo5 \"$backup\"; fi; "
            f"chmod 755 {candidate}; mv {candidate} techo5; sync; "
            "pid=$(pidof techo5); test -n \"$pid\"; kill $pid; echo \"$pid\""
        )
        old_pids = set(run(ssh + [command], capture_output=True, text=True).stdout.split())
    except BaseException:
        subprocess.run(ssh + ["rm -f " + candidate], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        raise
    for _ in range(20):
        time.sleep(1)
        result = subprocess.run(ssh + [f"test \"$(sha256sum /usr/local/bin/techo5 | cut -d ' ' -f1)\" = {digest} && pidof techo5"], capture_output=True, text=True)
        new_pids = set(result.stdout.split())
        if result.returncode == 0 and new_pids and new_pids.isdisjoint(old_pids):
            print("Verified daemon is running; previous binary remains in /usr/local/bin/techo5.before-<sha256>.")
            return
    raise SystemExit("The update was installed but startup could not be verified. Use the retained backup and recovery path; do not assume automatic rollback.")


if __name__ == "__main__":
    main()
