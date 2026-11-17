package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCommandsAreLimitedAndReadOnly(t *testing.T) {
	stat := func(p string) (os.FileInfo, error) { return nil, errors.New("absent") }
	read := func(p string) ([]byte, error) {
		if p == "/etc/techo5-cube-root" {
			return []byte("TECHO5-CUBE-ROOT-V1\n"), nil
		}
		if p == "/proc/cmdline" {
			return []byte("bootopt=test\n"), nil
		}
		t.Fatalf("unexpected file read %q", p)
		return nil, nil
	}
	if got := command("status", stat, read); !strings.Contains(got, "root active") {
		t.Fatal(got)
	}
	if got := command("nodes", stat, read); !strings.Contains(got, "/dev/fb0: unavailable") {
		t.Fatal(got)
	}
	if got := command("cmdline", stat, read); got != "bootopt=test\n" {
		t.Fatal(got)
	}
	if got := command("cat /etc/shadow", stat, read); !strings.Contains(got, "unknown") {
		t.Fatal(got)
	}
}
