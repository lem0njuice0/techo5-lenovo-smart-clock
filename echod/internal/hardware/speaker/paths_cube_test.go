//go:build cube && !dot && !spot && !panel

package speaker

import "testing"

func TestCubePlaybackProfile(t *testing.T) {
	if PlaybackDevice != 0 || hwRate != 48000 || DRAMHold != "" || !softRef || DriverTuning || HasJack || HasBoth {
		t.Fatalf("unexpected cube playback configuration")
	}
	if len(initSequence) != 0 || len(openSequence) != 0 || len(pathSequence[OutputSpeaker]) != 0 {
		t.Fatal("Echo mixer controls must not be applied to the clock")
	}
}
