//go:build cube_boot

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootRequiresExactArgument(t *testing.T) {
	for _, s := range []string{"", "techo5.cube.root=system_a", "xtecho5.cube.root=system_b", "techo5.cube.root=system_b_extra"} {
		if bootRequested(s) {
			t.Fatalf("unexpected boot for %q", s)
		}
	}
	if !bootRequested("console=tty0 techo5.cube.root=system_b other=1") {
		t.Fatal("explicit flag ignored")
	}
}

func TestPartitionRequiresUniqueSystemB(t *testing.T) {
	root := t.TempDir()
	add := func(name, data string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.Mkdir(p, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "uevent"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	add("mmcblk0p3", "PARTNAME=system_b\nDEVNAME=mmcblk0p3\n")
	add("mmcblk0p4", "PARTNAME=system_a\nDEVNAME=mmcblk0p4\n")
	got, err := partition(root, "/dev")
	if err != nil || got != "/dev/mmcblk0p3" {
		t.Fatalf("%q %v", got, err)
	}
	add("mmcblk1p3", "PARTNAME=system_b\nDEVNAME=mmcblk1p3\n")
	if _, err := partition(root, "/dev"); err == nil {
		t.Fatal("ambiguous partition accepted")
	}
}

func TestRootRequiresMarkerAndMountPoints(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"etc", "sbin", "dev", "proc", "sys"} {
		if err := os.Mkdir(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sbin/init"), []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := validateRoot(root); err == nil {
		t.Fatal("missing marker accepted")
	}
	marker := filepath.Join(root, "etc/techo5-cube-root")
	if err := os.WriteFile(marker, []byte(rootMarker), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateRoot(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("other"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateRoot(root); err == nil {
		t.Fatal("incorrect marker accepted")
	}
}
