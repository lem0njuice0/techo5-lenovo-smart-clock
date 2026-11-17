//go:build cube && !dot && !spot && !panel

package touch

import "testing"

func TestCubeTouchCorners(t *testing.T) {
	for _, c := range []struct{ x, y, wantX, wantY int }{
		{480, 0, 0, 0}, {480, 800, 799, 0},
		{0, 0, 0, 479}, {0, 800, 799, 479},
	} {
		x, y := toFrame(481, 801, c.x, c.y)
		if x != c.wantX || y != c.wantY {
			t.Errorf("(%d,%d) got (%d,%d), want (%d,%d)", c.x, c.y, x, y, c.wantX, c.wantY)
		}
	}
}

func rawFrame() (int, int, func(int, int) (int, int)) {
	return Height, Width, func(x, y int) (int, int) { return Height - 1 - y, x }
}
