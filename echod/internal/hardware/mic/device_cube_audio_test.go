//go:build cube && !dot && !spot && !panel

package mic

import (
	"encoding/binary"
	"math"
	"testing"
)

// Verify the existing 48k -> 16k conversion against synthetic stereo samples;
// this does not establish the physical ADC's S32 alignment or amplitude.
func TestCubeCapturePrototypeResamplesStereo(t *testing.T) {
	old := boardKernel
	boardKernel = &cubeCapturePrototype
	t.Cleanup(func() { boardKernel = old })
	c := newPairedCapture(nil, nil, 48000)
	const frames = FrameSamples
	var result []byte
	for block := 0; block < 20; block++ {
		input := make([]byte, frames*3*2*4)
		for i := 0; i < frames*3; i++ {
			v := int32(math.Sin(2*math.Pi*1000*float64(block*frames*3+i)/48000) * (1 << 30))
			binary.LittleEndian.PutUint32(input[i*8:], uint32(v))
			binary.LittleEndian.PutUint32(input[i*8+4:], uint32(-v))
		}
		out := make([]byte, frames*Channels*Bits/8)
		c.convert(out, input, nil)
		result = append(result, out...)
	}
	got := Decode(result)
	const skip = FrameSamples * 2
	left, right := rmsInt16(got[0], skip), rmsInt16(got[1], skip)
	expected := float64(math.MaxInt16) * 0.5 / math.Sqrt2
	if math.Abs(left-expected) > expected*0.03 || math.Abs(right-expected) > expected*0.03 {
		t.Fatalf("resampled levels %f/%f, expected near %f", left, right, expected)
	}
	for _, ch := range Reference(result) {
		if rmsInt16(ch, skip) != 0 {
			t.Fatal("missing playback reference should be silent")
		}
	}
}
