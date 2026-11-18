//go:build cube_slot && cube_update_trial

package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"strings"
	"syscall"
)

func verifyRetryTrial(record []byte, cmdline, slot string) error {
	if _, err := markSlotGood(record, cmdline, slot); err != nil {
		return err
	}
	at := 8
	if slot == "b" {
		at = 12
	}
	if record[at+2] != 0 {
		return errors.New("slot is already confirmed")
	}
	return nil
}

// retryCubeTrial reboots only a signed pending trial with a confirmed fallback.
// It is not invoked on ordinary errors or on an already successful slot.
func retryCubeTrial(publicKey ed25519.PublicKey) error {
	if err := secureCubeDirectory("/run/techo5"); err != nil {
		return err
	}
	lock, err := acquireCubeUpdateLock("/run/techo5/cube-update.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	p, miscNode, err := trialIdentity(publicKey)
	if err != nil {
		return err
	}
	m, err := verifyCubeManifest(publicKey, p.Manifest, p.Signature)
	if err != nil {
		return err
	}
	misc, err := openCubePartition(miscNode, false)
	if err != nil {
		return err
	}
	defer misc.Close()
	sector, err := readSector(misc)
	if err != nil {
		return err
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return err
	}
	if err := verifyRetryTrial(sector[:32], string(cmd), m.Slot); err != nil {
		return err
	}
	syscall.Sync()
	return syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
}

// watchCubeUpdate runs once. A supervisor may call it again later; a staged
// update waiting for its first reboot is deliberately left untouched.
func watchCubeUpdate(ctx context.Context, publicKey ed25519.PublicKey) error {
	if err := secureCubeDirectory("/run/techo5"); err != nil {
		return err
	}
	lock, err := acquireCubeUpdateLock("/run/techo5/cube-update.lock")
	if err != nil {
		return err
	}
	if err := secureCubeDirectory("/data/techo5-linux"); err != nil {
		lock.Close()
		return err
	}
	path := "/data/techo5-linux/cube-update-pending.json"
	p, err := readCubePending(path, publicKey)
	if os.IsNotExist(err) {
		lock.Close()
		return nil
	}
	if err != nil {
		lock.Close()
		return err
	}
	m, err := verifyCubeManifest(publicKey, p.Manifest, p.Signature)
	if err != nil {
		lock.Close()
		return err
	}
	seq, err := readCubeSequence("/data/techo5-linux/cube-update-sequence")
	if err != nil || seq != m.Sequence {
		lock.Close()
		return errors.New("pending sequence mismatch")
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		lock.Close()
		return err
	}
	current := ""
	n := 0
	for _, word := range strings.Fields(string(cmd)) {
		if strings.HasPrefix(word, "androidboot.slot_suffix=") {
			n++
			if word == "androidboot.slot_suffix=_a" {
				current = "a"
			} else if word == "androidboot.slot_suffix=_b" {
				current = "b"
			} else {
				lock.Close()
				return errors.New("invalid running slot")
			}
		}
	}
	if n != 1 || current == "" {
		lock.Close()
		return errors.New("ambiguous running slot")
	}
	if current == m.Slot {
		lock.Close()
		err := monitorCubeTrial(ctx, publicKey)
		if errors.Is(err, errCubeTrialUnhealthy) {
			return retryCubeTrial(publicKey)
		}
		return err
	}
	// Current is the fallback or the old slot before the first reboot.
	_, nodes, err := inspectCubeUpdateDevice(m)
	if err != nil {
		lock.Close()
		return err
	}
	var miscNode cubePartitionNode
	for _, node := range nodes {
		if node.name == "misc" {
			miscNode = node
		}
	}
	if miscNode.path == "" {
		lock.Close()
		return errors.New("misc partition missing")
	}
	misc, err := openCubePartition(miscNode, false)
	if err != nil {
		lock.Close()
		return err
	}
	sector, err := readSector(misc)
	misc.Close()
	if err != nil {
		lock.Close()
		return err
	}
	if err := canResolvePending(sector[:32], string(cmd), current, m.Slot); err != nil {
		lock.Close()
		return nil
	}
	err = removeCubePending(path, publicKey, p)
	lock.Close()
	return err
}
