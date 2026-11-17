//go:build cube && !dot && !spot && !panel

package layout

import "testing"

func TestCubeIdentity(t *testing.T) {
	if Board != "cube" || Model != "Lenovo Smart Clock (CD-24501F)" || DefaultName != "Lenovo Smart Clock" {
		t.Fatalf("unexpected cube identity: %q, %q, %q", Board, Model, DefaultName)
	}
}
