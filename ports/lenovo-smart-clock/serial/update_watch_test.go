//go:build cube_slot && cube_update_trial

package main

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestRetryRequiresSignedTrialStateWithFallback(t *testing.T) {
	b := exampleSlotRecord()
	cmd := "androidboot.slot_suffix=_b techo5.cube.root=system_b"
	if err := verifyRetryTrial(b, cmd, "b"); err != nil {
		t.Fatal(err)
	}
	if err := verifyRetryTrial(b, "androidboot.slot_suffix=_a techo5.cube.root=system_a", "b"); err == nil {
		t.Fatal("accepted wrong running slot")
	}
	b[10] = 0
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	if err := verifyRetryTrial(b, cmd, "b"); err == nil {
		t.Fatal("accepted failed fallback")
	}
	b = exampleSlotRecord()
	b[13], b[14] = 0, 1
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	if err := verifyRetryTrial(b, cmd, "b"); err == nil {
		t.Fatal("rebooted a confirmed slot")
	}
	b = exampleSlotRecord()
	b[13] = 0
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	if err := verifyRetryTrial(b, cmd, "b"); err != nil {
		t.Fatalf("last trial could not retry to fallback: %v", err)
	}
}
