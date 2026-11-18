//go:build cube_slot && cube_update_trial

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func eligibleCubeCleanup(current, target string, stored, offered uint64, allowFailed bool) bool {
	return stored == offered && (current == target || allowFailed)
}

// cleanupCubeIncoming removes only a fully authenticated package whose trial
// has either been confirmed or is marked unbootable after fallback. The caller
// holds the update lock and has already established that no pending record exists.
func cleanupCubeIncoming(publicKey ed25519.PublicKey, allowFailed bool) error {
	const dir = "/data/techo5-linux/incoming"
	if err := secureCubeDirectory(dir); err != nil {
		return err
	}
	dataInfo, err := os.Stat("/data")
	if err != nil {
		return err
	}
	dataStat, ok := dataInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("shared data device unavailable")
	}
	open := func(name string, max int64) (*os.File, error) {
		return openCubeIncoming(filepath.Join(dir, name), max, uint64(dataStat.Dev))
	}
	manifest, err := open("manifest.json", 16384)
	if err != nil {
		return err
	}
	defer manifest.Close()
	signature, err := open("manifest.sig", ed25519.SignatureSize)
	if err != nil {
		return err
	}
	defer signature.Close()
	system, err := open("system.img", 1024*1024*1024)
	if err != nil {
		return err
	}
	defer system.Close()
	boot, err := open("boot.img", 64*1024*1024)
	if err != nil {
		return err
	}
	defer boot.Close()
	payload, err := io.ReadAll(io.LimitReader(manifest, 16385))
	if err != nil {
		return err
	}
	sig, err := io.ReadAll(io.LimitReader(signature, ed25519.SignatureSize+1))
	if err != nil {
		return err
	}
	m, err := verifyCubeManifest(publicKey, payload, sig)
	if err != nil {
		return err
	}
	seq, err := readCubeSequence("/data/techo5-linux/cube-update-sequence")
	if err != nil {
		return err
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return err
	}
	current := ""
	for _, word := range bytes.Fields(cmd) {
		if bytes.Equal(word, []byte("androidboot.slot_suffix=_a")) {
			current = "a"
		}
		if bytes.Equal(word, []byte("androidboot.slot_suffix=_b")) {
			current = "b"
		}
	}
	if current == "" {
		return errors.New("running slot unavailable")
	}
	if !eligibleCubeCleanup(current, m.Slot, seq, m.Sequence, allowFailed) {
		return nil
	}
	probe := m
	if current == m.Slot {
		if current == "a" {
			probe.Slot = "b"
		} else {
			probe.Slot = "a"
		}
	}
	running, nodes, err := inspectCubeUpdateDevice(probe)
	if err != nil || running != current {
		return errors.New("current layout unavailable")
	}
	var miscNode cubePartitionNode
	for _, node := range nodes {
		if node.name == "misc" {
			miscNode = node
		}
	}
	if miscNode.path == "" {
		return errors.New("misc partition missing")
	}
	misc, err := openCubePartition(miscNode, false)
	if err != nil {
		return err
	}
	sector, readErr := readSector(misc)
	misc.Close()
	if readErr != nil {
		return readErr
	}
	if err := canResolvePending(sector[:32], string(cmd), current, m.Slot); err != nil {
		return err
	}
	sh, _ := hex.DecodeString(m.SystemSHA256)
	bh, _ := hex.DecodeString(m.BootSHA256)
	r := updateImage{source: system, size: m.SystemSize}
	copy(r.hash[:], sh)
	k := updateImage{source: boot, size: m.BootSize}
	copy(k.hash[:], bh)
	if err := digestImage(r); err != nil {
		return err
	}
	if err := digestImage(k); err != nil {
		return err
	}
	for _, f := range []*os.File{manifest, signature, system, boot} {
		if err := f.Close(); err != nil {
			return err
		}
	}
	for _, name := range []string{"manifest.json", "manifest.sig", "system.img", "boot.img"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	if err := os.Remove(dir); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(dir))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}
