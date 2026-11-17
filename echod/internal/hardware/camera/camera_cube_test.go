//go:build cube

package camera

import "testing"

func TestCubeHasNoCameraEvenWithDeviceNodes(t *testing.T) {
	old := nodes
	nodes = []string{"/dev/null"}
	t.Cleanup(func() { nodes = old })
	if Available() {
		t.Fatal("cube must not expose a camera")
	}
	if _, err := Get().Acquire(); err == nil {
		t.Fatal("cube must reject camera acquisition")
	}
}
