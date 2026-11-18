//go:build cube_slot

package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type cubePartition struct {
	name         string
	major, minor uint64
	size         int64
}

type cubeMount struct {
	major, minor    uint64
	root, point, fs string
	rw              bool
}

func parseCubeMounts(data string) ([]cubeMount, error) {
	var mounts []cubeMount
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			return nil, errors.New("malformed mountinfo")
		}
		separator := -1
		for i := 6; i < len(fields); i++ {
			if fields[i] == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || separator+3 >= len(fields) {
			return nil, errors.New("malformed mountinfo separator")
		}
		parts := strings.Split(fields[2], ":")
		if len(parts) != 2 {
			return nil, errors.New("malformed device number")
		}
		major, e1 := strconv.ParseUint(parts[0], 10, 32)
		minor, e2 := strconv.ParseUint(parts[1], 10, 32)
		if e1 != nil || e2 != nil {
			return nil, errors.New("malformed device number")
		}
		rw := false
		for _, opt := range strings.Split(fields[5], ",") {
			if opt == "rw" {
				rw = true
			}
		}
		mounts = append(mounts, cubeMount{major, minor, fields[3], fields[4], fields[separator+1], rw})
	}
	return mounts, nil
}

// validateCubeUpdateLayout is a read-only check on independently obtained,
// unique and device-number-verified partition descriptors and mountinfo. The
// eventual caller must hold a system-wide update lock across all checks and
// writes, and reopen and revalidate block descriptors without symlink following.
func validateCubeUpdateLayout(cmdline, current string, parts []cubePartition, mountinfo string, rootMarker, enabledMarker, dataMarker []byte, systemSize, bootSize int64) error {
	if current != "a" && current != "b" {
		return errors.New("invalid running slot")
	}
	other := "a"
	if current == "a" {
		other = "b"
	}
	seenSlot, seenRoot := 0, 0
	for _, word := range strings.Fields(cmdline) {
		if strings.HasPrefix(word, "androidboot.slot_suffix=") {
			seenSlot++
			if word != "androidboot.slot_suffix=_"+current {
				return errors.New("boot slot mismatch")
			}
		}
		if strings.HasPrefix(word, "techo5.cube.root=") {
			seenRoot++
			if word != "techo5.cube.root=system_"+current {
				return errors.New("root slot mismatch")
			}
		}
	}
	if seenSlot != 1 || seenRoot != 1 {
		return errors.New("ambiguous running slot")
	}
	if string(rootMarker) != "TECHO5-CUBE-ROOT-V1\n" || string(enabledMarker) != "TECHO5-CUBE-SHARED-ENABLE-V1\n" || string(dataMarker) != "TECHO5-CUBE-SHARED-DATA-V1\n" {
		return errors.New("required cube or shared-data markers missing")
	}
	want := map[string]bool{"misc": true, "userdata": true, "system_" + current: true, "system_" + other: true, "boot_" + other: true}
	found := map[string]cubePartition{}
	devices := map[string]bool{}
	for _, p := range parts {
		if !want[p.name] {
			continue
		}
		if _, ok := found[p.name]; ok {
			return fmt.Errorf("duplicate partition %s", p.name)
		}
		dev := fmt.Sprintf("%d:%d", p.major, p.minor)
		if devices[dev] || p.size <= 0 {
			return errors.New("invalid or duplicate partition device")
		}
		devices[dev] = true
		found[p.name] = p
	}
	if len(found) != len(want) {
		return errors.New("missing required partition")
	}
	if found["system_"+other].size != systemSize || bootSize <= 0 || found["boot_"+other].size < bootSize {
		return errors.New("partition size mismatch")
	}
	mounts, err := parseCubeMounts(mountinfo)
	if err != nil {
		return err
	}
	rootOK, dataOK, storeOK := false, false, false
	points := map[string]bool{}
	for _, m := range mounts {
		if m.point == "/" || m.point == "/data" || m.point == "/run/cube-userdata" {
			if points[m.point] {
				return errors.New("duplicate essential mount point")
			}
			points[m.point] = true
		}
		matches := func(p cubePartition) bool { return p.major == m.major && p.minor == m.minor }
		if matches(found["system_"+other]) || matches(found["boot_"+other]) || matches(found["misc"]) {
			return errors.New("update target or misc is mounted")
		}
		if m.point == "/" {
			rootOK = matches(found["system_"+current]) && m.root == "/" && m.fs == "ext4"
		}
		if m.point == "/data" {
			dataOK = matches(found["userdata"]) && m.root == "/techo5-cube/data" && m.fs == "ext4" && m.rw
		}
		if m.point == "/run/cube-userdata" {
			storeOK = matches(found["userdata"]) && m.root == "/" && m.fs == "ext4" && m.rw
		}
	}
	if !rootOK || !dataOK || !storeOK {
		return errors.New("active root or shared data mount does not match")
	}
	return nil
}
