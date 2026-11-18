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
	"io"
	"os"
	"testing"
)

func fakeBoot(slot string) []byte {
	b := make([]byte, 6144)
	copy(b, "ANDROID!")
	binary.LittleEndian.PutUint32(b[8:], 1)
	binary.LittleEndian.PutUint32(b[16:], 1)
	binary.LittleEndian.PutUint32(b[36:], 2048)
	copy(b[64:], "keep_initramfs techo5.cube.root=system_"+slot)
	return b
}

func TestCubeUpdateRealImageReadOnly(t *testing.T) {
	path := os.Getenv("CUBE_TEST_BOOT_IMAGE")
	if path == "" {
		t.Skip("no private boot image")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkCubeBoot(f, info.Size(), "b"); err != nil {
		t.Fatal(err)
	}
	if err := checkCubeBoot(f, info.Size(), "a"); err == nil {
		t.Fatal("accepted wrong target")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	if err := digestImage(updateImage{source: f, size: info.Size(), hash: sum}); err != nil {
		t.Fatal(err)
	}
}

func TestSignedBundleRequiresVerifiedInactiveImages(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := make([]byte, 2048)
	root[1080], root[1081] = 0x53, 0xef
	for _, scenario := range []string{"valid", "bad-signature", "active-slot", "wrong-boot-root", "wrong-system-size", "bad-system-magic", "bad-digest", "older-sequence"} {
		t.Run(scenario, func(t *testing.T) {
			kernel := fakeBoot("a")
			data := append([]byte(nil), root...)
			if scenario == "wrong-boot-root" {
				kernel = fakeBoot("b")
			}
			if scenario == "bad-system-magic" {
				data[1080] = 0
			}
			rh, bh := sha256.Sum256(data), sha256.Sum256(kernel)
			m := cubeUpdateManifest{Format: 1, Board: "cube", Version: "1.2.3", Sequence: 2, Slot: "a", SystemSize: int64(len(data)), SystemSHA256: hex.EncodeToString(rh[:]), BootSize: int64(len(kernel)), BootSHA256: hex.EncodeToString(bh[:])}
			if scenario == "active-slot" {
				m.Slot = "b"
			}
			if scenario == "bad-digest" {
				m.SystemSHA256 = hex.EncodeToString(bh[:])
			}
			payload, _ := json.Marshal(m)
			sig := ed25519.Sign(priv, payload)
			if scenario == "bad-signature" {
				sig[0] ^= 1
			}
			misc := &memoryBlock{data: make([]byte, 4096)}
			record, err := markSlotGood(exampleSlotRecord(), "androidboot.slot_suffix=_b", "b")
			if err != nil {
				t.Fatal(err)
			}
			copy(misc.data[slotOffset:], record)
			system := &memoryBlock{data: make([]byte, len(data))}
			boot := &memoryBlock{data: make([]byte, len(kernel))}
			capacity := int64(len(data))
			if scenario == "wrong-system-size" {
				capacity++
			}
			floor := uint64(2)
			if scenario == "older-sequence" {
				floor = 3
			}
			err = applySignedBundle(pub, payload, sig, misc, system, boot, "androidboot.slot_suffix=_b", "b", floor, capacity, int64(len(kernel)), bytes.NewReader(data), bytes.NewReader(kernel))
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("unexpected result: %v", err)
			}
			if scenario != "valid" && (misc.writes != 0 || system.writes != 0 || boot.writes != 0) {
				t.Fatal("unverified bundle wrote to a device")
			}
		})
	}
}
