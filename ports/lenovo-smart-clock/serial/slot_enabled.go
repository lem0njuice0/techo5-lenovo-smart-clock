//go:build cube_slot

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

const slotOffset = 2048

func slotHelp() string { return ", slot-status, slot-mark-good" }

var blockName = regexp.MustCompile(`^mmcblk[0-9]+p[0-9]+$`)

// slotRecord validates the old libavb A/B record used by this clock.
func slotRecord(b []byte) error {
	if len(b) != 32 || string(b[:4]) != "\x00AB0" || b[4] != 1 || b[5] != 0 {
		return errors.New("unsupported slot record")
	}
	if binary.BigEndian.Uint32(b[28:]) != crc32.ChecksumIEEE(b[:28]) {
		return errors.New("slot record checksum mismatch")
	}
	for _, at := range []int{8, 12} {
		if b[at] > 15 || b[at+1] > 7 || b[at+2] > 1 {
			return errors.New("invalid slot fields")
		}
	}
	return nil
}

func markBGood(b []byte, cmdline string) ([]byte, error) {
	return markSlotGood(b, cmdline, "b")
}

func markSlotGood(b []byte, cmdline, target string) ([]byte, error) {
	if err := slotRecord(b); err != nil {
		return nil, err
	}
	if target != "a" && target != "b" {
		return nil, errors.New("unsupported target slot")
	}
	suffixes := 0
	for _, word := range strings.Fields(cmdline) {
		if strings.HasPrefix(word, "androidboot.slot_suffix=") {
			suffixes++
			if word != "androidboot.slot_suffix=_"+target {
				return nil, errors.New("bootloader selected another slot")
			}
		}
	}
	if suffixes != 1 {
		return nil, errors.New("missing or ambiguous bootloader slot")
	}
	selected, stock := 8, 12
	if target == "b" {
		selected, stock = 12, 8
	}
	if b[stock] == 0 || b[stock+2] != 1 || b[selected] <= b[stock] || b[selected] == 0 {
		return nil, errors.New("slot state does not preserve the successful stock slot and preferred target")
	}
	// The bootloader decrements tries before starting Linux. On the final trial,
	// zero tries can therefore describe the currently running slot. Require an
	// unambiguous matching cube root in that case, so a temporary fastboot boot
	// of the other system cannot mark the wrong slot successful.
	if b[selected+1] == 0 && b[selected+2] == 0 {
		roots := 0
		for _, word := range strings.Fields(cmdline) {
			if strings.HasPrefix(word, "techo5.cube.root=") {
				roots++
				if word != "techo5.cube.root=system_"+target {
					return nil, errors.New("boot root differs from selected slot")
				}
			}
		}
		if roots != 1 {
			return nil, errors.New("missing or ambiguous boot root")
		}
	}
	out := append([]byte(nil), b...)
	out[selected+1], out[selected+2] = 0, 1
	binary.BigEndian.PutUint32(out[28:], crc32.ChecksumIEEE(out[:28]))
	return out, nil
}

// miscDevice requires one sysfs partition named misc and matching device numbers.
func miscDevice(sysroot, devroot string) (string, uint64, uint64, error) {
	entries, err := os.ReadDir(sysroot)
	if err != nil {
		return "", 0, 0, err
	}
	var path string
	var major, minor uint64
	for _, e := range entries {
		if !blockName.MatchString(e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sysroot, e.Name(), "uevent"))
		if err != nil {
			continue
		}
		fields := map[string]string{}
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				fields[k] = v
			}
		}
		if fields["PARTNAME"] != "misc" || fields["DEVNAME"] != e.Name() {
			continue
		}
		if path != "" {
			return "", 0, 0, errors.New("multiple misc partitions")
		}
		major, err = strconv.ParseUint(fields["MAJOR"], 10, 32)
		if err != nil {
			return "", 0, 0, err
		}
		minor, err = strconv.ParseUint(fields["MINOR"], 10, 32)
		if err != nil {
			return "", 0, 0, err
		}
		path = filepath.Join(devroot, e.Name())
	}
	if path == "" {
		return "", 0, 0, errors.New("misc partition not found")
	}
	return path, major, minor, nil
}

func openMisc(write bool) (*os.File, error) {
	path, maj, min, err := miscDevice("/sys/class/block", "/dev")
	if err != nil {
		return nil, err
	}
	flags := syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	if write {
		flags = syscall.O_RDWR | syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_SYNC
	}
	fd, err := syscall.Open(path, flags, 0)
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
	if !ok || info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		f.Close()
		return nil, errors.New("misc is not a block device")
	}
	rdev := uint64(st.Rdev)
	actualMajor := (rdev >> 8 & 0xfff) | (rdev >> 32 & 0xfffff000)
	actualMinor := (rdev & 0xff) | (rdev >> 12 & 0xffffff00)
	if actualMajor != maj || actualMinor != min {
		f.Close()
		return nil, errors.New("misc device number mismatch")
	}
	return f, nil
}

func slotCommand(s string) (string, bool) {
	if s != "slot-status" && s != "slot-mark-good" {
		return "", false
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return fmt.Sprintf("unavailable: %v\n", err), true
	}
	f, err := openMisc(s == "slot-mark-good")
	if err != nil {
		return fmt.Sprintf("unavailable: %v\n", err), true
	}
	defer f.Close()
	sector := make([]byte, 512)
	if _, err := f.ReadAt(sector, slotOffset); err != nil {
		return fmt.Sprintf("read failed: %v\n", err), true
	}
	record := sector[:32]
	if err := slotRecord(record); err != nil {
		return fmt.Sprintf("invalid: %v\n", err), true
	}
	if s == "slot-status" {
		return fmt.Sprintf("A priority=%d tries=%d successful=%d; B priority=%d tries=%d successful=%d\n", record[8], record[9], record[10], record[12], record[13], record[14]), true
	}
	target := ""
	for _, word := range strings.Fields(string(cmd)) {
		if word == "androidboot.slot_suffix=_a" {
			target = "a"
		}
		if word == "androidboot.slot_suffix=_b" {
			target = "b"
		}
	}
	updated, err := markSlotGood(record, string(cmd), target)
	if err != nil {
		return fmt.Sprintf("refused: %v\n", err), true
	}
	copy(sector[:32], updated)
	if _, err := f.WriteAt(sector, slotOffset); err != nil {
		return fmt.Sprintf("write failed: %v\n", err), true
	}
	if err := f.Sync(); err != nil {
		return fmt.Sprintf("sync failed: %v\n", err), true
	}
	check := make([]byte, 512)
	if _, err := f.ReadAt(check, slotOffset); err != nil || string(check) != string(sector) {
		return "readback mismatch\n", true
	}
	return fmt.Sprintf("slot %s marked successful; other slot metadata preserved\n", strings.ToUpper(target)), true
}
