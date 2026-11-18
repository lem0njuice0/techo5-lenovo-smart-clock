//go:build cube_slot && cube_update_trial

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCubeCandidateRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root-owned fixture")
	}
	d := t.TempDir()
	key := bytes.Repeat([]byte{1}, 32)
	for _, p := range []string{"etc/techo5-cube", "usr/local/sbin"} {
		if err := os.MkdirAll(filepath.Join(d, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string][]byte{"etc/techo5-cube-root": []byte("TECHO5-CUBE-ROOT-V1\n"), "etc/techo5-cube/enable-shared-data": []byte("TECHO5-CUBE-SHARED-ENABLE-V1\n"), "etc/techo5-cube/update-signing.pub": key, "etc/inittab": []byte("::respawn:/bin/sh /usr/local/sbin/cube-update-watch\n"), "usr/local/sbin/cube-update-watch": []byte("cube-serial ota-watch"), "usr/local/sbin/cube-serial": []byte("ota-fetch ota-check ota-update ota-stage ota-watch")}
	for p, data := range files {
		mode := os.FileMode(0644)
		if p == "usr/local/sbin/cube-serial" {
			mode = 0755
		}
		if err := os.WriteFile(filepath.Join(d, p), data, mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkCubeCandidateRoot(d, key); err != nil {
		t.Fatal(err)
	}
	if err := checkCubeCandidateRoot(d, bytes.Repeat([]byte{2}, 32)); err == nil {
		t.Fatal("accepted another key")
	}
	if err := os.WriteFile(filepath.Join(d, "etc/inittab"), []byte("no watcher\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkCubeCandidateRoot(d, key); err == nil {
		t.Fatal("accepted missing watcher")
	}
	if err := os.WriteFile(filepath.Join(d, "etc/inittab"), files["etc/inittab"], 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "usr/local/sbin/cube-serial"), []byte("ota-fetch ota-check ota-update ota-stage ota-watch CUBE-OTA-FAULT-TEST-DO-NOT-RELEASE"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := checkCubeCandidateRoot(d, key); err == nil {
		t.Fatal("accepted fault-injection image")
	}
}

func TestCubeCandidateImageReadOnlyLive(t *testing.T) {
	if os.Getenv("CUBE_LIVE_IMAGE") == "" {
		t.Skip("no live candidate image")
	}
	f, err := os.Open(os.Getenv("CUBE_LIVE_IMAGE"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	key, err := os.ReadFile(os.Getenv("CUBE_LIVE_PUBLIC_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkCubeCandidateImage(f, key); err != nil {
		t.Fatal(err)
	}
}
