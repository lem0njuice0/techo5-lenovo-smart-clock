#!/usr/bin/env python3
"""Update the marked active-slot daemon using a matching USB recovery image."""
import argparse
import base64
import hashlib
from pathlib import Path
import serial


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('port')
    p.add_argument('daemon', type=Path)
    a = p.parse_args()
    digest = hashlib.sha256()
    size = a.daemon.stat().st_size
    with a.daemon.open('rb') as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b''):
            digest.update(chunk)
    with serial.Serial(a.port, 115200, timeout=45, write_timeout=45) as device:
        device.reset_input_buffer()

        def request(line, expected):
            device.write(line.encode('ascii') + b'\n')
            # The startup banner may arrive after the port is opened and before
            # the response to the first command. Ignore at most two old prompts.
            for _ in range(3):
                response = device.read_until(b'> ')
                if expected.encode() in response and response.endswith(b'> '):
                    return
                if b'error:' in response or not response.endswith(b'> '):
                    break
            # Do not print echoed data: chunks can be large.
            tail = response[-500:].decode('ascii', errors='replace')
            raise RuntimeError('unexpected response: ' + tail)

        request('status', 'diagnostic mode')
        request(f'update-begin {size} {digest.hexdigest()}', 'ok begin')
        try:
            sent = 0
            with a.daemon.open('rb') as f:
                for chunk in iter(lambda: f.read(24576), b''):
                    request('update-data ' + base64.b64encode(chunk).decode('ascii'), 'ok data')
                    sent += len(chunk)
                    if sent % (24576 * 100) == 0:
                        print(f'Transferred {sent}/{size} bytes', flush=True)
            request('update-commit', 'ok installed')
        except Exception:
            try:
                request('update-abort', 'ok abort')
            except Exception:
                pass
            raise
    print(f'Installed and verified SHA-256 {digest.hexdigest()}')


if __name__ == '__main__':
    main()
