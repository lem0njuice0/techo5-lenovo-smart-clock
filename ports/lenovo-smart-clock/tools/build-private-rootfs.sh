#!/bin/bash
# Private, experimental rootfs built from the caller's extracted vendor inputs.
# Requires root (only for a local temporary build tree), Alpine apk.static and mke2fs.
# Does not access or write any device. ARM package scripts run through registered QEMU binfmt.
set -euo pipefail
[ "$(id -u)" -eq 0 ] || { echo 'run in the build VM as root' >&2; exit 1; }
port=${CUBE_PORT:-$(cd "$(dirname "$0")/.." && pwd)}
repo=${CUBE_REPO:-$(cd "$port/../.." && pwd)}
inputs=${CUBE_INPUTS:-$port/inputs}
export CUBE_PORT="$port"
export CUBE_FIRMWARE=${CUBE_FIRMWARE:?set to extracted firmware directory}
export CUBE_AUDIO=${CUBE_AUDIO:?set to extracted 2019 audio table directory}
export CUBE_KERNEL_OUT=${CUBE_KERNEL_OUT:?set to matching runtime kernel build output}
output=${CUBE_OUTPUT:?set a new private output directory}
[ ! -e "$output" ] || { echo 'output already exists' >&2; exit 1; }
[ -f "$repo/bin/echod-cube-arm" ] && [ -x "$inputs/apk.static" ] || { echo 'missing daemon or apk.static' >&2; exit 1; }
[ -f "$port/build/cube-serial-arm" ] || { echo 'missing ARM runtime diagnostic binary' >&2; exit 1; }
python3 - "$repo/bin/echod-cube-arm" "$port/build/cube-serial-arm" <<'PY'
import pathlib,sys
for name in sys.argv[1:]:
    with pathlib.Path(name).open('rb') as f: header=f.read(20)
    if len(header)<20 or header[:6]!=b'\x7fELF\x01\x01' or header[18:20]!=b'\x28\x00':
        raise SystemExit('expected a little-endian ARM32 ELF: '+name)
PY
[ -s "$inputs/alpine-minirootfs-3.24.2-armv7.tar.gz" ] || { echo 'missing Alpine base' >&2; exit 1; }
[ -d "$inputs/models" ] && ls "$inputs"/models/*.tflite >/dev/null 2>&1 || { echo 'missing wake word models' >&2; exit 1; }
tmpbase=${CUBE_TMPROOT:-/tmp}
mkdir -p "$tmpbase"
work=$(mktemp -d "$tmpbase/techo5-cube-rootfs.XXXXXXXX")
trap 'rm -rf -- "$work"' EXIT
stage="$work/in"
mkdir -p "$stage/bin" "$stage/tools" "$stage/inputs" "$stage/overlay/usr/share/techo5/models"
cp -a "$repo/tools/linux/rootfs/." "$stage/overlay/"
cp -a "$port/overlay/." "$stage/overlay/"
cp "$inputs"/models/*.tflite "$inputs"/models/*.json "$stage/overlay/usr/share/techo5/models/"
cp "$inputs/alpine-minirootfs-3.24.2-armv7.tar.gz" "$stage/inputs/"
cp "$repo/bin/echod-cube-arm" "$stage/bin/techo5"
[ ! -f "$repo/bin/fbprobe-cube-arm" ] || cp "$repo/bin/fbprobe-cube-arm" "$stage/bin/fbprobe"
[ ! -f "$repo/bin/audioprobe-cube-arm" ] || cp "$repo/bin/audioprobe-cube-arm" "$stage/bin/audioprobe"
[ ! -f "$repo/bin/btbridge-cube-arm" ] || cp "$repo/bin/btbridge-cube-arm" "$stage/bin/btbridge"
cp "$repo/tools/linux/slotctl" "$repo/tools/linux/bootctl" "$repo/tools/linux/techo5-lib.sh" "$repo/tools/linux/mkrootfs.sh" "$repo/tools/linux/packages-rootfs.txt" "$stage/tools/"
printf '\nwpa_supplicant\npython3\ni2c-tools\niproute2\n' >> "$stage/tools/packages-rootfs.txt"
cp "$port/tools/post-cube-rootfs.sh" "$stage/tools/rootfs-post.sh"
# Git for Windows can check out the upstream shell scripts as CRLF.
find "$stage/tools" -type f -exec sed -i 's/\r$//' {} +
[ -r /proc/sys/fs/binfmt_misc/qemu-arm ] || { echo 'qemu-arm binfmt is required for Alpine package scripts' >&2; exit 1; }
sh "$stage/tools/mkrootfs.sh" -i "$stage" -o "$work/rootfs.tar.gz" -w "$work/build" -V cube-experimental -z Asia/Tokyo -a armv7 -A "$inputs/apk.static" -W alpine
R="$work/build/root"
# Verify the files that prevent an Android partition from being mistaken for this root.
grep -qx 'TECHO5-CUBE-ROOT-V1' "$R/etc/techo5-cube-root"
[ -x "$R/usr/local/bin/techo5" ] && [ -x "$R/etc/techo5-cube/rcS" ]
[ ! -e "$R/usr/local/sbin/slotctl" ] && [ ! -e "$R/usr/local/sbin/bootctl" ]
[ ! -e "$R/lib/firmware/FT6336U_Holitech.bin" ] && [ ! -e "$R/lib/firmware/FT6336U_KD.bin" ]
# This is an ext4 image for offline examination only, not an installer.
mke2fs -q -t ext4 -O ^metadata_csum,^metadata_csum_seed,^orphan_file -L techo5-cube -d "$R" "$work/system_b.img" 512M
mkdir "$output"
cp "$work/system_b.img" "$output/system_b-experimental-DO-NOT-FLASH.img"
cp "$work/rootfs.tar.gz" "$output/rootfs-private.tar.gz"
sha256sum "$output/system_b-experimental-DO-NOT-FLASH.img" "$output/rootfs-private.tar.gz" > "$output/SHA256SUMS"
printf '%s\n' 'Experimental rootfs; no hardware validation. Do not flash or redistribute proprietary inputs.' > "$output/README.txt"
echo "Saved private experimental files in $output"
