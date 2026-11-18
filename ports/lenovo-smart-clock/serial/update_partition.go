//go:build cube_slot

package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// cubePartitionNode is discovered read-only from a unique eMMC partition.
type cubePartitionNode struct {
	cubePartition
	path string
}

func discoverCubePartition(sysroot, devroot, name string) (cubePartitionNode, error) {
	var out cubePartitionNode
	switch name {
	case "misc", "userdata", "system_a", "system_b", "boot_a", "boot_b":
	default:
		return out, errors.New("unexpected partition name")
	}
	entries, err := os.ReadDir(sysroot)
	if err != nil {
		return out, err
	}
	for _, entry := range entries {
		if !blockName.MatchString(entry.Name()) {
			continue
		}
		base := filepath.Join(sysroot, entry.Name())
		raw, err := os.ReadFile(filepath.Join(base, "uevent"))
		if err != nil {
			continue
		}
		values := map[string]string{}
		for _, line := range strings.Split(string(raw), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				if _, exists := values[k]; exists {
					return out, errors.New("duplicate uevent key")
				}
				values[k] = v
			}
		}
		if values["PARTNAME"] != name {
			continue
		}
		if out.path != "" {
			return out, fmt.Errorf("duplicate partition %s", name)
		}
		if values["DEVNAME"] != entry.Name() {
			return out, errors.New("partition name mismatch")
		}
		major, e1 := strconv.ParseUint(values["MAJOR"], 10, 32)
		minor, e2 := strconv.ParseUint(values["MINOR"], 10, 32)
		rawSize, e3 := os.ReadFile(filepath.Join(base, "size"))
		sectors, e4 := strconv.ParseUint(strings.TrimSpace(string(rawSize)), 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || sectors == 0 || sectors > math.MaxInt64/512 {
			return out, errors.New("invalid partition geometry")
		}
		out = cubePartitionNode{cubePartition{name, major, minor, int64(sectors) * 512}, filepath.Join(devroot, entry.Name())}
	}
	if out.path == "" {
		return out, fmt.Errorf("partition %s not found", name)
	}
	return out, nil
}

// openCubePartition validates the opened descriptor rather than trusting a
// path or sysfs alone. The caller must perform mount checks and hold the lock.
func openCubePartition(p cubePartitionNode, write bool) (*os.File, error) {
	flags := syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	if write {
		flags = syscall.O_RDWR | syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_SYNC
	}
	fd, err := syscall.Open(p.path, flags, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), p.path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		f.Close()
		return nil, errors.New("partition is not a block device")
	}
	rdev := uint64(st.Rdev)
	major := (rdev >> 8 & 0xfff) | (rdev >> 32 & 0xfffff000)
	minor := (rdev & 0xff) | (rdev >> 12 & 0xffffff00)
	if major != p.major || minor != p.minor {
		f.Close()
		return nil, errors.New("partition device number mismatch")
	}
	var actual uint64
	// BLKGETSIZE64 encodes sizeof(size_t), which differs in the 32-bit
	// userspace used on this clock. The ioctl's result itself is uint64.
	blkgetsize64 := uintptr(0x80001272) | unsafe.Sizeof(uintptr(0))<<16
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), blkgetsize64, uintptr(unsafe.Pointer(&actual)))
	if errno != 0 {
		f.Close()
		return nil, errno
	}
	if actual != uint64(p.size) {
		f.Close()
		return nil, errors.New("partition size changed")
	}
	return f, nil
}
