//go:build cube_slot

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCubeUpdateLock(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "update.lock")
	first, err := acquireCubeUpdateLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquireCubeUpdateLock(path); err == nil {
		second.Close()
		t.Fatal("accepted concurrent lock")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := acquireCubeUpdateLock(path)
	if err != nil {
		t.Fatal(err)
	}
	again.Close()
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if f, err := acquireCubeUpdateLock(path); err == nil {
		f.Close()
		t.Fatal("accepted public lock file")
	}
	link := filepath.Join(d, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if f, err := acquireCubeUpdateLock(link); err == nil {
		f.Close()
		t.Fatal("followed symlink")
	}
}

func TestCubeUpdateLockWaitsForBriefContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.lock")
	first, err := acquireCubeUpdateLock(path)
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(100 * time.Millisecond); first.Close() }()
	second, err := waitCubeUpdateLock(path, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
}
