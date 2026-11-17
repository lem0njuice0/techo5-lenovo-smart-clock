//go:build cube && !dot && !spot && !panel

package layout

import (
	"bytes"
	"testing"
)

func TestCubeIdentifierUsesStableBootSerial(t *testing.T) {
	a, err := cubeID("console=x androidboot.serialno=ABC123 bootopt=x")
	if err != nil || MAC(string(a)) == "" {
		t.Fatalf("%q %v", a, err)
	}
	b, _ := cubeID("androidboot.serialno=ABC123")
	c, _ := cubeID("androidboot.serialno=XYZ987")
	if !bytes.Equal(a, b) || bytes.Equal(a, c) {
		t.Fatal("identity is not stable and distinct")
	}
	for _, s := range []string{"", "androidboot.serialno=", "androidboot.serialno=a androidboot.serialno=b", "androidboot.serialno=a/b"} {
		if _, err := cubeID(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}
