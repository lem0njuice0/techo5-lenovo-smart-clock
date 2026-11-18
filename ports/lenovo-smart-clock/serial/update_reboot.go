//go:build cube_slot && cube_update_trial

package main

import (
	"crypto/ed25519"
	"errors"
	"os"
	"strings"
	"syscall"
	"time"
)

func verifyStagedTrial(record []byte, cmdline, current, target string) error {
	if err := slotRecord(record); err != nil {
		return err
	}
	if (current != "a" && current != "b") || (target != "a" && target != "b") || current == target {
		return errors.New("invalid staged slots")
	}
	slotCount, rootCount := 0, 0
	for _, word := range strings.Fields(cmdline) {
		if strings.HasPrefix(word, "androidboot.slot_suffix=") {
			slotCount++
			if word != "androidboot.slot_suffix=_"+current {
				return errors.New("running slot mismatch")
			}
		}
		if strings.HasPrefix(word, "techo5.cube.root=") {
			rootCount++
			if word != "techo5.cube.root=system_"+current {
				return errors.New("running root mismatch")
			}
		}
	}
	if slotCount != 1 || rootCount != 1 {
		return errors.New("ambiguous running slot")
	}
	active, next := 8, 12
	if current == "b" {
		active, next = 12, 8
	}
	if record[active] == 0 || record[active+2] != 1 || record[next] <= record[active] || record[next+1] == 0 || record[next+2] != 0 {
		return errors.New("staged trial or fallback is not valid")
	}
	return nil
}

// rebootStagedCubeUpdate only reboots after revalidating the authenticated
// pending trial and the current successful fallback under the update lock.
func rebootStagedCubeUpdate(publicKey ed25519.PublicKey) error {
	if err := secureCubeDirectory("/run/techo5"); err != nil {
		return err
	}
	lock, err := waitCubeUpdateLock("/run/techo5/cube-update.lock", 10*time.Second)
	if err != nil {
		return err
	}
	defer lock.Close()
	p, err := readCubePending("/data/techo5-linux/cube-update-pending.json", publicKey)
	if err != nil {
		return err
	}
	m, err := verifyCubeManifest(publicKey, p.Manifest, p.Signature)
	if err != nil {
		return err
	}
	seq, err := readCubeSequence("/data/techo5-linux/cube-update-sequence")
	if err != nil || seq != m.Sequence {
		return errors.New("pending sequence mismatch")
	}
	current, nodes, err := inspectCubeUpdateDevice(m)
	if err != nil {
		return err
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
	defer misc.Close()
	sector, err := readSector(misc)
	if err != nil {
		return err
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return err
	}
	if err := verifyStagedTrial(sector[:32], string(cmd), current, m.Slot); err != nil {
		return err
	}
	syscall.Sync()
	return syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
}
