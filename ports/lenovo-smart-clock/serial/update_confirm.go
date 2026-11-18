//go:build cube_slot

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var errCubeTrialUnhealthy = errors.New("trial did not become healthy before timeout")

// trialIdentity rechecks the signed pending update, shared store, selected
// system and the continued availability of the other slot. It only reads.
func trialIdentity(publicKey ed25519.PublicKey) (cubePending, cubePartitionNode, error) {
	var empty cubePending
	var noNode cubePartitionNode
	if err := secureCubeDirectory("/data/techo5-linux"); err != nil {
		return empty, noNode, err
	}
	p, err := readCubePending("/data/techo5-linux/cube-update-pending.json", publicKey)
	if err != nil {
		return empty, noNode, err
	}
	m, err := verifyCubeManifest(publicKey, p.Manifest, p.Signature)
	if err != nil {
		return empty, noNode, err
	}
	seq, err := readCubeSequence("/data/techo5-linux/cube-update-sequence")
	if err != nil {
		return empty, noNode, err
	}
	if seq != m.Sequence {
		return empty, noNode, errors.New("pending sequence mismatch")
	}
	other := "a"
	if m.Slot == "a" {
		other = "b"
	}
	probe := m
	probe.Slot = other
	current, nodes, err := inspectCubeUpdateDevice(probe)
	if err != nil {
		return empty, noNode, err
	}
	if current != m.Slot {
		return empty, noNode, errors.New("pending trial is not running")
	}
	for _, node := range nodes {
		if node.name == "misc" {
			return p, node, nil
		}
	}
	return empty, noNode, errors.New("misc partition missing")
}

func removeCubePending(path string, publicKey ed25519.PublicKey, expected cubePending) error {
	p, err := readCubePending(path, publicKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(p.Manifest, expected.Manifest) || !bytes.Equal(p.Signature, expected.Signature) {
		return errors.New("pending record changed")
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	if err := os.Remove(path); err != nil {
		return err
	}
	return d.Sync()
}

// monitorCubeTrial is not connected to init or the daemon. The deployment must
// pin a trusted key and verify display and wake-up behavior before enabling it.
// It keeps the update lock while continuously observing the same daemon.
func monitorCubeTrial(ctx context.Context, publicKey ed25519.PublicKey) error {
	if err := secureCubeDirectory("/run/techo5"); err != nil {
		return err
	}
	lock, err := acquireCubeUpdateLock("/run/techo5/cube-update.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	initial, _, err := trialIdentity(publicKey)
	if err != nil {
		return err
	}
	pid, start, err := awaitCubeHealth(ctx, 5*time.Second, 2*time.Minute, 15*time.Second, 10*time.Minute, func() (int, uint64, bool, error) {
		p, _, checkErr := trialIdentity(publicKey)
		if checkErr == nil && (!bytes.Equal(p.Manifest, initial.Manifest) || !bytes.Equal(p.Signature, initial.Signature)) {
			return 0, 0, false, errors.New("pending trial changed")
		}
		pid, start, processErr := findTrialDaemon("/proc", "/usr/local/bin/techo5")
		fb, fbErr := os.Stat("/dev/fb0")
		healthy := checkErr == nil && processErr == nil && fbErr == nil && fb.Mode()&os.ModeDevice != 0 && fb.Mode()&os.ModeCharDevice != 0
		return pid, start, healthy, nil
	})
	if err != nil {
		return err
	}
	p, miscNode, err := trialIdentity(publicKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(p.Manifest, initial.Manifest) || !bytes.Equal(p.Signature, initial.Signature) {
		return errors.New("pending trial changed")
	}
	pid2, start2, err := findTrialDaemon("/proc", "/usr/local/bin/techo5")
	if err != nil || pid2 != pid || start2 != start {
		return errors.New("daemon changed before confirmation")
	}
	misc, err := openCubePartition(miscNode, true)
	if err != nil {
		return err
	}
	sector, err := readSector(misc)
	if err != nil {
		misc.Close()
		return err
	}
	m, err := verifyCubeManifest(publicKey, p.Manifest, p.Signature)
	if err != nil {
		misc.Close()
		return err
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		misc.Close()
		return err
	}
	updated, err := markSlotGood(sector[:32], string(cmd), m.Slot)
	if err != nil {
		misc.Close()
		return err
	}
	err = writeSlotSector(misc, sector, updated)
	closeErr := misc.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return removeCubePending("/data/techo5-linux/cube-update-pending.json", publicKey, p)
}
