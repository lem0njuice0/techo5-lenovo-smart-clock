# First-generation Lenovo Smart Clock: experimental installation guide

**English** | [日本語](INSTALL.ja.md) · [Technical notes](README.md)

This guide describes the development workflow used on the CD-24501F (`cube`, 2019 version). **It is not a complete end-user installation guide.** Instructions for obtaining stock partitions, unlocking the bootloader, rebuilding the patched kernel, marking a boot successful, and full recovery are not yet complete. Inputs that are not supplied by this project are identified below. Do not use these instructions on another Lenovo Smart Clock model or an Echo.

Display, touch, Wi-Fi, and other features were tested on two devices, but testing does not cover every feature, stock firmware version, or recovery method. A successful trial boot does not automatically enable persistent shared settings or updates.

## Start here: what is available?

| Stage | Current status | Instructions |
| --- | --- | --- |
| Obtain stock data and unlock the device | **Incomplete**: you must provide the data and unlock the device yourself | [1. Requirements](#1-requirements) |
| Extract required stock files | Extraction scripts are available | [2. Extract stock inputs](#2-extract-stock-inputs) |
| Build the kernel and userspace | Obtaining patches and configuration and reproducing the kernel build are incomplete | [3. Prepare build inputs](#3-prepare-build-inputs) |
| Create and verify images | Experimental tools can be used with prepared inputs | [4. Create rootfs and boot images](#4-create-rootfs-and-boot-images) |
| Trial boot and install | Overwrites the inactive slot; general-purpose boot-success instructions are incomplete | [5. Test and install with fastboot](#5-test-and-install-with-fastboot) |

**This guide alone cannot complete an installation on a factory-state device.** If you cannot obtain the necessary inputs or establish unlocking and recovery, do not proceed to flashing; wait until the missing instructions are available.

## Unlocking and stock data

See [obtaining stock data and unlocking](STOCK-UNLOCK.en.md) for the sequence and command examples.

For background, see the [XDA Smart Clock bootloader / AVB unlock discussion](https://xdaforums.com/t/lenovo-smart-clock-bootloader-avb-unlock-firmware-region-changer-kernel-source.4130295/) and the [independent first-generation unlock notes](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/01-unlocking.md). The XDA page could not be retrieved when these instructions were written, so its current contents, files and suitability have not been independently verified. Unlocking can erase data; the tools here do not unlock a device or supply unlock credentials.

No authorized download of the complete stock ROM has been established. The pipeline accepts either an owner's raw, complete eMMC user-area image with a valid GPT or separate boot, vendor and OEM images from their device. It does not read the device to acquire them. [Device extraction notes](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/02-backup.md) describe a separate BootROM method; it has not been validated by this project for all firmware versions. Extracted data may be device-specific and must be kept private.

## Single-entry experimental pipeline

### Enter fastboot

1. Connect the clock to the PC with a USB data cable.
2. Leave USB connected and unplug the clock's power.
3. Hold volume up while reconnecting power, and keep holding for 20–30 seconds.
4. The screen may remain dark. Run `fastboot devices` on the PC and check for the intended serial. When using WSL on Windows, attach the fastboot USB device to WSL.

The guided flow shows these instructions and waits for the selected serial. If it is not detected, check the data cable, drivers and WSL USB connection.

### Language selection

In an interactive terminal, the script first asks for `1` (English) or `2` (Japanese). Pass `--language en` or `--language ja` to skip the prompt. Non-interactive runs default to English. Output from external tools such as compilers and downloaders may remain in English.

`cube_pipeline.py` is a Linux entry point for extraction from an owner's stock image, public input downloads, kernel and image builds, and separate fastboot inspect/trial/install stages. It calls the existing tools; **the complete workflow has not been tested end to end**. It cannot unlock the device or mark the installed slot successful. The prerequisites and limitations in this guide still apply.

For example, on x86_64 Linux with the tools and WSL-compatible environment described below, as root, with paths replaced:

```sh
python3 ports/lenovo-smart-clock/tools/cube_pipeline.py build --work /private/cube-run --slot a --stock-image /private/full-user.img --source-slot a
```

Instead of `--stock-image`, provide `--boot /private/boot.img --vendor /private/vendor.img --oem /private/oem.img`. The raw whole-disk path extracts only boot, vendor and OEM partitions to the private work directory after checking both GPT copies. It does not make a full backup. To skip rebuilding the kernel, provide `--kernel-out`; use `--lenovo-archive` for an already downloaded hash-matching source archive.

For an interactive flow after building, replace `build` with `build-guided` and add `--serial DEVICE_SERIAL`. For an existing bundle, use `guided --work /private/cube-run --slot a --serial DEVICE_SERIAL`. It waits for the specified fastboot serial, runs preflight, and asks before the trial write. After the trial, it asks you to return to fastboot. When the same serial reappears, it checks that the bundle is unchanged, writes boot and reboots. It does not automatically test or ask whether display, touch or connectivity worked, so it does not establish that the trial was functional. The default wait is 600 seconds; use `--wait-timeout` to change it.

On Windows with WSL, you may need to reattach USB to WSL each time the clock reconnects. Automatic detection sees only fastboot devices available inside WSL. The guided flow has not yet been tested end to end on hardware.

After building, connect the unlocked clock in fastboot and use the same `--work` and `--slot` with `inspect --serial DEVICE_SERIAL`. `trial --serial DEVICE_SERIAL` **writes the selected inactive system partition** and boots temporarily. Check the trial, return to fastboot, and use `install --serial DEVICE_SERIAL --trial-confirmed` only after testing the same bundle. Reboot and check boot-success status separately; automatic success registration and complete recovery remain unfinished.

On Windows, run the PowerShell wrapper through a prepared Ubuntu WSL environment. All paths in `-PipelineArgs` are WSL paths, not Windows paths. The wrapper runs the pipeline as WSL root; install the documented Linux dependencies and attach the clock's fastboot USB device to WSL (for example, using usbipd-win) before fastboot actions. This wrapper and full workflow are experimental and have not been verified end to end on Windows.

```powershell
.\ports\lenovo-smart-clock\tools\cube_pipeline.ps1 -Distro Ubuntu -PipelineArgs @('build','--work','/home/user/cube-run','--slot','a','--stock-image','/mnt/d/private/full-user.img','--source-slot','a')
```

## Experimental: automated preparation and build

`prepare_cube_inputs.py` downloads public TECHO5 helpers, wake-word models, and required Alpine files from their publishers and verifies pinned SHA-256 hashes. Given the owner's stock data, `build_cube_bundle.py` also downloads Lenovo's 2019 kernel source, reference patches and a pinned toolchain, builds the kernel, and runs extraction, image creation and verification. It does not download a complete stock ROM. The new source-to-device workflow has **not** yet been tested end to end from a clean environment or factory-state device.

In a dedicated x86_64 Linux build environment, replace the example paths below. Root, Go, JADX, `debugfs`, `mke2fs`, registered `qemu-arm`, Git, tar, make, GCC, gzip, cpio, bc, bison, flex, and ample free disk space are required. Use a case-sensitive Linux filesystem with enough space for a roughly 0.9 GB source archive, extracted source, compiler, build intermediates and images; WSL also needs free space on the drive that stores its virtual disk. `--work` must not exist yet and must not contain whitespace.

```sh
sudo python3 ports/lenovo-smart-clock/tools/build_cube_bundle.py \
  --vendor /private/source/vendor.img --oem /private/source/oem.img \
  --donor-boot /private/source/boot.img \
  --slot a --work /private/cube-build
```

The tool requires a matching 2019 stock boot with an embedded configuration. A separately built matching kernel can instead be supplied with `--kernel-out`; a locally downloaded, hash-matching Lenovo source archive can be supplied with `--lenovo-archive`. No stock ROM or device-specific data is fetched. The resulting image is a private experimental build. This step does not write to the clock. After connecting the device in fastboot, use `--action inspect` for preflight checks. Set `a` to the actual inactive target slot.

`--prebuilt-dir` can point to a directory containing the four prebuilt cube executables and `SHA256.json` to skip their Go builds. No public package is currently provided by this project; redistribution terms and device validation remain outstanding. Hash and CPU architecture checks do not establish provenance or working behavior. Stock data and a kernel are still required.

```sh
python3 ports/lenovo-smart-clock/tools/cube_install.py --work /private/cube-build --serial YOUR_SERIAL --slot a --action inspect
```

With the same arguments, `--action trial` **overwrites the inactive system partition** and boots temporarily. After checking the screen, connectivity, and recovery path, return to fastboot and use `--action install --trial-confirmed` with the same arguments to write boot and select the slot. The user must confirm that the exact image was tested; the tool cannot establish this itself. It does not unlock the device, mark a boot successful, or enable shared settings and updates. See [section 5](#5-test-and-install-with-fastboot) for the requirements.

To fetch only public inputs first, run `python3 ports/lenovo-smart-clock/tools/prepare_cube_inputs.py --output /private/cube-inputs`; the output directory must be new.

## 1. Requirements

- The target clock and its power supply, a USB data cable, Python 3, and `fastboot` from Android platform-tools. Building images on Linux also requires root, `debugfs`, `mke2fs`, registered `qemu-arm` binfmt support, and tools needed to build the Alpine rootfs. Audio extraction from the APK requires JADX.
- The owner's stock `vendor` and `oem` partitions as raw ext4 images or extracted directories, plus a compatible stock boot image. General-purpose acquisition and restoration instructions are not yet available. Check important data and recovery options before proceeding.
- A device with both bootloader and AVB unlocked. This repository does not supply unlocking instructions or credentials. Unlocking may erase device data.
- For the manual workflow below, Lenovo's published 2019 kernel source with the required patches already applied, a matching stock configuration, and an ARM64 cross-compiler. The experimental automated workflow above obtains the source, patches and compiler and extracts the configuration from the donor boot; it has not been validated end to end.
- Alpine `apk.static`, `alpine-minirootfs-3.24.2-armv7.tar.gz`, and wake-word models (`.tflite` and `.json`). Place these under `ports/lenovo-smart-clock/inputs/` or the directory specified by `CUBE_INPUTS`. They are not collected automatically.

The commands below assume a Linux shell at the repository root. `/private/cube` and the source paths are examples; replace them with your own. Keep backups and extracted files outside Git in access-restricted locations. Each extraction tool creates its own new output directory.

## 2. Extract stock inputs

Set the paths to your own files. Source files are not modified. Raw ext4 tools cannot directly consume Android sparse images.

```sh
PORT=ports/lenovo-smart-clock
PRIVATE=/private/cube
VENDOR_IMG=/private/source/vendor.img
OEM_IMG=/private/source/oem.img
mkdir -p "$PRIVATE"
python3 "$PORT/tools/extract_stock_firmware.py" "$VENDOR_IMG" "$PRIVATE/firmware"
python3 "$PORT/tools/extract_oem_apk.py" "$OEM_IMG" "$PRIVATE/oem-apk"
```

The `firmware` directory contains Wi-Fi/Bluetooth files and a `manifest.json` with hashes. Touch firmware is not extracted. Find the actual APK filename under `oem-apk`, then replace the example below. Other stock versions may not have the same files or layout.

```sh
OEM_APK=/private/cube/oem-apk/actual-apk-filename.apk
python3 "$PORT/tools/extract_audio_apk.py" "$OEM_APK" "$PRIVATE/audio"
```

The audio tool uses JADX to inspect `Lenovo.java` in the 2019 APK. If extraction fails, do not substitute data from another model or firmware version without establishing compatibility. Run each script with `--help` for details.

## 3. Prepare build inputs

On Windows, with PowerShell and Go installed, `ports/lenovo-smart-clock/tools/build-cube-userspace.ps1` builds the experimental daemon, Bluetooth bridge, diagnostic binary, and kernel init. The standard diagnostic binary does not include experimental OTA support.

The following shows equivalent Linux builds. The Go version and other module requirements must be satisfied.

```sh
mkdir -p bin "$PORT/build"
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go -C echod build -tags cube -o "$(pwd)/bin/echod-cube-arm" ./cmd/echod
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o bin/btbridge-cube-arm ./cmd/btbridge
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go -C "$PORT/serial" build -tags cube_slot,cube_shared -o "$(pwd)/$PORT/build/cube-serial-arm" .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go -C "$PORT/rescue" build -tags cube_boot -o "$(pwd)/$PORT/build/cube-boot-init-aarch64" .
```

Only if you have the patched kernel source and a matching configuration, you can start a build as shown below. Replace all paths with absolute paths on your system. `CUBE_KERNEL_BUILD` must not exist yet; paths must not contain spaces.

```sh
export CUBE_KERNEL_SRC=/private/source/patched-kernel
export CUBE_STOCK_CONFIG=/private/source/stock.config
export CUBE_RUNTIME_INIT="$(pwd)/$PORT/build/cube-boot-init-aarch64"
export CUBE_KERNEL_BUILD="$PRIVATE/kernel-build"
export CROSS_COMPILE=aarch64-linux-gnu-
bash "$PORT/tools/build_cube_kernel.sh"
export CUBE_KERNEL_OUT="$CUBE_KERNEL_BUILD/out"
```

The build script does not apply patches. Reproducing the kernel using only publicly supplied tools and inputs has not been verified.

## 4. Create rootfs and boot images

Place the required Alpine files and other inputs in `CUBE_INPUTS` and choose an output directory that does not exist. `CUBE_TEST_AUDIO=1` enables experimental 2019 audio initialization. Without separately supplied receiver helpers, features such as AirPlay are unavailable.

```sh
export CUBE_FIRMWARE="$PRIVATE/firmware"
export CUBE_AUDIO="$PRIVATE/audio"
export CUBE_INPUTS=/private/cube-inputs
export CUBE_OUTPUT="$PRIVATE/rootfs-output"
export CUBE_TEST_AUDIO=1
export CUBE_BLUETOOTH=1
sudo --preserve-env=CUBE_FIRMWARE,CUBE_AUDIO,CUBE_INPUTS,CUBE_OUTPUT,CUBE_KERNEL_OUT,CUBE_TEST_AUDIO,CUBE_BLUETOOTH bash "$PORT/tools/build-private-rootfs.sh"
```

The resulting `system_b-experimental-DO-NOT-FLASH.img` is a private experimental image. Its filename does not establish slot compatibility or safety. Shared settings and OTA are disabled by default. See the [technical notes](README.md) for the Japanese font and optional helpers.

This example creates an image **for slot A**, using a compatible stock donor boot, the built kernel, and DTB. It applies only when B is the current working slot. For B, use `--experimental-system-b` and `--expect-system-b`, and change the remaining slot references accordingly.

```sh
DONOR_BOOT=/private/source/boot.img
KERNEL_GZ="$CUBE_KERNEL_OUT/arch/arm64/boot/Image.gz"
DTB="$CUBE_KERNEL_OUT/arch/arm64/boot/dts/mediatek/mt8167s_ref.dtb"
BOOT="$PRIVATE/boot-a.img"
python3 "$PORT/tools/make_diagnostic_boot.py" --experimental-system-a "$DONOR_BOOT" "$KERNEL_GZ" "$DTB" "$BOOT"
python3 "$PORT/tools/verify_diagnostic_boot.py" --expect-system-a "$DONOR_BOOT" "$BOOT" "$KERNEL_GZ" "$DTB"
SYSTEM="$CUBE_OUTPUT/system_b-experimental-DO-NOT-FLASH.img"
python3 "$PORT/tools/verify_private_rootfs.py" --serial "$PORT/build/cube-serial-arm" --expect-test-audio --expect-test-bluetooth --expect-japanese-font --audio "$CUBE_AUDIO" "$SYSTEM" bin/echod-cube-arm "$CUBE_FIRMWARE" > "$PRIVATE/root-report.json"
```

Passing these checks does not guarantee that the device will boot or be recoverable. The image contains proprietary stock-derived data. Do not publish it without checking the redistribution terms.

## 5. Test and install with fastboot

This is a developer example. The trial **overwrites the selected inactive `system` partition**. Do not proceed if you need to preserve that slot's stock system. Establish a recovery path for your device and computer before writing.

Disconnect power, connect USB, then hold volume up while reconnecting power. Confirm the clock appears in fastboot. Set `SERIAL` to the target device shown by `fastboot devices`; `BOOT_SHA256` is calculated from the verified boot image. Do not select another device.

```sh
fastboot devices
SERIAL=your-device-serial
fastboot -s "$SERIAL" getvar current-slot
fastboot -s "$SERIAL" getvar unlocked
fastboot -s "$SERIAL" getvar at-vboot-state
BOOT_SHA256=$(sha256sum "$BOOT" | cut -d ' ' -f 1)
python3 "$PORT/tools/install_cube_fastboot.py" --serial "$SERIAL" --slot a --boot "$BOOT" --boot-sha256 "$BOOT_SHA256" --system "$SYSTEM" --root-report "$PRIVATE/root-report.json" --action inspect
```

`inspect` validates the files and device state. For this A-slot example, B must be the current successful, bootable slot, A must be inactive, the images must fit their partitions, and bootloader and AVB must be unlocked. If a check fails, do not bypass it.

### Trial boot: overwrite system A

Once inspection succeeds and you understand that A will be overwritten, replace the last option in the command above with `--action trial`. The tool writes `system_a` and temporarily boots the boot image. Check the display, touch, Wi-Fi, audio, and the recovery path you need. During a temporary boot, the root slot and the bootloader's recorded current slot can differ.

### Normal boot: write boot A and select A

After a successful trial of the same image on the same device, return to fastboot and replace the last option with `--action install --trial-confirmed`. This writes `boot_a` and selects A. Then run `fastboot -s "$SERIAL" reboot` and check the features again.

**This operation does not mark A successful.** Until it is marked successful, boot attempts are consumed and the device may return to the previous slot. General-purpose instructions for safe success registration, rollback, and migration to shared data are incomplete. This procedure alone does not enable settings preservation between slots or automatic updates.
