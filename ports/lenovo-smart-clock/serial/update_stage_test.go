//go:build cube_slot

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"strings"
	"testing"
)

// Exercises the full-size private images against regular temporary files, not
// block devices. The test is explicitly opted in and removes its own files.
func TestStageFullSizePrivateImages(t *testing.T) {
	dir := os.Getenv("CUBE_TEST_STAGE_DIR")
	sysPath := os.Getenv("CUBE_TEST_SYSTEM_IMAGE")
	bootPath := os.Getenv("CUBE_TEST_BOOT_IMAGE")
	if dir == "" || sysPath == "" || bootPath == "" {
		t.Skip("no private full-size image fixture")
	}
	root, err := os.Open(sysPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	boot, err := os.Open(bootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer boot.Close()
	rs, err := root.Stat()
	if err != nil {
		t.Fatal(err)
	}
	bs, err := boot.Stat()
	if err != nil {
		t.Fatal(err)
	}
	hashFile := func(f *os.File) string {
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(h.Sum(nil))
	}
	rh, bh := hashFile(root), hashFile(boot)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m := cubeUpdateManifest{Format: 1, Board: "cube", Version: "1.0", Sequence: 5, Slot: "b", SystemSize: rs.Size(), SystemSHA256: rh, BootSize: bs.Size(), BootSHA256: bh}
	payload, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, payload)
	create := func(pattern string, size int64) *os.File {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { name := f.Name(); f.Close(); os.Remove(name) })
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		return f
	}
	misc := create("cube-test-misc-*", 4096)
	system := create("cube-test-system-*", rs.Size())
	destBoot := create("cube-test-boot-*", 32*1024*1024)
	record := exampleSlotRecord()
	copy(record[8:], []byte{15, 0, 1, 0, 14, 0, 1, 0})
	binary.BigEndian.PutUint32(record[28:], crc32.ChecksumIEEE(record[:28]))
	if _, err := misc.WriteAt(record, slotOffset); err != nil {
		t.Fatal(err)
	}
	sequence, pending := uint64(0), false
	err = stagePrepared(pub, payload, sig, "a", "androidboot.slot_suffix=_a", 0, rs.Size(), 32*1024*1024, root, boot, misc, system, destBoot, func(s uint64) error { sequence = s; return nil }, func(_, _ []byte) error { pending = true; return nil })
	if err != nil || sequence != 5 || !pending {
		t.Fatalf("full-size staging failed: %v", err)
	}
	sector, err := readSector(misc)
	if err != nil {
		t.Fatal(err)
	}
	if err := slotRecord(sector[:32]); err != nil || sector[12] != 15 || sector[13] != 3 || sector[14] != 0 || sector[10] != 1 {
		t.Fatal("invalid full-size trial state")
	}
}

func TestStagePersistsBeforeChangingSlots(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := make([]byte, 2048)
	root[1080], root[1081] = 0x53, 0xef
	bootData := fakeBoot("a")
	rh, bh := sha256.Sum256(root), sha256.Sum256(bootData)
	m := cubeUpdateManifest{Format: 1, Board: "cube", Version: "1.0", Sequence: 5, Slot: "a", SystemSize: int64(len(root)), SystemSHA256: hex.EncodeToString(rh[:]), BootSize: int64(len(bootData)), BootSHA256: hex.EncodeToString(bh[:])}
	payload, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, payload)
	for _, scenario := range []string{"success", "advance-failure", "pending-failure", "invalid-image"} {
		t.Run(scenario, func(t *testing.T) {
			misc := &memoryBlock{data: make([]byte, 4096)}
			record, e := markSlotGood(exampleSlotRecord(), "androidboot.slot_suffix=_b", "b")
			if e != nil {
				t.Fatal(e)
			}
			copy(misc.data[slotOffset:], record)
			system := &memoryBlock{data: make([]byte, len(root))}
			boot := &memoryBlock{data: make([]byte, len(bootData))}
			events := ""
			candidate := append([]byte(nil), root...)
			if scenario == "invalid-image" {
				candidate[0] ^= 1
			}
			advance := func(sequence uint64) error {
				if misc.writes != 0 || sequence != 5 {
					t.Fatal("sequence persisted after slot mutation")
				}
				events += "S"
				if scenario == "advance-failure" {
					return errors.New("injected")
				}
				return nil
			}
			pending := func(p, s []byte) error {
				if misc.writes != 0 || !bytes.Equal(p, payload) || !bytes.Equal(s, sig) {
					t.Fatal("pending persisted after slot mutation")
				}
				events += "P"
				if scenario == "pending-failure" {
					return errors.New("injected")
				}
				return nil
			}
			err := stagePrepared(pub, payload, sig, "b", "androidboot.slot_suffix=_b", 4, int64(len(root)), int64(len(bootData)), bytes.NewReader(candidate), bytes.NewReader(bootData), misc, system, boot, advance, pending)
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected result: %v", err)
			}
			want := map[string]string{"success": "SP", "advance-failure": "S", "pending-failure": "SP", "invalid-image": ""}[scenario]
			if events != want {
				t.Fatalf("wrong persistence order: %q", events)
			}
			if scenario != "success" && (misc.writes != 0 || system.writes != 0 || boot.writes != 0) {
				t.Fatal("failed preconditions wrote a partition")
			}
			if scenario == "success" && (misc.writes != 2 || misc.data[slotOffset+8] != 15 || misc.data[slotOffset+9] != 3) {
				t.Fatal("trial was not activated")
			}
		})
	}
}

// This negative test creates only the runtime lock file and stops before any
// block-device write when the sequence has not been provisioned.
func TestCubeStageUnprovisionedLive(t *testing.T) {
	if os.Getenv("CUBE_LIVE_STAGING_PREFLIGHT") != "1" {
		t.Skip("no live staging preflight")
	}
	if _, err := os.Lstat("/data/techo5-linux/cube-update-sequence"); !os.IsNotExist(err) {
		t.Skip("device has a provisioned sequence")
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		t.Fatal(err)
	}
	target := "a"
	if strings.Contains(string(cmd), "androidboot.slot_suffix=_a") {
		target = "b"
	}
	p, err := discoverCubePartition("/sys/class/block", "/dev", "system_"+target)
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m := cubeUpdateManifest{Format: 1, Board: "cube", Version: "1.0", Sequence: 1, Slot: target, SystemSize: p.size, SystemSHA256: strings.Repeat("0", 64), BootSize: 1024, BootSHA256: strings.Repeat("0", 64)}
	payload, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	err = stageCubeUpdate(pub, payload, ed25519.Sign(priv, payload), nil, nil)
	if !os.IsNotExist(err) {
		t.Fatalf("expected fail-closed unprovisioned sequence, got %v", err)
	}
}
