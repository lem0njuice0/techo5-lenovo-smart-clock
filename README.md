# TECHO5 for Lenovo Smart Clock

**English** | [日本語](README.ja.md)

An experimental port of [TECHO5](https://github.com/HuskerMinion/techo5) to the first-generation Lenovo Smart Clock (CD-24501F, `cube`). It can act as a Home Assistant voice satellite and music player with a touchscreen.

> **Work in progress:** Some features have been tested on physical devices, but the port has not been tested exhaustively. A complete installer for a factory-state device is not available. The tested devices use the 2019 kernel. Do not use Echo images, installers, or OTA updates on this device.

## Getting started

| Looking for | Start here |
| --- | --- |
| Installation | [Installation guide](ports/lenovo-smart-clock/INSTALL.en.md): prerequisites, extracting stock data, building, trial booting, and flashing. |
| Feature status | [Feature tables below](#feature-status) |
| Technical details | [English technical notes](ports/lenovo-smart-clock/README.md) / [Japanese notes](ports/lenovo-smart-clock/README.ja.md) |
| Upstream | [TECHO5](https://github.com/HuskerMinion/techo5) / [copy of the upstream README](README.upstream.md) |

Installation requires an unlocked device, data extracted from the owner's stock firmware, a compatible kernel, and a build environment. Instructions for obtaining stock data, unlocking, reproducing the kernel build, marking a boot successful, and recovery are not yet complete. There is no ready-to-flash public image. The installation guide describes the current developer workflow and its gaps; it has not been validated end to end from a factory-state device.

The [experimental single-entry pipeline](ports/lenovo-smart-clock/INSTALL.en.md#single-entry-experimental-pipeline) accepts the owner's stock data, obtains pinned public inputs and kernel source, builds a private bundle, and provides inspect, trial, and install stages. A PowerShell wrapper runs it under WSL. The complete source-to-device workflow is still awaiting end-to-end testing.

An experimental [shared-template workflow](ports/lenovo-smart-clock/INSTALL.en.md#assembling-from-a-shared-template) separates the common rootfs from the owner's stock firmware and audio data. A full-size assembled image passed offline checks, but the workflow has not been boot-tested and no distributable template is available yet.

## Feature status

These results describe the tested devices and conditions, not a guarantee for all firmware versions or configurations. Some optional features require additional build inputs or settings.

### Tested on hardware

| Feature | What was tested and limitations |
| --- | --- |
| Display and touch | TECHO5 interface and touch input |
| Wi-Fi | Connecting and reconnecting after reboot |
| Speaker and volume | Audio output and volume control |
| Microphone switch | Physical switch and on-screen mute indicator |
| Home Assistant voice | Speech recognition and spoken responses through the clock |
| Wake word | Hands-free wake, recognition, and response |
| Music Assistant / Sendspin | Music playback, including after reboot |
| AirPlay / AirPlay 2 | Music from an iPhone, with the required helpers in a test build |
| Bluetooth speaker | Pairing, playback, and reconnecting after reboot |
| Japanese text | Song titles, notifications, and other text with a Japanese font installed |
| Video | A short H.264/AAC clip with picture and sound; other formats and sources are untested |
| A/B slots and settings | Wi-Fi and Home Assistant settings retained across slot switches using shared data |
| Experimental OTA | Signed HTTPS update and rollback on the second unit when the kernel and health watcher could run. No production feed is available |

### Untested or incomplete

| Feature or area | Current status |
| --- | --- |
| Spotify Connect | A helper can be included, but playback has not been tested on hardware |
| Other video formats and streams | Not comprehensively tested; a YouTube watch-page URL may not play directly |
| Other Bluetooth uses | BLE proxy and other uses beyond speaker playback are untested |
| Other upstream features | Features not listed as tested should not be assumed to be ported or working |
| Public automatic updates | No production feed or release signing key; OTA has not been tested on the first unit |
| Recovery | Return to a stock slot was tested on the second unit. Full backup restoration and automatic recovery from an unbootable kernel remain untested |
| Factory-state installation | End-to-end instructions for stock extraction, unlocking, kernel reproduction, and marking boot successful are incomplete |

### Unavailable or disabled

| Feature | Reason |
| --- | --- |
| Camera | Disabled because this device has no camera |
| Echo OTA and installers | Designed for different hardware and must not be used here |

## Stock firmware

The extraction tools take the owner's stock `vendor` partition for Wi-Fi/Bluetooth firmware and the `oem` APK for audio settings. The kernel uses Lenovo's published 2019 source and external patches. Stock firmware and extracted proprietary data are not included in this repository. See the [installation guide](ports/lenovo-smart-clock/INSTALL.en.md) and [technical notes](ports/lenovo-smart-clock/README.md).

## Credits and licensing

This port is based on [HuskerMinion/techo5](https://github.com/HuskerMinion/techo5), with kernel-related reference work from [kortregt/lenovo-smart-clock-linux](https://github.com/kortregt/lenovo-smart-clock-linux). See [LICENSE](LICENSE) and [NOTICE](NOTICE) for TECHO5. Other inputs have their own terms; kernel provenance and redistribution terms are still being reviewed.
