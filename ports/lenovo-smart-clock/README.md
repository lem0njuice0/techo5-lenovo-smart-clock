# Lenovo Smart Clock (first generation) experimental port

This is a work-in-progress port of TECHO5 for the CD-24501F (`cube`). It is
based on upstream commit `41001e543df805a7ab99ea8a16159ea2d7a77355` and the
Lenovo 2019 kernel. On one unit, display, touch, Wi-Fi, Home Assistant voice,
the speaker, the mute indication and Music Assistant / Sendspin playback were
confirmed by the owner. These results do not cover every upstream feature or
every firmware revision. Camera support is disabled; the Echo OTA updater must
not be used for this device. Bluetooth, wake-word detection and recovery need
separate device tests.

## Current status

This is source for an experimental build, **not a ready-to-install release**.
The existing prototype boots a marked ext4 root on `system_b`, with its own
boot image in `boot_b`; slot A was retained on the test unit. The owner's stock
backup and recovery path were not independently verified. A USB recovery init
can update the daemon in the marked system without replacing its settings. It
verifies the complete transfer and saves a copy of the previous daemon, but
it is not a complete installer or a rollback-tested OTA mechanism.

Do not use the Echo installers or release images on a Lenovo Smart Clock. A
fresh-device installer, board-specific updates, reproducible kernel build and
documentation are still being developed.

## Source and extracted inputs

`rescue/` and `serial/` are the two USB diagnostics; `overlay/` contains the
board startup scripts. `tools/` includes the extraction, verification, rootfs
and temporary daemon-update scripts. The rootfs builder expects the owner's
extracted firmware and audio tables, matching kernel output, Alpine packages,
and wake-word models via explicit paths. It does not download or bundle them.
See each script's usage for its required inputs. `build-private-rootfs.sh` uses
`CUBE_FIRMWARE`, `CUBE_AUDIO`, `CUBE_KERNEL_OUT`, and `CUBE_OUTPUT`; it requires
Linux, root, QEMU binfmt, and the prerequisite tools and packages. It emits a
private experimental image and does not access the clock.

The kernel build used Lenovo's official 2019 source and external patches from
[kortregt/lenovo-smart-clock-linux](https://github.com/kortregt/lenovo-smart-clock-linux).
Their full source provenance and redistribution terms, the exact kernel build
recipe, and all modified kernel source still need to be documented and verified
before offering an image for distribution. Proprietary firmware, device backups,
credentials, and extracted vendor audio data are deliberately excluded.

The port reuses [TECHO5](https://github.com/HuskerMinion/techo5), which is MIT
licensed; see the repository's LICENSE and NOTICE for upstream attribution. Other
inputs retain their own licenses.
