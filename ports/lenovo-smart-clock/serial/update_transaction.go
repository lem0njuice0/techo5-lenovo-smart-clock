//go:build cube_slot

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
)

// updateBlock represents an already validated, exclusively held block device.
// Device discovery, trust, mount checks and locking are the caller's job.
type updateBlock interface {
	io.ReaderAt
	io.WriterAt
	Sync() error
}

type updateImage struct {
	source io.ReaderAt
	size   int64
	hash   [sha256.Size]byte
}

// digestImage rejects truncated and oversized input, and checks the bytes
// against an authenticated expected digest before any metadata is changed.
func digestImage(image updateImage) error {
	if image.source == nil || image.size <= 0 {
		return errors.New("invalid image")
	}
	h := sha256.New()
	if _, err := io.CopyN(h, io.NewSectionReader(image.source, 0, image.size), image.size); err != nil {
		return err
	}
	var extra [1]byte
	n, err := image.source.ReadAt(extra[:], image.size)
	if n != 0 || err != io.EOF {
		return errors.New("image size does not match")
	}
	if !bytes.Equal(h.Sum(nil), image.hash[:]) {
		return errors.New("image digest mismatch")
	}
	return nil
}

func hashRange(h hash.Hash, r io.ReaderAt, size int64) error {
	_, err := io.CopyN(h, io.NewSectionReader(r, 0, size), size)
	return err
}

// writeImage rechecks source bytes while copying and hashes readback after
// sync. A changed source, short write or failed readback prevents activation.
func writeImage(dst updateBlock, image updateImage) error {
	buf := make([]byte, 1024*1024)
	h := sha256.New()
	for off := int64(0); off < image.size; {
		n := int64(len(buf))
		if image.size-off < n {
			n = image.size - off
		}
		part := buf[:int(n)]
		if nread, err := image.source.ReadAt(part, off); err != nil {
			return err
		} else if nread != len(part) {
			return io.ErrUnexpectedEOF
		}
		if _, err := h.Write(part); err != nil {
			return err
		}
		w, err := dst.WriteAt(part, off)
		if err != nil {
			return err
		}
		if w != len(part) {
			return io.ErrShortWrite
		}
		off += n
	}
	if !bytes.Equal(h.Sum(nil), image.hash[:]) {
		return errors.New("source changed during copy")
	}
	if err := dst.Sync(); err != nil {
		return err
	}
	h = sha256.New()
	if err := hashRange(h, dst, image.size); err != nil {
		return err
	}
	if !bytes.Equal(h.Sum(nil), image.hash[:]) {
		return errors.New("image readback mismatch")
	}
	return nil
}

func readSector(misc updateBlock) ([]byte, error) {
	sector := make([]byte, 512)
	n, err := misc.ReadAt(sector, slotOffset)
	if err == nil && n != len(sector) {
		err = io.ErrUnexpectedEOF
	}
	return sector, err
}

func writeSlotSector(misc updateBlock, before, record []byte) error {
	current, err := readSector(misc)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, before) {
		return errors.New("slot metadata changed during update")
	}
	updated := append([]byte(nil), before...)
	copy(updated[:32], record)
	n, err := misc.WriteAt(updated, slotOffset)
	if err != nil {
		return err
	}
	if n != len(updated) {
		return io.ErrShortWrite
	}
	if err := misc.Sync(); err != nil {
		return err
	}
	actual, err := readSector(misc)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, updated) {
		return errors.New("slot metadata readback mismatch")
	}
	return nil
}

// applySlotImages implements only the transactional core; it is not exposed
// on the device until the caller verifies signed metadata, target identity,
// partition sizes, active root, shared data and exclusive access. In
// particular, it must never be called with a mounted or active destination.
func applySlotImages(misc, system, boot updateBlock, cmdline, current string, rootfs, kernel updateImage) error {
	if misc == nil || system == nil || boot == nil {
		return errors.New("missing device")
	}
	for _, image := range []updateImage{rootfs, kernel} {
		if err := digestImage(image); err != nil {
			return fmt.Errorf("image validation: %w", err)
		}
	}
	sector, err := readSector(misc)
	if err != nil {
		return err
	}
	prepared, err := slotTransition(sector[:32], cmdline, current, "prepare")
	if err != nil {
		return err
	}
	if err = writeSlotSector(misc, sector, prepared); err != nil {
		return err
	}
	sector, err = readSector(misc)
	if err != nil {
		return err
	}
	if !bytes.Equal(sector[:32], prepared) {
		return errors.New("prepared slot metadata changed")
	}
	if err = writeImage(system, rootfs); err != nil {
		return fmt.Errorf("system write: %w", err)
	}
	if err = writeImage(boot, kernel); err != nil {
		return fmt.Errorf("boot write: %w", err)
	}
	trial, err := slotTransition(sector[:32], cmdline, current, "activate")
	if err != nil {
		return err
	}
	return writeSlotSector(misc, sector, trial)
}
