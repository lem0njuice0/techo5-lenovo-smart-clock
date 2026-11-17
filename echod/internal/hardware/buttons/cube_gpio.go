//go:build cube && !dot && !spot && !panel

package buttons

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Reference-board GPIO numbers and polarities require checks on the 2019 unit.
var cubePins = []struct {
	path   string
	name   Name
	active string
}{
	{"/sys/class/gpio/gpio429/value", VolumeUp, "0"},
	{"/sys/class/gpio/gpio576/value", VolumeDown, "1"},
	{"/sys/class/gpio/gpio410/value", Mute, "0"},
}

func cubePin(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v != "0" && v != "1" {
		return "", fmt.Errorf("invalid GPIO value %q", v)
	}
	return v, nil
}

type cubeDebounce struct {
	stable, candidate bool
	since             time.Time
}

func (d *cubeDebounce) update(value bool, now time.Time) (bool, bool) {
	if value != d.candidate {
		d.candidate = value
		d.since = now
		return false, d.stable
	}
	if value != d.stable && now.Sub(d.since) >= 40*time.Millisecond {
		d.stable = value
		return true, value
	}
	return false, d.stable
}

func startCubeButtons(*Controller) error {
	for _, pin := range cubePins {
		if _, err := cubePin(pin.path); err != nil {
			return fmt.Errorf("buttons: %s: %w", pin.path, err)
		}
	}
	return nil
}

func runCubeButtons(ctx context.Context, c *Controller) error {
	states := make([]cubeDebounce, len(cubePins))
	heldButtons := make(map[Name]*held)
	defer func() {
		for n, h := range heldButtons {
			h.stop()
			c.mark(n, false)
		}
	}()
	now := time.Now()
	for i, p := range cubePins {
		v, err := cubePin(p.path)
		if err != nil {
			return err
		}
		active := v == p.active
		states[i] = cubeDebounce{stable: active, candidate: active, since: now}
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			for i, p := range cubePins {
				v, err := cubePin(p.path)
				if err != nil {
					return fmt.Errorf("buttons: %s: %w", p.path, err)
				}
				changed, active := states[i].update(v == p.active, now)
				if !changed {
					continue
				}
				if p.name == Mute {
					c.Events.Emit(Event{Name: Mute, Kind: Tap})
					continue
				}
				if active {
					heldButtons[p.name] = c.pressed(p.name)
				} else if h := heldButtons[p.name]; h != nil {
					delete(heldButtons, p.name)
					c.released(p.name, h)
				}
			}
		}
	}
}

func init() {
	codes = map[uint16]Name{114: VolumeDown, 115: VolumeUp, 116: Mute}
	boardStart = startCubeButtons
	boardRun = runCubeButtons
}
