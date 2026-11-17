//go:build cube && !dot && !spot && !panel

package buttons

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCubeDebounce(t *testing.T) {
	t0 := time.Unix(10, 0)
	d := cubeDebounce{since: t0}
	if changed, _ := d.update(true, t0); changed {
		t.Fatal("immediate transition")
	}
	if changed, _ := d.update(false, t0.Add(10*time.Millisecond)); changed {
		t.Fatal("bounce transition")
	}
	d.update(true, t0.Add(20*time.Millisecond))
	if changed, _ := d.update(true, t0.Add(50*time.Millisecond)); changed {
		t.Fatal("too early")
	}
	if changed, value := d.update(true, t0.Add(60*time.Millisecond)); !changed || !value {
		t.Fatal("stable press not reported")
	}
}
func TestCubeGPIOValidation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "value")
	if _, err := cubePin(p); err == nil {
		t.Fatal("missing GPIO accepted")
	}
	if err := os.WriteFile(p, []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if v, err := cubePin(p); err != nil || v != "1" {
		t.Fatalf("%q %v", v, err)
	}
	if err := os.WriteFile(p, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := cubePin(p); err == nil {
		t.Fatal("invalid GPIO accepted")
	}
}
