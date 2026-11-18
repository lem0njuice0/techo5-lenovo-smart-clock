//go:build cube_update && cube_boot

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateTransfer(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "techo5")
	old := []byte("old daemon")
	next := []byte("new daemon")
	if err := os.WriteFile(dest, old, 0755); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(next))
	tr, err := beginTransfer(dir, int64(len(next)), sum)
	if err != nil {
		t.Fatal(err)
	}
	if err = tr.data(base64.StdEncoding.EncodeToString(next)); err != nil {
		t.Fatal(err)
	}
	if err = tr.commit(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != string(next) {
		t.Fatalf("new daemon %q: %v", got, err)
	}
	backup := fmt.Sprintf("%s.before-%x", dest, sha256.Sum256(old))
	got, err = os.ReadFile(backup)
	if err != nil || string(got) != string(old) {
		t.Fatalf("backup %q: %v", got, err)
	}
}
func TestUpdateRejectsBadDigest(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "techo5")
	_ = os.WriteFile(dest, []byte("old"), 0755)
	tr, err := beginTransfer(dir, 3, fmt.Sprintf("%x", sha256.Sum256([]byte("new"))))
	if err != nil {
		t.Fatal(err)
	}
	defer tr.abort()
	if err = tr.data(base64.StdEncoding.EncodeToString([]byte("bad"))); err != nil {
		t.Fatal(err)
	}
	if tr.commit() == nil {
		t.Fatal("accepted wrong digest")
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "old" {
		t.Fatalf("original changed: %q", got)
	}
}

func TestUpdateRootFollowsUnambiguousBootSlot(t *testing.T) {
	for _, c := range []struct{ cmd, want string }{
		{"androidboot.slot_suffix=_a", "system_a"},
		{"x=1 androidboot.slot_suffix=_b", "system_b"},
		{"", ""},
		{"androidboot.slot_suffix=_a androidboot.slot_suffix=_b", ""},
		{"androidboot.slot_suffix=_z", ""},
	} {
		got, err := activeUpdateRoot(c.cmd)
		if got != c.want || (err != nil) != (c.want == "") {
			t.Errorf("%q: got %q, %v", c.cmd, got, err)
		}
	}
}
