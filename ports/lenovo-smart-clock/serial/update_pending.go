//go:build cube_slot

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type cubePending struct {
	Manifest  []byte `json:"manifest"`
	Signature []byte `json:"signature"`
}

// writeCubePending records signed trial identity before any slot activation.
// The parent must be a root-controlled directory on shared storage and the
// caller must hold the update lock. It never overwrites an existing record.
func writeCubePending(path string, publicKey ed25519.PublicKey, payload, signature []byte) error {
	if _, err := verifyCubeManifest(publicKey, payload, signature); err != nil {
		return err
	}
	data, err := json.Marshal(cubePending{payload, signature})
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	info, err := d.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm()&0022 != 0 || st.Uid != uint32(os.Geteuid()) {
		return errors.New("unsafe pending directory")
	}
	f, err := os.CreateTemp(dir, ".cube-update-pending-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// A hard link is an atomic create-if-absent on the same filesystem.
	if err := os.Link(tmp, path); err != nil {
		return err
	}
	if err := os.Remove(tmp); err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		return err
	}
	read, err := readCubePending(path, publicKey)
	if err != nil || !bytes.Equal(read.Manifest, payload) || !bytes.Equal(read.Signature, signature) {
		return errors.New("pending update readback failed")
	}
	return nil
}

func readCubePending(path string, publicKey ed25519.PublicKey) (cubePending, error) {
	var p cubePending
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return p, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return p, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Size() <= 0 || info.Size() > 24000 {
		return p, errors.New("unsafe pending update record")
	}
	data, err := io.ReadAll(io.LimitReader(f, 24001))
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, errors.New("invalid pending update")
	}
	canonical, err := json.Marshal(p)
	if err != nil || !bytes.Equal(canonical, data) {
		return p, errors.New("noncanonical pending update")
	}
	if _, err := verifyCubeManifest(publicKey, p.Manifest, p.Signature); err != nil {
		return p, err
	}
	return p, nil
}
