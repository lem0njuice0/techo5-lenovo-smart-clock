# Obtaining stock data and unlocking the first-generation Smart Clock

**English** | [日本語](STOCK-UNLOCK.ja.md) · [Installation guide](INSTALL.en.md)

This applies to the first-generation Lenovo Smart Clock CD-24501F. The workflow below is based on [kortregt's unlock notes](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/01-unlocking.md) and [extraction notes](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/02-backup.md), which reference the [XDA discussion](https://xdaforums.com/t/lenovo-smart-clock-bootloader-avb-unlock-firmware-region-changer-kernel-source.4130295/). External tools and credentials are not included in this repository; their safety and compatibility with all stock versions have not been established.

Unlocking or flashing can erase data or leave the device unable to boot. If you want to preserve stock data for recovery, obtain it **before** unlocking or writing. Full restoration has not been tested by this project.

## 1. Read the stock data

The reference uses [mtkclient](https://github.com/bkerler/mtkclient) to connect to MediaTek BootROM mode and read the eMMC. BootROM is distinct from fastboot. The reference does not establish an exact device button combination, so no unverified combination is prescribed here. Install mtkclient and the USB drivers appropriate for your OS, and consult the [extraction notes](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/02-backup.md) for connection and permissions. With WSL on Windows, the BootROM USB device must be attached to WSL.

Once mtkclient recognizes the target, the reference lists the following read commands. How to invoke `mtk` depends on its installation. Ensure sufficient free space and check the target and output location.

```sh
mtk printgpt > gpt.txt
mtk rf full_user.bin
mtk r preloader boot1.bin --parttype boot1
mtk r preloader boot2.bin --parttype boot2
```

`full_user.bin` is the complete user area; it was about 7.3 GiB on the reference unit. `boot1.bin` and `boot2.bin` are separate eMMC boot areas. These files may contain device-specific information: do not publish them. A successful read does not establish that restoration will work. If retaining them, record sizes and SHA-256 hashes and investigate recovery separately.

The pipeline can validate the GPT in `full_user.bin` and extract boot, vendor and OEM from the selected slot for a build. **It does not read the device itself.** Existing individual stock partition images can also be supplied. See the [single-entry pipeline](INSTALL.en.md#single-entry-experimental-pipeline).

## 2. Enter fastboot

Connect the clock to the PC with a USB data cable and disconnect the clock's power. Hold volume up while reconnecting power, and keep holding for 20–30 seconds. The display may remain dark. Check recognition from the PC. If multiple devices are connected, specify the correct serial in subsequent commands.

```sh
fastboot devices
fastboot -s DEVICE_SERIAL getvar product
fastboot -s DEVICE_SERIAL getvar unlocked
fastboot -s DEVICE_SERIAL getvar at-vboot-state
```

Replace `DEVICE_SERIAL` with the intended device's serial. The product reported on the tested clock was `mt8167s`, but this value alone does not establish model or firmware compatibility.

## 3. Unlock the bootloader

The reference uses the following commands after identifying the target. Data or settings may be erased. If a confirmation appears on the clock or PC, read it before proceeding.

```sh
fastboot -s DEVICE_SERIAL flashing unlock
fastboot -s DEVICE_SERIAL getvar unlocked
```

Check for `unlocked: yes`. The reference reports that the `bootloader-locked: 1` field in `at-vboot-state` can remain set after a successful unlock; do not use that field alone as the result.

## 4. Unlock Android Things AVB

Bootloader and AVB are separate locks. The reference says that AVB unlocking requires product-specific credentials and describes a package named `cube_unlock_credentials_v2.zip` inside the XDA download `Cube-AVB-factory-Unlock.zip`. **This project does not distribute, retrieve or verify those credentials.** Users must establish the provenance, terms and safety of any external files themselves.

The [unlock notes](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/01-unlocking.md) describe a Windows tool and a Python alternative for Linux/macOS. The Python reference says it also clears the factory partition's persistent digest after unlocking. This is a state-changing operation, not a read-only check. Do not run it without understanding that operation and its implications for returning to stock. Without usable credentials, this method cannot proceed.

For example, the reference gives the command below when its Python tool, `uv`, credentials and `fastboot` are separately available. This example uses the tool and credentials in the working directory and changes AVB state and the persistent digest. Connect only the intended fastboot device and verify the target.

```sh
uv run --no-project --with pycryptodome python at_auth_unlock.py -v cube_unlock_credentials_v2.zip
```

Afterwards, inspect the reported state:

```sh
fastboot -s DEVICE_SERIAL getvar at-vboot-state
```

The reference identifies `avb-locked: 0` as the AVB-unlocked state. If the state cannot be confirmed, do not proceed to flashing. Returning to stock depends on the slots, AVB state and partitions changed; establish recovery before attempting to relock.
