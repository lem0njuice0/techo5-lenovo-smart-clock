//go:build cube && !dot && !spot && !panel

package privacy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCubeMuteFailsClosed(t *testing.T) {
	old := cubeMicSwitchPath
	oldSoft := softwareCut.Load()
	cubeMicSwitchPath = filepath.Join(t.TempDir(), "value")
	t.Cleanup(func() { cubeMicSwitchPath = old; softwareCut.Store(oldSoft) })
	softwareCut.Store(false)
	m := cubeMute{}
	if !SoftwareCut() {
		t.Fatal("missing GPIO must cut capture")
	}
	if err := m.Set(false); err == nil {
		t.Fatal("missing GPIO must not unmute")
	}
	for _, s := range []string{"0\n", "unexpected\n", ""} {
		if err := os.WriteFile(cubeMicSwitchPath, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
		if !SoftwareCut() {
			t.Fatalf("value %q did not cut", s)
		}
	}
	if err := os.WriteFile(cubeMicSwitchPath, []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(false); err != nil || SoftwareCut() {
		t.Fatalf("valid live switch: %v", err)
	}
	if err := m.Set(true); err != nil || !SoftwareCut() {
		t.Fatalf("software mute: %v", err)
	}
	if err := m.Set(false); err != nil || SoftwareCut() {
		t.Fatalf("software unmute: %v", err)
	}
	Seed(true)
	if !SoftwareCut() {
		t.Fatal("saved mute must be seeded before capture")
	}
}
