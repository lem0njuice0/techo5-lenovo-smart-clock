//go:build cube_slot

package main

import (
	"strings"
	"testing"
)

func TestCubeUpdatePreflightRejectsUnsafeLayouts(t *testing.T) {
	parts := []cubePartition{{"misc", 179, 3, 4096}, {"userdata", 179, 29, 5000000}, {"system_b", 179, 18, 2048}, {"system_a", 179, 17, 2048}, {"boot_a", 179, 7, 8192}}
	base := "21 0 179:18 / / rw,relatime - ext4 /dev/root rw\n29 21 179:29 / /run/cube-userdata rw - ext4 /dev/userdata rw\n30 21 179:29 /techo5-cube/data /data rw - ext4 /dev/userdata rw\n"
	root, enable, data := []byte("TECHO5-CUBE-ROOT-V1\n"), []byte("TECHO5-CUBE-SHARED-ENABLE-V1\n"), []byte("TECHO5-CUBE-SHARED-DATA-V1\n")
	cmd := "androidboot.slot_suffix=_b techo5.cube.root=system_b"
	check := func(c string, p []cubePartition, m string, marker []byte) error {
		return validateCubeUpdateLayout(c, "b", p, m, root, enable, marker, 2048, 6144)
	}
	if err := check(cmd, parts, base, data); err != nil {
		t.Fatalf("valid layout: %v", err)
	}
	tests := []struct {
		name, cmd, mount string
		partitions       []cubePartition
		marker           []byte
	}{
		{"wrong root", "androidboot.slot_suffix=_b techo5.cube.root=system_a", base, parts, data},
		{"ambiguous slot", cmd + " androidboot.slot_suffix=_a", base, parts, data},
		{"missing marker", cmd, base, parts, nil},
		{"target mounted", cmd, base + "31 21 179:17 / /tmp/target rw - ext4 /dev/target rw\n", parts, data},
		{"duplicate data mount", cmd, base + "31 21 179:29 /other /data rw - ext4 /dev/userdata rw\n", parts, data},
		{"wrong data root", cmd, strings.Replace(base, "/techo5-cube/data /data", "/other /data", 1), parts, data},
		{"read-only data", cmd, strings.Replace(base, "/data rw -", "/data ro -", 1), parts, data},
		{"bad mountinfo", cmd, base + "invalid\n", parts, data},
		{"duplicate partition", cmd, base, append(append([]cubePartition(nil), parts...), parts[0]), data},
		{"missing partition", cmd, base, parts[:4], data},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := check(test.cmd, test.partitions, test.mount, test.marker); err == nil {
				t.Fatal("unsafe layout accepted")
			}
		})
	}
}
