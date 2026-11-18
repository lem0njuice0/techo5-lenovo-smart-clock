//go:build cube_slot

package main

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func exampleSlotRecord() []byte {
	b := make([]byte, 32)
	copy(b, "\x00AB0\x01\x00\x00\x00")
	copy(b[8:], []byte{14, 0, 1, 0, 15, 6, 0, 0})
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	return b
}

func TestMarkAGoodPreservesSuccessfulB(t *testing.T) {
	b := exampleSlotRecord()
	copy(b[8:], []byte{15, 6, 0, 0, 14, 0, 1, 0})
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	out, err := markSlotGood(b, "androidboot.slot_suffix=_a", "a")
	if err != nil || out[9] != 0 || out[10] != 1 || string(out[12:16]) != string(b[12:16]) || slotRecord(out) != nil {
		t.Fatalf("bad output %x %v", out, err)
	}
	if _, err := markSlotGood(b, "androidboot.slot_suffix=_a androidboot.slot_suffix=_b", "a"); err == nil {
		t.Fatal("accepted ambiguous slot")
	}
	b[14] = 0
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	if _, err := markSlotGood(b, "androidboot.slot_suffix=_a", "a"); err == nil {
		t.Fatal("accepted unsuccessful stock B")
	}
}

func TestMarkBGoodRequiresValidRecoverableState(t *testing.T) {
	b := exampleSlotRecord()
	out, err := markBGood(b, "console=x androidboot.slot_suffix=_b boot=x")
	if err != nil || out[13] != 0 || out[14] != 1 || string(out[8:12]) != string(b[8:12]) || slotRecord(out) != nil {
		t.Fatalf("bad output %x %v", out, err)
	}
	if b[13] != 6 || b[14] != 0 {
		t.Fatal("input changed")
	}
	if _, err = markBGood(b, "androidboot.slot_suffix=_a"); err == nil {
		t.Fatal("accepted slot A")
	}
	b[12] ^= 1
	if _, err = markBGood(b, "androidboot.slot_suffix=_b"); err == nil {
		t.Fatal("accepted bad checksum")
	}
	b = exampleSlotRecord()
	b[10] = 0
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	if _, err = markBGood(b, "androidboot.slot_suffix=_b"); err == nil {
		t.Fatal("accepted unverified slot A")
	}
}

func TestFinalTrialCanBeConfirmedOnlyForMatchingRunningRoot(t *testing.T) {
	b := exampleSlotRecord()
	b[13] = 0
	binary.BigEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[:28]))
	if _, err := markBGood(b, "androidboot.slot_suffix=_b"); err == nil {
		t.Fatal("accepted missing root")
	}
	if _, err := markBGood(b, "androidboot.slot_suffix=_b techo5.cube.root=system_a"); err == nil {
		t.Fatal("accepted wrong root")
	}
	out, err := markBGood(b, "androidboot.slot_suffix=_b techo5.cube.root=system_b")
	if err != nil || out[14] != 1 {
		t.Fatalf("final running trial not confirmed: %v", err)
	}
}

func TestMiscDeviceRequiresUniqueName(t *testing.T) {
	d := t.TempDir()
	add := func(name string) {
		p := filepath.Join(d, name)
		if err := os.Mkdir(p, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "uevent"), []byte("PARTNAME=misc\nDEVNAME="+name+"\nMAJOR=179\nMINOR=3\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	add("mmcblk0p3")
	p, maj, min, err := miscDevice(d, "/dev")
	if err != nil || p != "/dev/mmcblk0p3" || maj != 179 || min != 3 {
		t.Fatalf("%s %d %d %v", p, maj, min, err)
	}
	add("mmcblk1p3")
	if _, _, _, err := miscDevice(d, "/dev"); err == nil {
		t.Fatal("accepted duplicate misc")
	}
}
