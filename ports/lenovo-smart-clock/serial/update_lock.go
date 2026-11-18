//go:build cube_slot

package main

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// Lock a root-owned path on a local runtime filesystem before preflight, then
// retain the returned descriptor through writes, readback and activation.
// This helper does not itself select or create the parent directory.
func acquireCubeUpdateLock(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		f.Close()
		return nil, errors.New("unsafe update lock file")
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func waitCubeUpdateLock(path string, timeout time.Duration) (*os.File, error) {
	if timeout <= 0 || timeout > 30*time.Second {
		return nil, errors.New("invalid update lock timeout")
	}
	deadline := time.Now().Add(timeout)
	for {
		f, err := acquireCubeUpdateLock(path)
		if err == nil {
			return f, nil
		}
		if (!errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN)) || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(250 * time.Millisecond)
	}
}
