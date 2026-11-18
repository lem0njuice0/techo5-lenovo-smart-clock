//go:build cube_slot && cube_update_trial

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCubeIncomingFiles(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to test file ownership")
	}
	d := t.TempDir()
	path := filepath.Join(d, "image")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(d)
	if err != nil {
		t.Fatal(err)
	}
	device := uint64(info.Sys().(*syscall.Stat_t).Dev)
	f, err := openCubeIncoming(path, 16, device)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if f, err := openCubeIncoming(path, 2, device); err == nil {
		f.Close()
		t.Fatal("accepted oversized input")
	}
	if f, err := openCubeIncoming(path, 16, device+1); err == nil {
		f.Close()
		t.Fatal("accepted wrong filesystem")
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if f, err := openCubeIncoming(path, 16, device); err == nil {
		f.Close()
		t.Fatal("accepted writable input")
	}
	link := filepath.Join(d, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if f, err := openCubeIncoming(link, 16, device); err == nil {
		f.Close()
		t.Fatal("followed input symlink")
	}
	if err := runOptionalUpdateCommand([]string{"unknown"}); err == nil {
		t.Fatal("accepted unknown command")
	}
}
