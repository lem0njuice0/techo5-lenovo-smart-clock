//go:build cube_slot

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverCubePartition(t *testing.T) {
	d := t.TempDir()
	add := func(device, name string) {
		p := filepath.Join(d, device)
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "uevent"), []byte(fmt.Sprintf("PARTNAME=%s\nDEVNAME=%s\nMAJOR=179\nMINOR=17\n", name, device)), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "size"), []byte("1024\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	add("mmcblk0p17", "system_a")
	p, err := discoverCubePartition(d, "/dev", "system_a")
	if err != nil || p.path != "/dev/mmcblk0p17" || p.size != 1024*512 || p.major != 179 || p.minor != 17 {
		t.Fatalf("unexpected discovery: %+v %v", p, err)
	}
	if _, err := discoverCubePartition(d, "/dev", "system_b"); err == nil {
		t.Fatal("accepted missing partition")
	}
	add("mmcblk1p17", "system_a")
	if _, err := discoverCubePartition(d, "/dev", "system_a"); err == nil {
		t.Fatal("accepted duplicate partition")
	}
}

func TestCubePartitionReadOnlyLive(t *testing.T) {
	if os.Getenv("CUBE_LIVE_READONLY") != "1" {
		t.Skip("no live read-only test")
	}
	for _, name := range []string{"misc", "userdata", "system_a", "system_b", "boot_a", "boot_b"} {
		p, err := discoverCubePartition("/sys/class/block", "/dev", name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := openCubePartition(p, false)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		f.Close()
	}
}

func TestCubeUpdateDeviceReadOnlyLive(t *testing.T) {
	if os.Getenv("CUBE_LIVE_READONLY") != "1" {
		t.Skip("no live read-only test")
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		t.Fatal(err)
	}
	target := "a"
	if strings.Contains(string(cmd), "androidboot.slot_suffix=_a") {
		target = "b"
	}
	p, err := discoverCubePartition("/sys/class/block", "/dev", "system_"+target)
	if err != nil {
		t.Fatal(err)
	}
	current, nodes, err := inspectCubeUpdateDevice(cubeUpdateManifest{Slot: target, SystemSize: p.size, BootSize: 1024})
	if err != nil || current == target || len(nodes) != 5 {
		t.Fatalf("inspection failed: %v", err)
	}
}
