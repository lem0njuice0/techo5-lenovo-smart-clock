//go:build cube_slot

package main

import (
	"encoding/json"
	"os"
	"testing"
)

// An explicitly provided private, read-only fixture exercises the preflight
// against a real device without embedding device identifiers in this source.
func TestCubeUpdatePreflightCapturedDevice(t *testing.T) {
	path := os.Getenv("CUBE_PREFLIGHT_FIXTURE")
	if path == "" {
		t.Skip("no private device fixture")
	}
	var f struct {
		Cmdline, Current, Mountinfo, RootMarker, EnabledMarker, DataMarker string
		SystemSize, BootSize                                               int64
		Partitions                                                         []struct {
			Name         string
			Major, Minor uint64
			Size         int64
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	var parts []cubePartition
	for _, p := range f.Partitions {
		parts = append(parts, cubePartition{p.Name, p.Major, p.Minor, p.Size})
	}
	if err := validateCubeUpdateLayout(f.Cmdline, f.Current, parts, f.Mountinfo, []byte(f.RootMarker), []byte(f.EnabledMarker), []byte(f.DataMarker), f.SystemSize, f.BootSize); err != nil {
		t.Fatalf("captured device did not pass preflight: %v", err)
	}
}
