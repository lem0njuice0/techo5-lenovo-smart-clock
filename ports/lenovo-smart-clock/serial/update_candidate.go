//go:build cube_slot && cube_update_trial

package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// checkCubeCandidateRoot examines an already mounted authenticated image.
// It refuses images that cannot supervise their own trial or preserve data.
func checkCubeCandidateRoot(root string, publicKey []byte) error {
	if len(publicKey) != 32 {
		return errors.New("invalid trusted update key")
	}
	for _, p := range []string{"etc", "etc/techo5-cube", "usr", "usr/local", "usr/local/sbin"} {
		info, err := os.Lstat(filepath.Join(root, p))
		if err != nil {
			return err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || st.Uid != 0 {
			return errors.New("unsafe candidate directory")
		}
	}
	check := func(path string, max int64, expected []byte) error {
		b, err := readCubeRootFile(filepath.Join(root, path), max)
		if err != nil {
			return err
		}
		if !bytes.Equal(b, expected) {
			return errors.New("candidate marker or key mismatch")
		}
		return nil
	}
	if err := check("etc/techo5-cube-root", 128, []byte("TECHO5-CUBE-ROOT-V1\n")); err != nil {
		return err
	}
	if err := check("etc/techo5-cube/enable-shared-data", 128, []byte("TECHO5-CUBE-SHARED-ENABLE-V1\n")); err != nil {
		return err
	}
	if err := check("etc/techo5-cube/update-signing.pub", 128, publicKey); err != nil {
		return err
	}
	inittab, err := readCubeRootFile(filepath.Join(root, "etc/inittab"), 16384)
	if err != nil {
		return err
	}
	count := 0
	for _, line := range strings.Split(string(inittab), "\n") {
		if strings.TrimSpace(line) == "::respawn:/bin/sh /usr/local/sbin/cube-update-watch" {
			count++
		}
	}
	if count != 1 {
		return errors.New("candidate update watcher not enabled exactly once")
	}
	watch, err := readCubeRootFile(filepath.Join(root, "usr/local/sbin/cube-update-watch"), 16384)
	if err != nil {
		return err
	}
	if !bytes.Contains(watch, []byte("cube-serial ota-watch")) {
		return errors.New("candidate watcher is unavailable")
	}
	serialPath := filepath.Join(root, "usr/local/sbin/cube-serial")
	info, err := os.Stat(serialPath)
	if err != nil || info.Mode().Perm()&0111 == 0 {
		return errors.New("candidate diagnostic is not executable")
	}
	serial, err := readCubeRootFile(serialPath, 64*1024*1024)
	if err != nil {
		return err
	}
	for _, token := range [][]byte{[]byte("ota-fetch"), []byte("ota-check"), []byte("ota-update"), []byte("ota-stage"), []byte("ota-watch")} {
		if !bytes.Contains(serial, token) {
			return errors.New("candidate update command missing")
		}
	}
	// Decode at runtime so the normal executable does not itself contain the
	// fault marker that the image builder and signer search for.
	faultMarker, _ := hex.DecodeString("435542452d4f54412d4641554c542d544553542d444f2d4e4f542d52454c45415345")
	if bytes.Contains(serial, faultMarker) {
		return errors.New("fault-injection image cannot be applied automatically")
	}
	return nil
}

func cubeIoctl(fd uintptr, request uintptr, arg uintptr) (uintptr, error) {
	r, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, request, arg)
	if e != 0 {
		return 0, e
	}
	return r, nil
}

// checkCubeCandidateImage mounts an already signature-and-hash-verified
// regular image read-only with journal replay disabled, then always detaches
// it. The caller holds the update lock and retains the source descriptor.
func checkCubeCandidateImage(image *os.File, publicKey []byte) (err error) {
	if image == nil {
		return errors.New("missing candidate image")
	}
	if err := secureCubeDirectory("/run/techo5"); err != nil {
		return err
	}
	control, err := os.OpenFile("/dev/loop-control", os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer control.Close()
	index, err := cubeIoctl(control.Fd(), 0x4c82, 0)
	if err != nil {
		return err
	}
	if index > 4095 {
		return errors.New("invalid loop device number")
	}
	loopPath := "/dev/loop" + strconv.FormatUint(uint64(index), 10)
	loop, err := os.OpenFile(loopPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer loop.Close()
	if _, err := cubeIoctl(loop.Fd(), 0x4c00, image.Fd()); err != nil {
		return err
	}
	defer func() {
		if _, e := cubeIoctl(loop.Fd(), 0x4c01, 0); e != nil {
			err = errors.Join(err, fmt.Errorf("loop detach: %w", e))
		}
	}()
	mountpoint, err := os.MkdirTemp("/run/techo5", "cube-candidate-")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(mountpoint); e != nil {
			err = errors.Join(err, fmt.Errorf("candidate directory cleanup: %w", e))
		}
	}()
	flags := uintptr(syscall.MS_RDONLY | syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC)
	if err := syscall.Mount(loopPath, mountpoint, "ext4", flags, "noload"); err != nil {
		return err
	}
	defer func() {
		if e := syscall.Unmount(mountpoint, 0); e != nil {
			err = errors.Join(err, fmt.Errorf("candidate unmount: %w", e))
		}
	}()
	return checkCubeCandidateRoot(mountpoint, publicKey)
}
