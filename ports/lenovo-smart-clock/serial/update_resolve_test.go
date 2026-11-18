//go:build cube_slot

package main

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestResolvePendingOnlyWhenSafe(t *testing.T) {
	b := exampleSlotRecord() // A successful, B trial
	if err := canResolvePending(b, "androidboot.slot_suffix=_a", "a", "b"); err == nil {
		t.Fatal("discarded a bootable trial")
	}
	if err := canResolvePending(b, "androidboot.slot_suffix=_b", "b", "b"); err == nil {
		t.Fatal("discarded an unconfirmed current trial")
	}
	b[12], b[13], b[14] = 0, 0, 0
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	if err := canResolvePending(b, "androidboot.slot_suffix=_a", "a", "b"); err != nil {
		t.Fatal(err)
	}
	b[12], b[13], b[14], b[8] = 15, 0, 1, 14
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	if err := canResolvePending(b, "androidboot.slot_suffix=_b", "b", "b"); err != nil {
		t.Fatal(err)
	}
	if err := canResolvePending(b, "androidboot.slot_suffix=_b androidboot.slot_suffix=_a", "b", "b"); err == nil {
		t.Fatal("accepted ambiguous boot slot")
	}
}
