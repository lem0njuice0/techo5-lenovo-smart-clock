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
install -m 644 "$CUBE_KERNEL_OUT/drivers/misc/mediatek/connectivity/wlan/gen4-mt7668/wlan_drv_gen4_mt7668.ko" "$R/lib/modules/cube/"
install -m 644 "$CUBE_KERNEL_OUT/drivers/misc/mediatek/connectivity/bt/mt76xx/sdio/btmtksdio.ko" "$R/lib/modules/cube/"
chmod 755 "$R/etc/techo5-cube/rcS" "$R/usr/local/sbin/cube-run"
# The Echo boot and slot helpers assume a different device's partition layout.
rm -f "$R/usr/local/sbin/slotctl" "$R/usr/local/sbin/bootctl" "$R/usr/local/sbin/techo5-console"
