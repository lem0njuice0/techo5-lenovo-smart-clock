//go:build cube_slot

package main

import (
	"crypto/ed25519"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

func secureCubeDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || st.Uid != 0 {
		return errors.New("unsafe update directory")
	}
	return nil
}

// stagePrepared runs after the caller has acquired the lock and verified the
// device layout and exclusive descriptors. The callbacks persist sequence
// and signed pending data on the verified shared filesystem.
func stagePrepared(publicKey ed25519.PublicKey, payload, signature []byte, current, cmdline string, floor uint64, systemSize, bootSize int64, rootfs, kernel io.ReaderAt, misc, system, boot updateBlock, advance func(uint64) error, pending func([]byte, []byte) error) error {
	if advance == nil || pending == nil || misc == nil || system == nil || boot == nil {
		return errors.New("incomplete stage dependencies")
	}
	m, r, k, err := validateSignedBundle(publicKey, payload, signature, current, floor, systemSize, bootSize, rootfs, kernel)
	if err != nil {
		return err
	}
	sector, err := readSector(misc)
	if err != nil {
		return err
	}
	if _, err := slotTransition(sector[:32], cmdline, current, "prepare"); err != nil {
		return err
	}
	if err := advance(m.Sequence); err != nil {
		return err
	}
	if err := pending(payload, signature); err != nil {
		return err
	}
	return applySlotImages(misc, system, boot, cmdline, current, r, k)
}

// stageCubeUpdate is a prototype and is intentionally not exposed through a
// command, API or service. The caller must pin the trusted key, initialize the
// sequence through a verified installation, and implement authenticated trial
// recovery and health confirmation before enabling it. It does not reboot.
func stageCubeUpdate(publicKey ed25519.PublicKey, payload, signature []byte, rootfs, kernel io.ReaderAt) error {
	if os.Geteuid() != 0 {
		return errors.New("update requires root")
	}
	if err := secureCubeDirectory("/run/techo5"); err != nil {
		return err
	}
	lock, err := waitCubeUpdateLock("/run/techo5/cube-update.lock", 10*time.Second)
	if err != nil {
		return err
	}
	defer lock.Close()
	m, err := verifyCubeManifest(publicKey, payload, signature)
	if err != nil {
		return err
	}
	current, nodes, err := inspectCubeUpdateDevice(m)
	if err != nil {
		return err
	}
	if err := secureCubeDirectory("/data/techo5-linux"); err != nil {
		return err
	}
	sequencePath := "/data/techo5-linux/cube-update-sequence"
	pendingPath := "/data/techo5-linux/cube-update-pending.json"
	floor, err := readCubeSequence(sequencePath)
	if err != nil {
		return err
	}
	node := map[string]cubePartitionNode{}
	for _, p := range nodes {
		node[p.name] = p
	}
	if _, err := os.Lstat(pendingPath); err == nil {
		return errors.New("pending update requires resolution")
	} else if !os.IsNotExist(err) {
		return err
	}
	misc, err := openCubePartition(node["misc"], true)
	if err != nil {
		return err
	}
	defer misc.Close()
	system, err := openCubePartition(node["system_"+m.Slot], true)
	if err != nil {
		return err
	}
	defer system.Close()
	boot, err := openCubePartition(node["boot_"+m.Slot], true)
	if err != nil {
		return err
	}
	defer boot.Close()
	// Repeat mount and root checks after claiming all inactive devices.
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return err
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	rootMarker, err := readCubeMarker("/etc/techo5-cube-root")
	if err != nil {
		return err
	}
	enabled, err := readCubeMarker("/etc/techo5-cube/enable-shared-data")
	if err != nil {
		return err
	}
	dataMarker, err := readCubeMarker("/run/cube-userdata/techo5-cube/marker")
	if err != nil {
		return err
	}
	var parts []cubePartition
	for _, p := range nodes {
		parts = append(parts, p.cubePartition)
	}
	if err := validateCubeUpdateLayout(string(cmd), current, parts, string(mounts), rootMarker, enabled, dataMarker, m.SystemSize, m.BootSize); err != nil {
		return err
	}
	return stagePrepared(publicKey, payload, signature, current, string(cmd), floor, node["system_"+m.Slot].size, node["boot_"+m.Slot].size, rootfs, kernel, misc, system, boot,
		func(seq uint64) error { return advanceCubeSequence(sequencePath, seq) },
		func(manifest, signature []byte) error {
			return writeCubePending(pendingPath, publicKey, manifest, signature)
		})
}
