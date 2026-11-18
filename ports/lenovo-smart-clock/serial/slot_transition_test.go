//go:build cube_slot

package main

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestSlotTransitionPreservesCurrentAndMakesOtherTrial(t *testing.T) {
	for _, current := range []string{"a", "b"} {
		b := exampleSlotRecord()
		active, target := 8, 12
		if current == "b" {
			active, target = 12, 8
		}
		b[active], b[active+1], b[active+2] = 15, 0, 1
		b[target], b[target+1], b[target+2] = 14, 0, 1
		binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
		cmd := "androidboot.slot_suffix=_" + current
		if _, err := slotTransition(b, cmd, current, "activate"); err == nil {
			t.Fatal("activated without preparation")
		}
		prepared, err := slotTransition(b, cmd, current, "prepare")
		if err != nil || slotRecord(prepared) != nil || prepared[target] != 0 || prepared[active+2] != 1 {
			t.Fatalf("prepare %s: %v", current, err)
		}
		trial, err := slotTransition(prepared, cmd, current, "activate")
		if err != nil || slotRecord(trial) != nil || trial[active] != 14 || trial[active+2] != 1 || trial[target] != 15 || trial[target+1] != 3 || trial[target+2] != 0 {
			t.Fatalf("activate %s: %v", current, err)
		}
		if _, err := slotTransition(b, cmd+" androidboot.slot_suffix=_a", current, "prepare"); err == nil {
			t.Fatal("accepted ambiguous slot")
		}
		b[28] ^= 1
		if _, err := slotTransition(b, cmd, current, "prepare"); err == nil {
			t.Fatal("accepted invalid checksum")
		}
	}
}
