//go:build cube && !dot && !spot && !panel

package speaker

// The reference describes I2S8CH playback at hw:0,0, 48 kHz stereo. The
// external TAS5805M must be initialized separately using the owner's matching
// 2019 data; these settings have not been checked on the user's hardware.
func init() {
	PlaybackDevice = 0
	hwRate = Rate
	DRAMHold = ""
	initSequence = nil
	openSequence = nil
	softRef = true
	DriverTuning = false
	gainOnPath = false
	HasJack = false
	HasBoth = false
	pathSequence, headphoneOff = showPaths(false)
}
