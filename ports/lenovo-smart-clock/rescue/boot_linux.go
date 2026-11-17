//go:build cube_boot

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const rootMarker = "TECHO5-CUBE-ROOT-V1\n"

var mmcPartition = regexp.MustCompile(`^mmcblk[0-9]+p[0-9]+$`)

func bootRequested(cmdline string) bool {
	for _, word := range strings.Fields(cmdline) {
		if word == "techo5.cube.root=system_b" {
			return true
		}
	}
	return false
}

// partition finds exactly one system_b eMMC partition. Never guess block numbers.
func partition(sysBlock, devRoot string) (string, error) {
	entries, err := os.ReadDir(sysBlock)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range entries {
		if !mmcPartition.MatchString(e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sysBlock, e.Name(), "uevent"))
		if err != nil {
			continue
		}
		part, dev := "", ""
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch k {
			case "PARTNAME":
				part = v
			case "DEVNAME":
				dev = v
			}
		}
		if part == "system_b" && dev == e.Name() {
			found = append(found, filepath.Join(devRoot, dev))
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("expected one system_b eMMC partition, found %d", len(found))
	}
	return found[0], nil
}

func validateRoot(root string) error {
	marker := filepath.Join(root, "etc/techo5-cube-root")
	info, err := os.Lstat(marker)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(rootMarker)) {
		return errors.New("root marker is not a regular file")
	}
	b, err := os.ReadFile(marker)
	if err != nil {
		return err
	}
	if string(b) != rootMarker {
		return errors.New("wrong root marker")
	}
	initInfo, err := os.Lstat(filepath.Join(root, "sbin/init"))
	if err != nil {
		return fmt.Errorf("root has no init: %w", err)
	}
	if !initInfo.Mode().IsRegular() && initInfo.Mode()&os.ModeSymlink == 0 {
		return errors.New("root init is not a file or symlink")
	}
	for _, name := range []string{"dev", "proc", "sys"} {
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("invalid root mount point %s", name)
		}
	}
	return nil
}

// Only the explicitly marked root is mounted, first read-only; an unrecognized
// system_b falls back to the serial diagnostic without writing to it.
func bootIfRequested() (result error) {
	var mounted bool
	var bound []string
	var changedRoot bool
	defer func() {
		if result == nil || changedRoot {
			return
		}
		for i := len(bound) - 1; i >= 0; i-- {
			_ = syscall.Unmount(bound[i], 0)
		}
		if mounted {
			_ = syscall.Unmount("/newroot", 0)
		}
	}()
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return err
	}
	if !bootRequested(string(cmd)) {
		return nil
	}
	var dev string
	for i := 0; i < 20; i++ {
		dev, err = partition("/sys/class/block", "/dev")
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	const root = "/newroot"
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	if err := syscall.Mount(dev, root, "ext4", syscall.MS_RDONLY, "noload"); err != nil {
		return fmt.Errorf("read-only mount: %w", err)
	}
	mounted = true
	if err := validateRoot(root); err != nil {
		return fmt.Errorf("root validation: %w", err)
	}
	// Recovery must remain available until the candidate root is identified.
	if err := syscall.Mount("", root, "", syscall.MS_REMOUNT, ""); err != nil {
		return fmt.Errorf("remount: %w", err)
	}
	for _, name := range []string{"dev", "proc", "sys"} {
		if err := syscall.Mount("/"+name, filepath.Join(root, name), "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind %s: %w", name, err)
		}
		bound = append(bound, filepath.Join(root, name))
	}
	if err := os.Chdir(root); err != nil {
		return err
	}
	if err := syscall.Chroot("."); err != nil {
		return err
	}
	changedRoot = true
	if err := os.Chdir("/"); err != nil {
		return err
	}
	return syscall.Exec("/sbin/init", []string{"/sbin/init"}, os.Environ())
}
