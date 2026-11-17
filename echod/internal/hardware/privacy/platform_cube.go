//go:build cube && !dot && !spot && !panel

package privacy

import (
	"errors"
	"os"
	"strings"
	"time"
)

// The reference implementation reports the rear switch at GPIO 410, active
// low. This numbering and polarity must be verified on the 2019 unit. Capture
// is cut whenever the switch is missing, unreadable, or has an invalid value.
var cubeMicSwitchPath = "/sys/class/gpio/gpio410/value"

func cubePhysicalCut() bool {
	b, err := os.ReadFile(cubeMicSwitchPath)
	return err != nil || strings.TrimSpace(string(b)) != "1"
}

type cubeMute struct{}

func (cubeMute) Get() (bool, error) { return SoftwareCut(), nil }
func (cubeMute) Set(muted bool) error {
	if !muted && cubePhysicalCut() {
		return errors.New("privacy: physical microphone switch is muted or unavailable")
	}
	softwareCut.Store(muted)
	return nil
}
func (m cubeMute) Toggle() (bool, error) {
	next := !SoftwareCut()
	return next, m.Set(next)
}
func (cubeMute) HardwareActs(bool) bool { return true }
func (cubeMute) Lag() time.Duration     { return 0 }

func (cubeMute) Seed(muted bool) { softwareCut.Store(muted) }

func init() {
	boardMute = cubeMute{}
	boardCut = cubePhysicalCut

}
