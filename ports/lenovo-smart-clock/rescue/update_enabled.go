//go:build cube_update && cube_boot

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

func prepareSerial(f *os.File) error {
	var t syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&t))); e != 0 {
		return e
	}
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB
	t.Cflag |= syscall.CS8
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&t))); e != 0 {
		return e
	}
	return nil
}

const updateRoot = "/newroot"

type transfer struct {
	f             *os.File
	tmp, dest     string
	size, written int64
	sum           hash.Hash
	expected      string
}

var pending *transfer

func beginTransfer(dir string, size int64, expected string) (*transfer, error) {
	if size <= 0 || size > 100*1024*1024 || len(expected) != 64 {
		return nil, fmt.Errorf("invalid size or hash")
	}
	if _, err := hex.DecodeString(expected); err != nil || expected != strings.ToLower(expected) {
		return nil, fmt.Errorf("invalid hash")
	}
	dest := filepath.Join(dir, "techo5")
	info, err := os.Lstat(dest)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("existing daemon is not a regular file")
	}
	f, err := os.CreateTemp(dir, ".techo5-update-")
	if err != nil {
		return nil, err
	}
	return &transfer{f: f, tmp: f.Name(), dest: dest, size: size, sum: sha256.New(), expected: expected}, nil
}
func (t *transfer) abort() {
	if t.f != nil {
		_ = t.f.Close()
		t.f = nil
	}
	_ = os.Remove(t.tmp)
}
func (t *transfer) data(encoded string) error {
	b, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(b) == 0 || len(b) > 24576 || t.written+int64(len(b)) > t.size {
		return fmt.Errorf("invalid chunk")
	}
	n, err := t.f.Write(b)
	if err != nil || n != len(b) {
		return fmt.Errorf("write failed: %v", err)
	}
	_, _ = t.sum.Write(b)
	t.written += int64(n)
	return nil
}
func (t *transfer) commit() error {
	if t.written != t.size || hex.EncodeToString(t.sum.Sum(nil)) != t.expected {
		return fmt.Errorf("size or digest mismatch")
	}
	if err := t.f.Chmod(0755); err != nil {
		return err
	}
	if err := t.f.Sync(); err != nil {
		return err
	}
	if err := t.f.Close(); err != nil {
		return err
	}
	t.f = nil
	old, err := os.Open(t.dest)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, old)
	_ = old.Close()
	if err != nil {
		return err
	}
	backup := t.dest + ".before-" + hex.EncodeToString(h.Sum(nil))
	if err = os.Link(t.dest, backup); err != nil && !os.IsExist(err) {
		return fmt.Errorf("backup: %w", err)
	}
	if err = os.Rename(t.tmp, t.dest); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(t.dest))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func openUpdateRoot() (string, error) {
	dev, err := partition("/sys/class/block", "/dev")
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(updateRoot, 0755); err != nil {
		return "", err
	}
	if err = syscall.Mount(dev, updateRoot, "ext4", syscall.MS_RDONLY, "noload"); err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = syscall.Unmount(updateRoot, 0)
		}
	}()
	if err = validateRoot(updateRoot); err != nil {
		return "", err
	}
	for _, p := range []string{"usr", "usr/local", "usr/local/bin"} {
		fi, e := os.Lstat(filepath.Join(updateRoot, p))
		if e != nil || !fi.IsDir() {
			return "", fmt.Errorf("invalid directory %s", p)
		}
	}
	if err = syscall.Mount("", updateRoot, "", syscall.MS_REMOUNT, ""); err != nil {
		return "", err
	}
	ok = true
	return filepath.Join(updateRoot, "usr/local/bin"), nil
}
func updateCommand(s string) (string, bool) {
	fields := strings.Fields(s)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "update-") {
		return "", false
	}
	fail := func(err error) (string, bool) { return "error: " + err.Error() + "\n", true }
	switch fields[0] {
	case "update-begin":
		if len(fields) != 3 || pending != nil {
			return fail(fmt.Errorf("invalid request or transaction active"))
		}
		size, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return fail(err)
		}
		dir, err := openUpdateRoot()
		if err != nil {
			return fail(err)
		}
		pending, err = beginTransfer(dir, size, fields[2])
		if err != nil {
			_ = syscall.Unmount(updateRoot, 0)
			return fail(err)
		}
		return "ok begin\n", true
	case "update-data":
		if len(fields) != 2 || pending == nil {
			return fail(fmt.Errorf("no transaction or invalid data"))
		}
		if err := pending.data(fields[1]); err != nil {
			return fail(err)
		}
		return "ok data\n", true
	case "update-abort":
		if pending != nil {
			pending.abort()
			pending = nil
			if err := syscall.Unmount(updateRoot, 0); err != nil {
				return fail(err)
			}
		}
		return "ok abort\n", true
	case "update-commit":
		if len(fields) != 1 || pending == nil {
			return fail(fmt.Errorf("no transaction"))
		}
		if err := pending.commit(); err != nil {
			return fail(err)
		}
		pending = nil
		syscall.Sync()
		if err := syscall.Unmount(updateRoot, 0); err != nil {
			return fail(err)
		}
		return "ok installed\n", true
	}
	return fail(fmt.Errorf("unknown update command"))
}
