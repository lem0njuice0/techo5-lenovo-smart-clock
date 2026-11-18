//go:build cube_shared

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreLocalOnlyAfterSharedFailure(t *testing.T) {
	d := t.TempDir()
	marker := filepath.Join(d, "marker")
	failure := filepath.Join(d, "failure")
	if err := os.WriteFile(marker, []byte(sharedMarkerValue), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreLocal(marker, failure); err == nil {
		t.Fatal("accepted absent failure")
	}
	if err := os.WriteFile(failure, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreLocal(marker, failure); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("marker was not disabled")
	}
	if b, err := os.ReadFile(marker + ".disabled"); err != nil || string(b) != sharedMarkerValue {
		t.Fatalf("disabled marker: %q %v", b, err)
	}
}

func TestRestoreLocalRefusesInvalidOrOverwritingMarker(t *testing.T) {
	d := t.TempDir()
	marker := filepath.Join(d, "marker")
	failure := filepath.Join(d, "failure")
	_ = os.WriteFile(failure, nil, 0600)
	_ = os.WriteFile(marker, []byte("wrong"), 0600)
	if restoreLocal(marker, failure) == nil {
		t.Fatal("accepted invalid marker")
	}
	_ = os.WriteFile(marker, []byte(sharedMarkerValue), 0600)
	_ = os.WriteFile(marker+".disabled", []byte("keep"), 0600)
	if restoreLocal(marker, failure) == nil {
		t.Fatal("overwrote existing marker")
	}
	if b, _ := os.ReadFile(marker + ".disabled"); string(b) != "keep" {
		t.Fatal("changed existing marker")
	}
}
