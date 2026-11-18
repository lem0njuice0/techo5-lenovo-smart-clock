//go:build cube_slot && cube_update_trial

package main

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestVerifyStagedTrial(t *testing.T) {
	b := exampleSlotRecord()
	cmd := "androidboot.slot_suffix=_a techo5.cube.root=system_a"
	if err := verifyStagedTrial(b, cmd, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := verifyStagedTrial(b, cmd, "a", "a"); err == nil {
		t.Fatal("accepted active target")
	}
	if err := verifyStagedTrial(b, cmd+" techo5.cube.root=system_b", "a", "b"); err == nil {
		t.Fatal("accepted ambiguous root")
	}
	bad := append([]byte(nil), b...)
	bad[10] = 0
	binary.BigEndian.PutUint32(bad[28:], crc32.ChecksumIEEE(bad[:28]))
	if err := verifyStagedTrial(bad, cmd, "a", "b"); err == nil {
		t.Fatal("accepted unconfirmed fallback")
	}
	bad = append([]byte(nil), b...)
	bad[13] = 0
	binary.BigEndian.PutUint32(bad[28:], crc32.ChecksumIEEE(bad[:28]))
	if err := verifyStagedTrial(bad, cmd, "a", "b"); err == nil {
		t.Fatal("accepted exhausted trial")
	}
}
