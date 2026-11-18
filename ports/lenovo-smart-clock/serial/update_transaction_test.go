//go:build cube_slot

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"testing"
)

type memoryBlock struct {
	data            []byte
	writes          int
	failWrite       bool
	failAt          int
	failSync        bool
	corruptReadback bool
}

func (b *memoryBlock) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n := copy(p, b.data[off:])
	if b.corruptReadback && b.writes > 0 && n > 0 {
		p[0] ^= 1
	}
	if n != len(p) {
		return n, io.EOF
	}
	return n, nil
}
func (b *memoryBlock) WriteAt(p []byte, off int64) (int, error) {
	if b.failWrite || (b.failAt > 0 && b.writes+1 == b.failAt) {
		return 0, errors.New("injected write failure")
	}
	if off < 0 || off >= int64(len(b.data)) {
		return 0, io.ErrShortWrite
	}
	b.writes++
	n := copy(b.data[off:], p)
	if n != len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}
func (b *memoryBlock) Sync() error {
	if b.failSync {
		return errors.New("injected sync failure")
	}
	return nil
}

type changedReader struct {
	original, changed *bytes.Reader
	calls             int
}

func (r *changedReader) ReadAt(p []byte, off int64) (int, error) {
	r.calls++
	if r.calls > 2 {
		return r.changed.ReadAt(p, off)
	}
	return r.original.ReadAt(p, off)
}

func TestUpdateTransaction(t *testing.T) {
	for _, failure := range []string{"none", "bad-digest", "source-changed", "metadata-readback", "system-write", "system-sync", "system-readback", "boot-write", "boot-sync", "boot-readback", "activate-write"} {
		t.Run(failure, func(t *testing.T) {
			misc := &memoryBlock{data: make([]byte, 4096)}
			record := exampleSlotRecord() // B is a trial; construct an active, successful B.
			record, err := markSlotGood(record, "androidboot.slot_suffix=_b", "b")
			if err != nil {
				t.Fatal(err)
			}
			copy(misc.data[slotOffset:], record)
			sys := &memoryBlock{data: make([]byte, 32)}
			boot := &memoryBlock{data: make([]byte, 32)}
			root := []byte("root filesystem image")
			kern := []byte("boot image")
			r := updateImage{source: bytes.NewReader(root), size: int64(len(root)), hash: sha256.Sum256(root)}
			k := updateImage{source: bytes.NewReader(kern), size: int64(len(kern)), hash: sha256.Sum256(kern)}
			switch failure {
			case "bad-digest":
				r.hash[0] ^= 1
			case "source-changed":
				altered := append([]byte(nil), root...)
				altered[0] ^= 1
				r.source = &changedReader{original: bytes.NewReader(root), changed: bytes.NewReader(altered)}
			case "metadata-readback":
				misc.corruptReadback = true
			case "system-write":
				sys.failWrite = true
			case "system-sync":
				sys.failSync = true
			case "system-readback":
				sys.corruptReadback = true
			case "boot-write":
				boot.failWrite = true
			case "boot-sync":
				boot.failSync = true
			case "boot-readback":
				boot.corruptReadback = true
			case "activate-write":
				misc.failAt = 2
			}
			err = applySlotImages(misc, sys, boot, "androidboot.slot_suffix=_b", "b", r, k)
			if (err == nil) != (failure == "none") {
				t.Fatalf("unexpected error: %v", err)
			}
			got := misc.data[slotOffset : slotOffset+32]
			if err := slotRecord(got); err != nil {
				t.Fatal(err)
			}
			if got[12] != 14 && failure == "none" {
				t.Fatal("running B lost fallback priority")
			}
			if got[14] != 1 {
				t.Fatal("running B lost successful state")
			}
			switch failure {
			case "none":
				if got[8] != 15 || got[9] != 3 || got[10] != 0 || misc.writes != 2 || !bytes.Equal(sys.data[:len(root)], root) || !bytes.Equal(boot.data[:len(kern)], kern) {
					t.Fatal("bad successful transition")
				}
			case "bad-digest":
				if misc.writes != 0 || sys.writes != 0 || boot.writes != 0 {
					t.Fatal("wrote before authentication")
				}
			default:
				if got[8] != 0 || got[9] != 0 || got[10] != 0 || misc.writes != 1 {
					t.Fatal("failed update did not leave target unbootable")
				}
			}
		})
	}
}
