//go:build cube_slot

package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
)

func readCubeMarker(path string) ([]byte, error) {
	return readCubeRootFile(path, 128)
}

func readCubeRootFile(path string, max int64) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || st.Uid != 0 || st.Nlink != 1 || info.Size() < 0 || info.Size() > max {
		return nil, errors.New("unsafe cube marker")
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("trusted file too large")
	}
	return data, nil
}

// inspectCubeUpdateDevice only reads system state and block geometry. It does
// not prove the OEM firmware provenance and never opens a block device writable.
// A writer must recheck everything under a held update lock immediately before
// opening the inactive devices, then keep those validated descriptors open.
func inspectCubeUpdateDevice(m cubeUpdateManifest) (string, []cubePartitionNode, error) {
	if os.Geteuid() != 0 {
		return "", nil, errors.New("update inspection requires root")
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return "", nil, err
	}
	current := ""
	n := 0
	for _, word := range strings.Fields(string(cmd)) {
		if strings.HasPrefix(word, "androidboot.slot_suffix=") {
			n++
			switch word {
			case "androidboot.slot_suffix=_a":
				current = "a"
			case "androidboot.slot_suffix=_b":
				current = "b"
			default:
				return "", nil, errors.New("unsupported slot")
			}
		}
	}
	if n != 1 || current == "" || m.Slot == current || (m.Slot != "a" && m.Slot != "b") {
		return "", nil, errors.New("ambiguous or active target slot")
	}
	compat, err := os.ReadFile("/proc/device-tree/compatible")
	if err != nil {
		return "", nil, err
	}
	if !strings.Contains("\x00"+string(compat), "\x00mediatek,mt8167\x00") {
		return "", nil, errors.New("unsupported device tree")
	}
	mountinfo, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "", nil, err
	}
	rootMarker, err := readCubeMarker("/etc/techo5-cube-root")
	if err != nil {
		return "", nil, err
	}
	enabled, err := readCubeMarker("/etc/techo5-cube/enable-shared-data")
	if err != nil {
		return "", nil, err
	}
	storeInfo, err := os.Lstat("/run/cube-userdata/techo5-cube")
	if err != nil {
		return "", nil, err
	}
	storeStat, ok := storeInfo.Sys().(*syscall.Stat_t)
	if !ok || !storeInfo.IsDir() || storeInfo.Mode()&os.ModeSymlink != 0 || storeInfo.Mode().Perm()&0022 != 0 || storeStat.Uid != 0 {
		return "", nil, errors.New("unsafe shared store")
	}
	marker, err := readCubeMarker("/run/cube-userdata/techo5-cube/marker")
	if err != nil {
		return "", nil, err
	}
	var nodes []cubePartitionNode
	var parts []cubePartition
	for _, name := range []string{"misc", "userdata", "system_" + current, "system_" + m.Slot, "boot_" + m.Slot} {
		p, err := discoverCubePartition("/sys/class/block", "/dev", name)
		if err != nil {
			return "", nil, err
		}
		f, err := openCubePartition(p, false)
		if err != nil {
			return "", nil, err
		}
		f.Close()
		nodes = append(nodes, p)
		parts = append(parts, p.cubePartition)
	}
	if err := validateCubeUpdateLayout(string(cmd), current, parts, string(mountinfo), rootMarker, enabled, marker, m.SystemSize, m.BootSize); err != nil {
		return "", nil, err
	}
	return current, nodes, nil
}
