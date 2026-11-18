#!/bin/bash
# Build from a caller-supplied, already patched 2019 Lenovo kernel source.
# This only writes a new output directory and never accesses a device.
set -euo pipefail
: "${CUBE_KERNEL_SRC:?set to the patched 2019 kernel source}"
: "${CUBE_STOCK_CONFIG:?set to the matching stock kernel config}"
: "${CUBE_RUNTIME_INIT:?set to the ARM64 cube_boot init binary}"
: "${CUBE_KERNEL_BUILD:?set to a new output directory}"
: "${CROSS_COMPILE:?set to the ARM64 compiler prefix}"
for path in "$CUBE_KERNEL_SRC" "$CUBE_STOCK_CONFIG" "$CUBE_RUNTIME_INIT" "$CUBE_KERNEL_BUILD"; do
    case "$path" in *[[:space:]]*) echo 'kernel build paths must not contain whitespace' >&2; exit 1;; esac
done
[ -f "$CUBE_KERNEL_SRC/Makefile" ] && [ -f "$CUBE_STOCK_CONFIG" ] && [ -f "$CUBE_RUNTIME_INIT" ]
python3 - "$CUBE_RUNTIME_INIT" <<'PY'
import pathlib, sys
data = pathlib.Path(sys.argv[1]).read_bytes()[:20]
if len(data) != 20 or data[:6] != b'\x7fELF\x02\x01' or data[18:20] != b'\xb7\x00':
    raise SystemExit('expected a little-endian ARM64 ELF init')
PY
[ ! -e "$CUBE_KERNEL_BUILD" ] || { echo 'build directory already exists' >&2; exit 1; }
command -v "${CROSS_COMPILE}gcc" >/dev/null
mkdir -p "$CUBE_KERNEL_BUILD/out"
root=$(cd "$CUBE_KERNEL_BUILD" && pwd)
src=$(cd "$CUBE_KERNEL_SRC" && pwd)
out="$root/out"
install -m 755 "$CUBE_RUNTIME_INIT" "$root/init"
cat > "$root/initramfs.list" <<EOF
dir /dev 0755 0 0
nod /dev/console 0600 0 0 c 5 1
nod /dev/null 0666 0 0 c 1 3
dir /proc 0755 0 0
dir /sys 0755 0 0
dir /sbin 0755 0 0
file /init $root/init 0755 0 0
slink /sbin/multi_init /init 0777 0 0
EOF
cp "$CUBE_STOCK_CONFIG" "$out/.config"
for option in DEVTMPFS DEVTMPFS_MOUNT MTK_MUSB_SW_WITCH_MODE USB_CONFIGFS USB_CONFIGFS_SERIAL USB_CONFIGFS_ACM CONFIGFS_FS SYSVIPC BT BT_BREDR BT_RFCOMM BT_BNEP BT_HIDP BT_LE BT_HCIVHCI; do
    "$src/scripts/config" --file "$out/.config" --enable "$option"
done
"$src/scripts/config" --file "$out/.config" --set-str INITRAMFS_SOURCE "$root/initramfs.list"
export ARCH=arm64 CROSS_COMPILE
make -C "$src" O="$out" olddefconfig
for option in DEVTMPFS MTK_MUSB_SW_WITCH_MODE USB_CONFIGFS_ACM CONFIGFS_FS SYSVIPC BT BT_BREDR BT_LE BT_HCIVHCI; do
    grep -qx "CONFIG_${option}=y" "$out/.config" || { echo "missing CONFIG_${option}" >&2; exit 1; }
done
grep -Fqx "CONFIG_INITRAMFS_SOURCE=\"$root/initramfs.list\"" "$out/.config"
make -C "$src" O="$out" -j"${CUBE_JOBS:-8}" Image.gz modules
make -C "$src" O="$out" mediatek/mt8167s_ref.dtb
gzip -dc "$out/usr/initramfs_data.cpio.gz" | cpio -i --quiet --to-stdout init | cmp - "$root/init"
echo "Built kernel, modules and DTB in $out; embedded init verified."
