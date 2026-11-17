//go:build cube && !dot && !spot && !panel

package ambient

import (
	"errors"
	"math"
	"testing"
)

type lightFixture struct {
	reads  []uint8
	writes [][2]uint8
	fail   bool
}

func (f *lightFixture) ReadReg(uint8) (uint8, error) {
	if f.fail || len(f.reads) == 0 {
		return 0, errors.New("read")
	}
	v := f.reads[0]
	f.reads = f.reads[1:]
	return v, nil
}
func (f *lightFixture) WriteReg(r, v uint8) error {
	f.writes = append(f.writes, [2]uint8{r, v})
	return nil
}
func (f *lightFixture) Close() error { return nil }
func TestCubeLightSetupAndCount(t *testing.T) {
	f := &lightFixture{reads: []uint8{0xb1}}
	if err := setupCubeLight(f); err != nil {
		t.Fatal(err)
	}
	want := [][2]uint8{{0, 0}, {4, 5}, {5, 4}, {0, 2}}
	if len(f.writes) != len(want) {
		t.Fatal(f.writes)
	}
	for i := range want {
		if f.writes[i] != want[i] {
			t.Fatal(f.writes)
		}
	}
	f.reads = []uint8{0xf2, 0x34, 0x56, 0x34, 0x02}
	count, ok, err := cubeLightCount(f)
	if err != nil || !ok || count != 0x23456 {
		t.Fatalf("%x %t %v", count, ok, err)
	}
	if math.Abs(float64(90)*cubeLuxPerCount-1) > 1e-12 {
		t.Fatal("unexpected clear-window scale")
	}
	f.reads = []uint8{0x02, 0x34, 0xff, 0x35, 0x02}
	if _, ok, err := cubeLightCount(f); err != nil || ok {
		t.Fatalf("torn sample accepted: %t %v", ok, err)
	}
	f.reads = []uint8{0xff}
	if err := setupCubeLight(f); err == nil {
		t.Fatal("unknown device accepted")
	}
}
