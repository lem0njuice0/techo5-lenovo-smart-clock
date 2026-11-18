#!/bin/sh
# Called by TECHO5 mkrootfs on the builder, not on the clock.
set -eu
R=${1:?rootfs path required}
: "${CUBE_PORT:?}" "${CUBE_FIRMWARE:?}" "${CUBE_AUDIO:?}" "${CUBE_KERNEL_OUT:?}"
CUBE_SERIAL=${CUBE_SERIAL:-$CUBE_PORT/build/cube-serial-arm}
[ -f "$CUBE_SERIAL" ] || { echo 'missing cube-serial ARM binary' >&2; exit 1; }
# Recheck the extraction manifest before staging proprietary inputs.
python3 - "$CUBE_FIRMWARE" <<'PY'
import hashlib,json,pathlib,sys
p=pathlib.Path(sys.argv[1]); m=json.loads((p/'manifest.json').read_text())
required={'EEPROM_MT7668.bin','TxPwrLimit_MT76x8.dat','WIFI_RAM_CODE2_SDIO_MT7668.bin','WIFI_RAM_CODE_MT7668.bin','mt7668_patch_e2_hdr.bin','wifi.cfg'}
entries=m['files']
if len({e['name'] for e in entries}) != len(entries) or not required.issubset({e['name'] for e in entries}): raise SystemExit('invalid firmware manifest')
allowed=required|{'rgx.fw.signed.22.40.54.30'}
for e in entries:
    name=e['name']
    if name not in allowed or pathlib.Path(name).name!=name: raise SystemExit('unexpected firmware name')
    f=p/name
    if f.is_symlink() or not f.is_file() or hashlib.sha256(f.read_bytes()).hexdigest()!=e['sha256']: raise SystemExit('firmware hash mismatch: '+name)
PY
python3 "$CUBE_PORT/tools/audio_init_2019.py" "$CUBE_AUDIO" >/dev/null
# Media titles and notifications may need Japanese even when the interface is
# set to English. Install before mkrootfs packs either output.
if [ -n "${CUBE_CJK_ARCHIVE:-}" ]; then
    python3 "$CUBE_PORT/tools/install_japanese_font.py" "$R" --archive "$CUBE_CJK_ARCHIVE"
else
    python3 "$CUBE_PORT/tools/install_japanese_font.py" "$R"
fi
mkdir -p "$R/etc/techo5-cube/audio" "$R/lib/firmware" "$R/lib/modules/cube" "$R/usr/local/sbin"
printf 'TECHO5-CUBE-ROOT-V1\n' > "$R/etc/techo5-cube-root"
for name in EEPROM_MT7668.bin TxPwrLimit_MT76x8.dat WIFI_RAM_CODE2_SDIO_MT7668.bin WIFI_RAM_CODE_MT7668.bin mt7668_patch_e2_hdr.bin wifi.cfg; do
    [ -f "$CUBE_FIRMWARE/$name" ] || continue
    install -m 644 "$CUBE_FIRMWARE/$name" "$R/lib/firmware/$name"
done
for file in amp-init.regs amp-prelude.regs mic-init.regs manifest.json; do install -m 600 "$CUBE_AUDIO/$file" "$R/etc/techo5-cube/audio/$file"; done
install -m 755 "$CUBE_PORT/tools/audio_init_2019.py" "$R/usr/local/sbin/cube-audio-init.py"
install -m 755 "$CUBE_SERIAL" "$R/usr/local/sbin/cube-serial"
if [ "${CUBE_TEST_AUDIO:-0}" = 1 ]; then
    : > "$R/etc/techo5-cube/enable-unverified-audio"
fi
if [ "${CUBE_BLUETOOTH:-0}" = 1 ]; then
    [ -x "$R/usr/local/bin/btbridge" ] && [ -x "$R/usr/bin/bluealsa" ] && [ -x "$R/usr/lib/bluetooth/bluetoothd" ] || { echo 'Bluetooth components are missing' >&2; exit 1; }
    : > "$R/etc/techo5-cube/enable-unverified-bluetooth"
fi
if [ "${CUBE_SHARED_DATA:-0}" = 1 ]; then
    [ -f "$R/usr/local/libexec/cube_shared_data.py" ] && [ -f "$R/usr/local/sbin/cube-shared-mount" ] || { echo 'shared-data components are missing' >&2; exit 1; }
    printf 'TECHO5-CUBE-SHARED-ENABLE-V1\n' > "$R/etc/techo5-cube/enable-shared-data"
fi
if [ -n "${CUBE_UPDATE_PUB:-}" ]; then
    [ "${CUBE_SHARED_DATA:-0}" = 1 ] || { echo 'experimental updates require shared data' >&2; exit 1; }
    python3 - "$CUBE_UPDATE_PUB" "$CUBE_SERIAL" <<'PY'
import os,pathlib,sys
public=pathlib.Path(sys.argv[1]).read_bytes()
serial=pathlib.Path(sys.argv[2]).read_bytes()
if len(public)!=32 or any(x not in serial for x in (b'ota-fetch',b'ota-check',b'ota-update',b'ota-stage',b'ota-monitor',b'ota-watch')):
    raise SystemExit('expected a raw Ed25519 public key and experimental update-enabled serial binary')
if (b'CUBE-OTA-FAULT-TEST-DO-NOT-RELEASE' in serial) != (os.environ.get('CUBE_UPDATE_FAULT_TEST')=='1'):
    raise SystemExit('fault injection binary must be explicitly selected and never released')
PY
    install -m 644 "$CUBE_UPDATE_PUB" "$R/etc/techo5-cube/update-signing.pub"
fi
if [ "${CUBE_UPDATE_WATCH:-0}" = 1 ]; then
    [ -n "${CUBE_UPDATE_PUB:-}" ] && [ -f "$R/usr/local/sbin/cube-update-watch" ] || { echo 'update watcher requires an experimental public key and watcher' >&2; exit 1; }
    printf '%s\n' '::respawn:/bin/sh /usr/local/sbin/cube-update-watch' >> "$R/etc/inittab"
fi
install -m 644 "$CUBE_KERNEL_OUT/drivers/misc/mediatek/connectivity/wlan/gen4-mt7668/wlan_drv_gen4_mt7668.ko" "$R/lib/modules/cube/"
install -m 644 "$CUBE_KERNEL_OUT/drivers/misc/mediatek/connectivity/bt/mt76xx/sdio/btmtksdio.ko" "$R/lib/modules/cube/"
chmod 755 "$R/etc/techo5-cube/rcS" "$R/usr/local/sbin/cube-run"
# The Echo boot and slot helpers assume a different device's partition layout.
rm -f "$R/usr/local/sbin/slotctl" "$R/usr/local/sbin/bootctl" "$R/usr/local/sbin/techo5-console"
