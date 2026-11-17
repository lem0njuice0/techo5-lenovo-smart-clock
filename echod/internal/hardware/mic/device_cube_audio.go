//go:build cube && !dot && !spot && !panel

package mic

// cubeCapturePrototype records the published Linux bring-up's PCM format.
// ADC initialization must be done by the board startup; sample alignment,
// DMA ring size, and the physical mute switch still need hardware measurement.
var cubeCapturePrototype = athenaKernel{
	micDevice:   1,
	micChannels: 2,
	micBits:     32,
	micFull:     1 << 31, // provisional; verify converter alignment on hardware
	refDevice:   -1,
	softRef:     true,
	micRates:    []int{48000},
	refRate:     Rate,
	micPeriods:  3,
}

func init() {
	boardKernel = &cubeCapturePrototype
	// The ADC is programmed over I2C by board startup, not by Echo mixer controls.
	adcs = nil
}
