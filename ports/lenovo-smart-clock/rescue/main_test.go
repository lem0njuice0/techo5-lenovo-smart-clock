package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsAreReadOnlyAndBounded(t *testing.T) {
	reads := 0
	read := func(p string) ([]byte, error) {
		reads++
		if p != "/proc/version" {
			t.Fatalf("unexpected read: %s", p)
		}
		return []byte("kernel"), nil
	}
	if got := command("version", read); got != "kernel\n" {
		t.Fatal(got)
	}
	for _, s := range []string{"help", "status", "rm /dev/mmcblk0", "", "cat /etc/shadow"} {
		_ = command(s, read)
	}
	if reads != 1 {
		t.Fatalf("unexpected reads: %d", reads)
	}
	if got := command("partitions", func(string) ([]byte, error) { return nil, errors.New("missing") }); !strings.Contains(got, "unavailable") {
		t.Fatal(got)
	}
}

func TestHardwareCommandsReadOnlyFixedPaths(t *testing.T) {
	for commandName, want := range map[string]string{"input": "/proc/bus/input/devices", "sound": "/proc/asound/cards", "pcm": "/proc/asound/pcm", "framebuffer": "/proc/fb"} {
		got := command(commandName, func(path string) ([]byte, error) {
			if path != want {
				t.Fatalf("%s read %s, want %s", commandName, path, want)
			}
			return []byte("ok"), nil
		})
		if got != "ok\n" {
			t.Fatal(got)
		}
	}
}

func TestStatusReportsRootFailure(t *testing.T) {
	old := bootStatus
	t.Cleanup(func() { bootStatus = old })
	bootStatus = "root startup failed: root validation: missing marker"
	if got := command("status", func(string) ([]byte, error) { t.Fatal("status read a file"); return nil, nil }); !strings.Contains(got, "missing marker") {
		t.Fatal(got)
	}
}

func TestGadgetCreatesSerialOnly(t *testing.T) {
	base := t.TempDir()
	root, udc, mode := filepath.Join(base, "gadget"), filepath.Join(base, "udc"), filepath.Join(base, "swmode")
	for _, d := range []string{root, udc} {
		if err := os.Mkdir(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(mode, []byte("host"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(udc, "controller"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := gadget(root, udc, mode); err != nil {
		t.Fatal(err)
	}
	g := filepath.Join(root, "cube-rescue")
	if b, err := os.ReadFile(filepath.Join(g, "UDC")); err != nil || string(b) != "controller" {
		t.Fatalf("UDC %q %v", b, err)
	}
	if _, err := os.Lstat(filepath.Join(g, "configs/c.1/acm.usb0")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(g, "functions/ecm.usb0")); !os.IsNotExist(err) {
		t.Fatalf("unexpected network function: %v", err)
	}
}
