//go:build cube_slot

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// readCubeSequence fails closed if the installation has not been explicitly
// provisioned with a sequence file. The caller must hold the update lock.
func readCubeSequence(path string) (uint64, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return 0, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Size() < 2 || info.Size() > 21 {
		return 0, errors.New("unsafe update sequence file")
	}
	data, err := io.ReadAll(io.LimitReader(f, 22))
	if err != nil {
		return 0, err
	}
	value := strings.TrimSuffix(string(data), "\n")
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || string(data) != fmt.Sprintf("%d\n", n) {
		return 0, errors.New("invalid update sequence")
	}
	return n, nil
}

// advanceCubeSequence records an authenticated sequence before writing slot
// data. The private parent directory must be controlled by root and reside on
// the shared data filesystem; the caller holds the same lock throughout.
func advanceCubeSequence(path string, sequence uint64) error {
	current, err := readCubeSequence(path)
	if err != nil {
		return err
	}
	if sequence < current {
		return errors.New("update sequence would go backwards")
	}
	if sequence == current {
		return nil
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
		return errors.New("unsafe sequence directory")
	}
	f, err := os.CreateTemp(dir, ".cube-update-sequence-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := fmt.Fprintf(f, "%d\n", sequence); err != nil {
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
	// Recheck the current value just before rename; callers must also exclude
	// other writers via the lock and control directory access.
	if now, err := readCubeSequence(path); err != nil || now != current {
		return errors.New("update sequence changed")
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		return err
	}
	if now, err := readCubeSequence(path); err != nil || now != sequence {
		return errors.New("update sequence readback failed")
	}
	return nil
}
