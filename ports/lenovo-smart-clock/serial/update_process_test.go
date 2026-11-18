//go:build cube_slot

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindTrialDaemon(t *testing.T) {
	d := t.TempDir()
	add := func(id int, state string, start int) {
		p := filepath.Join(d, fmt.Sprint(id))
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
		stat := fmt.Sprintf("%d (name with ) chars) %s %s %d 0\n", id, state, strings.Repeat("0 ", 18), start)
		if err := os.WriteFile(filepath.Join(p, "stat"), []byte(stat), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/usr/local/bin/techo5", filepath.Join(p, "exe")); err != nil {
			t.Fatal(err)
		}
	}
	add(42, "S", 123)
	pid, start, err := findTrialDaemon(d, "/usr/local/bin/techo5")
	if err != nil || pid != 42 || start != 123 {
		t.Fatalf("bad process: %d %d %v", pid, start, err)
	}
	add(43, "Z", 124)
	if _, _, err := findTrialDaemon(d, "/usr/local/bin/techo5"); err != nil {
		t.Fatal(err)
	}
	add(44, "S", 125)
	if _, _, err := findTrialDaemon(d, "/usr/local/bin/techo5"); err == nil {
		t.Fatal("accepted multiple daemons")
	}
}

func TestFindTrialDaemonReadOnlyLive(t *testing.T) {
	if os.Getenv("CUBE_LIVE_READONLY") != "1" {
		t.Skip("no live read-only test")
	}
	pid, start, err := findTrialDaemon("/proc", "/usr/local/bin/techo5")
	if err != nil || pid <= 0 || start == 0 {
		t.Fatalf("daemon check failed: %v", err)
	}
}
