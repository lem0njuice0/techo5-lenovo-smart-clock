//go:build cube_slot

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

// checkCubeBoot checks the Android v0 container and its requested target root.
// The authenticated digest, rather than this parser, establishes provenance.
func checkCubeBoot(source io.ReaderAt, size int64, slot string) error {
	const headerLen = 1632
	if size < headerLen {
		return errors.New("boot header truncated")
	}
	header := make([]byte, headerLen)
	if n, err := source.ReadAt(header, 0); err != nil {
		return err
	} else if n != len(header) {
		return io.ErrUnexpectedEOF
	}
	if string(header[:8]) != "ANDROID!" {
		return errors.New("unexpected boot magic")
	}
	ks, rs, ss := binary.LittleEndian.Uint32(header[8:]), binary.LittleEndian.Uint32(header[16:]), binary.LittleEndian.Uint32(header[24:])
	page, version := binary.LittleEndian.Uint32(header[36:]), binary.LittleEndian.Uint32(header[40:])
	if version != 0 || page < headerLen || page > 65536 || page&(page-1) != 0 || ks == 0 || rs == 0 {
		return errors.New("unsupported boot header")
	}
	align := func(n uint32) int64 { return (int64(n) + int64(page) - 1) / int64(page) * int64(page) }
	if int64(page)+align(ks)+align(rs)+int64(ss) > size {
		return errors.New("boot components exceed image")
	}
	if !bytes.Equal(header[608:1632], make([]byte, 1024)) {
		return errors.New("extended boot command line unsupported")
	}
	cmd := header[64:576]
	if i := bytes.IndexByte(cmd, 0); i >= 0 {
		if !bytes.Equal(cmd[i:], make([]byte, len(cmd)-i)) {
			return errors.New("invalid command line padding")
		}
		cmd = cmd[:i]
	}
	if len(cmd) == 0 {
		return errors.New("missing command line")
	}
	for _, c := range cmd {
		if c < 32 || c > 126 {
			return errors.New("invalid command line")
		}
	}
	rootCount, keep := 0, false
	for _, word := range strings.Fields(string(cmd)) {
		if strings.HasPrefix(word, "techo5.cube.root=") {
			rootCount++
			if word != "techo5.cube.root=system_"+slot {
				return errors.New("boot image requests wrong root")
			}
		}
		if word == "keep_initramfs" {
			keep = true
		}
	}
	if rootCount != 1 || !keep {
		return errors.New("boot arguments incomplete")
	}
	return nil
}

// applySignedBundle joins trust, payload and transaction checks. It remains
// unreachable from device commands until real block-device discovery, mount
// verification, locking, version rollback policy and persistent trial-health
// handling are implemented.
func validateSignedBundle(publicKey ed25519.PublicKey, payload, signature []byte, current string, minSequence uint64, systemCapacity, bootCapacity int64, rootfs, kernel io.ReaderAt) (cubeUpdateManifest, updateImage, updateImage, error) {
	var empty cubeUpdateManifest
	var noImage updateImage
	m, err := verifyCubeManifest(publicKey, payload, signature)
	if err != nil {
		return empty, noImage, noImage, err
	}
	if m.Sequence < minSequence {
		return empty, noImage, noImage, errors.New("update sequence is older than accepted sequence")
	}
	if (current != "a" && current != "b") || m.Slot == current {
		return empty, noImage, noImage, errors.New("update target is not inactive")
	}
	if m.SystemSize != systemCapacity || m.BootSize > bootCapacity {
		return empty, noImage, noImage, errors.New("image sizes do not fit partitions")
	}
	if rootfs == nil || kernel == nil {
		return empty, noImage, noImage, errors.New("missing image")
	}
	if err := checkCubeBoot(kernel, m.BootSize, m.Slot); err != nil {
		return empty, noImage, noImage, err
	}
	var magic [2]byte
	if m.SystemSize < 1082 {
		return empty, noImage, noImage, errors.New("system image truncated")
	}
	if n, err := rootfs.ReadAt(magic[:], 1080); err != nil {
		return empty, noImage, noImage, err
	} else if n != len(magic) {
		return empty, noImage, noImage, io.ErrUnexpectedEOF
	}
	if magic != [2]byte{0x53, 0xef} {
		return empty, noImage, noImage, errors.New("system image is not ext4")
	}
	sh, _ := hex.DecodeString(m.SystemSHA256)
	bh, _ := hex.DecodeString(m.BootSHA256)
	r := updateImage{source: rootfs, size: m.SystemSize}
	copy(r.hash[:], sh)
	k := updateImage{source: kernel, size: m.BootSize}
	copy(k.hash[:], bh)
	if err := digestImage(r); err != nil {
		return empty, noImage, noImage, err
	}
	if err := digestImage(k); err != nil {
		return empty, noImage, noImage, err
	}
	return m, r, k, nil
}

func applySignedBundle(publicKey ed25519.PublicKey, payload, signature []byte, misc, system, boot updateBlock, cmdline, current string, minSequence uint64, systemCapacity, bootCapacity int64, rootfs, kernel io.ReaderAt) error {
	_, r, k, err := validateSignedBundle(publicKey, payload, signature, current, minSequence, systemCapacity, bootCapacity, rootfs, kernel)
	if err != nil {
		return err
	}
	return applySlotImages(misc, system, boot, cmdline, current, r, k)
}
