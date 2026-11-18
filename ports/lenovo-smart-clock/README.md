# Lenovo Smart Clock (first generation) experimental port

This is a work-in-progress port of TECHO5 for the CD-24501F (`cube`). It is
based on upstream main commit `8fdeba8b` (after release v1.6.44) and the
Lenovo 2019 kernel. On one unit, display, touch, Wi-Fi, Home Assistant voice,
the speaker, the mute indication, Music Assistant / Sendspin, AirPlay 2 and
Bluetooth speaker playback were confirmed by the owner. These results do not cover every upstream feature or
every firmware revision. Camera support is disabled; the Echo OTA updater must
not be used for this device. On-device wake-word recognition and response were
also confirmed; recovery from an unbootable image still needs a device test.
On the second unit, a short H.264/AAC video was confirmed with picture and
sound after the cube-specific decoder startup fix. This does not establish
support for every codec, streaming source, or live video feature.

## Current status

This is source for an experimental build, **not a ready-to-install release**.
The first prototype boots a marked ext4 root on `system_b`, with its own
boot image in `boot_b`; slot A was retained on that unit. A second stock unit
booted from B and was installed into its inactive A slot. Its owner confirmed
screen, touch, Wi-Fi and audio; switching from TECHO5 A back to stock B and then
back to TECHO5 A was also tested. Both A/B success records were verified after
the switch back. Later, that unit's stock B was replaced with TECHO5 to test
shared-data slot switching, so neither slot on that unit is stock now. A full
raw-backup restore has not been tested. A USB recovery init
can update the daemon in the marked system without replacing its settings. It
verifies the complete transfer and saves a copy of the previous daemon, but
it is not a complete installer or a rollback-tested OTA mechanism.
For an existing cube with SSH explicitly configured, `tools/update_daemon_ssh.py`
can install a locally built daemon while retaining a copy of the old executable.
It verifies the device marker and both transfer and startup; it is a manual
developer tool, not an automatic rollback or signed update service. It was
tested on one unit by installing a temporary copy and restoring the prior
executable, with settings retained.

An experimental, explicitly enabled shared-data migration copies the current
TECHO5 settings and Bluetooth state to a dedicated directory on the existing
userdata filesystem, without formatting it. On the second unit the migration
and startup were tested, and Wi-Fi and SSH reconnected with the shared data
mounted. A temporary boot of an image in the other system slot also reused
that store; display, touch and Home Assistant were confirmed. A manual fastboot
install then booted the other slot normally with the same Wi-Fi and Home
Assistant settings. Both slots' success records were read back. The original
slot-local data remains in place. A USB recovery command
can disable shared-data startup after a detected failure; that recovery path and
automatic rollback after a failed A/B update have not yet been tested on a
device. Enabling shared data alone does not implement or enable OTA updates.
For development images only, `CUBE_SHARED_DATA=1` opts into this migration;
verify the resulting image with `--expect-shared-data`. Use a diagnostic built
with `cube_shared` so USB recovery is available if migration fails.

Do not use the Echo installers or release images on a Lenovo Smart Clock. A
fresh-device installer, board-specific updates, reproducible kernel build and
documentation are still being developed.
`tools/install_cube_fastboot.py` is an experimental two-stage fastboot tool: it
requires a verified rootfs report, a matching boot image, an explicitly chosen
inactive slot and a successful active stock slot. It was used to trial and
install onto the inactive A slot of one stock unit; other revisions and the
corresponding B install path still need testing.

## Source and extracted inputs

`rescue/` and `serial/` are the two USB diagnostics; `overlay/` contains the
board startup scripts. `tools/` includes the extraction, verification, rootfs
and temporary daemon-update scripts. The rootfs builder expects the owner's
extracted firmware and audio tables, matching kernel output, Alpine packages,
and wake-word models via explicit paths. It does not download or bundle them.
Optional ARM receiver helpers can be supplied in the upstream `bin/` directory;
the builder stages them when present. AirPlay 2 was tested on the unit;
Spotify Connect and the other helper-dependent features still need tests.
`tools/extract_upstream_helpers.py` extracts these inputs from the SHA-256-pinned
upstream v1.6.44 rootfs into a new directory; copy the results into `bin/` before
building. These third-party binaries retain their own license terms.
See each script's usage for its required inputs. `build-private-rootfs.sh` uses
`CUBE_FIRMWARE`, `CUBE_AUDIO`, `CUBE_KERNEL_OUT`, and `CUBE_OUTPUT`; it requires
Linux, root, QEMU binfmt, and the prerequisite tools and packages. It emits a
private experimental image and does not access the clock.
The Japanese fallback font is fetched from the upstream pinned release and
checked before installation; `CUBE_CJK_ARCHIVE` can point to the same archive
for an offline build. Its OFL license is installed alongside it.
Set `CUBE_BLUETOOTH=1` to enable the experimentally tested MediaTek Bluetooth
bridge at startup; pairing remains controlled by the device settings.

The kernel build used Lenovo's official 2019 source and external patches from
[kortregt/lenovo-smart-clock-linux](https://github.com/kortregt/lenovo-smart-clock-linux).
Their full source provenance and redistribution terms, the exact kernel build
recipe, and all modified kernel source still need to be documented and verified
before offering an image for distribution. Proprietary firmware, device backups,
credentials, and extracted vendor audio data are deliberately excluded.
`tools/build_cube_kernel.sh` records the build steps for an already patched 2019
source tree, stock configuration, ARM64 toolchain and cube init; it does not fetch
or apply the external reference patches. Its fresh-build path needs independent
verification before use as an installation guide.

The port reuses [TECHO5](https://github.com/HuskerMinion/techo5), which is MIT
licensed; see the repository's LICENSE and NOTICE for upstream attribution. Other
inputs retain their own licenses.
