# Installation guide (experimental)

**English** | [日本語](INSTALL.ja.md) · [Feature status](README.md)

This guide is for the **first-generation Lenovo Smart Clock CD-24501F with 2019 firmware**. It walks through the guided installer in order.

> **This is not a finished installer.** An end-to-end test from factory state has not been completed. Obtaining stock data and unlocking are manual, and marking the new slot successful is not automated. No ready-to-flash image is published. Do not use Echo images or installers on this device.

## Overview

| Step | Task | Instructions |
| --- | --- | --- |
| 1 | Gather requirements | This page |
| 2 | Obtain stock data and unlock | [Stock data and unlocking](STOCK-UNLOCK.en.md) |
| 3 | Check the current slot | This page |
| 4 | Run the guided script | This page; then follow its prompts |
| 5 | Check boot-success status | This page; automatic registration is incomplete |

## 1. Requirements

- A CD-24501F, its power supply, and a USB data cable to the PC.
- Matching 2019 stock data: either a **raw image of the whole eMMC user area** or separate **boot, vendor and OEM partition images** from the same clock. The installer does not automatically download an unverified stock ROM.
- An x86_64 Linux build environment. On Windows, use Ubuntu WSL. USB must be visible to whichever environment performs extraction, unlocking or fastboot operations. WSL may require USB reattachment when the device reconnects.
- Root access and Python 3, Go, JADX, `fastboot`, `debugfs`, `mke2fs`, `git`, `tar`, `make`, `gcc`, `gzip`, `cpio`, `bc`, `bison`, `flex` and registered `qemu-arm` in Linux. Install these prerequisites yourself first.
- Sufficient free space for stock data, an approximately 0.9 GB kernel source archive, extracted source, toolchain, build files and the resulting images. Use a case-sensitive Linux filesystem and a work path without whitespace. With WSL, the drive holding its virtual disk also needs free space.

## 2. Obtain stock data and unlock

Follow [stock data and unlocking](STOCK-UNLOCK.en.md) to obtain the required data before unlocking the bootloader and AVB. The script does **not** automatically read the clock, acquire a complete stock ROM or perform unlocking. Unlocking may erase data. Stock data can contain device-specific information; do not publish it.

## 3. Check the current slot

1. Connect the clock to the PC with a USB data cable.
2. Keep USB connected and unplug the clock's power.
3. Hold volume up while reconnecting power, and keep holding for 20–30 seconds. The display may stay dark.
4. Run the following in the Linux/WSL environment that will run the script. If several fastboot devices are connected, identify the correct one and specify `-s DEVICE_SERIAL` in each command.

```sh
fastboot devices
fastboot getvar current-slot
fastboot getvar unlocked
fastboot getvar at-vboot-state
```

Check for `unlocked: yes` and `avb-locked: 0`. The target must be the **inactive slot**. For example, if B is current, select A. If A is current, change `--slot a` below to `--slot b`. Preflight stops on an active target or other unmet conditions.

<a id="single-entry-experimental-pipeline"></a>

## 4. Start the guided installation

First, clone the repository and enter its directory. These commands work in Linux or Windows PowerShell with Git installed:

```sh
git clone https://github.com/lem0njuice0/techo5-lenovo-smart-clock.git
cd techo5-lenovo-smart-clock
```

Run the following from that directory. The examples assume **B is current, A is the target**, and the stock inputs also come from A. Change the paths and slots to match your device. `--work` must be a new, private directory.

### Linux

Run as root:

```sh
sudo python3 ports/lenovo-smart-clock/tools/cube_pipeline.py build-guided \
  --work /private/cube-run --slot a \
  --stock-image /private/full-user.img --source-slot a
```

If your stock data is in three separate files, replace `--stock-image ... --source-slot a` with `--boot /private/boot.img --vendor /private/vendor.img --oem /private/oem.img`.

### Windows (PowerShell + WSL)

Open PowerShell at the repository root. This example assumes the WSL distribution is named `Ubuntu`. Paths inside **`-PipelineArgs` are WSL paths**; `/mnt/d/...` refers to Windows drive D.

```powershell
.\ports\lenovo-smart-clock\tools\cube_pipeline.ps1 -Distro Ubuntu -PipelineArgs @('build-guided','--work','/home/user/cube-run','--slot','a','--stock-image','/mnt/d/private/full-user.img','--source-slot','a')
```

The wrapper runs the Linux script as WSL root. You must install the Linux prerequisites and attach USB to WSL. End-to-end testing on a new environment, including Windows, is still incomplete.

### What happens next

1. Choose the script language. Non-interactive runs default to English; pass `--language ja` or `--language en` to choose explicitly. External tool output may remain in English.
2. The script extracts stock data, downloads and verifies public inputs, and builds the kernel and images. It does not write to the clock during this stage.
3. It automatically detects one fastboot device and runs preflight. If multiple devices appear, it stops; rerun with `--serial DEVICE_SERIAL`.
4. After confirmation, it **overwrites the selected inactive system partition** and trial-boots it. It does not ask for display, touch or connectivity testing.
5. When prompted to return to fastboot, repeat step 3's button procedure. When the same clock is detected again, the script verifies the image, writes boot and reboots. The default wait is 600 seconds.

The script does not determine whether features worked during the trial. To resume installation with an already built work directory, use `guided` instead of `build-guided` and specify `--work` and `--slot`.

## 5. After reboot

The script does **not** automatically mark the slot successful. Repeated restarts without registration can cause fallback to the previous slot when attempts are exhausted. General-purpose instructions for safe registration and full recovery remain incomplete. Shared settings and production OTA updates are not automatically enabled by this procedure.

<a id="assembling-from-a-shared-template"></a>

## Reference: shared-template workflow

A common rootfs without stock firmware, audio tables or kernel modules can be assembled with the owner's data. A full-size assembled image passed offline checks, but device boot and distribution terms remain unverified. **No distributable template or prebuilt package is currently published.**

## Troubleshooting

- **No fastboot device:** check the data cable, drivers and button procedure. With WSL, also check the USB attachment.
- **Multiple devices:** add `--serial DEVICE_SERIAL` to select the target.
- **No stock files or unable to unlock:** installation cannot proceed. See [stock data and unlocking](STOCK-UNLOCK.en.md).
- **Build or boot fails:** record the error and stage. Do not publish private keys, credentials, Wi-Fi passwords or stock images.
