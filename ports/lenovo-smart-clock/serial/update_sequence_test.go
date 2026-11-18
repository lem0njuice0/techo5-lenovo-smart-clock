//go:build cube_slot

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCubeUpdateSequence(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "sequence")
	if _, err := readCubeSequence(path); err == nil {
		t.Fatal("accepted missing sequence")
	}
	if err := os.WriteFile(path, []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := advanceCubeSequence(path, 3); err != nil {
		t.Fatal(err)
	}
	if n, err := readCubeSequence(path); err != nil || n != 3 {
		t.Fatalf("bad stored sequence %d %v", n, err)
	}
	if err := advanceCubeSequence(path, 2); err == nil {
		t.Fatal("accepted downgrade")
	}
	if err := advanceCubeSequence(path, 3); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("03\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCubeSequence(path); err == nil {
		t.Fatal("accepted noncanonical sequence")
	}
	link := filepath.Join(d, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readCubeSequence(link); err == nil {
		t.Fatal("followed sequence symlink")
	}
}
