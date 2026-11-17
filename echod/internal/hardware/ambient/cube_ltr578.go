//go:build cube && !dot && !spot && !panel

package ambient

import (
	"fmt"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/lib/i2c"
)

// The reference board has an LTR-578ALS at I2C0 address 0x53. The clear-window
// lux estimate is 0.8*counts/(18*4), for 18x gain and 400ms integration. The
// clock's dark cover needs an unknown correction factor; this reading is not
// calibrated and must be checked on the 2019 device.
const cubeLuxPerCount = 0.8 / (18.0 * 4.0)

type cubeLightBus interface {
	ReadReg(uint8) (uint8, error)
	WriteReg(uint8, uint8) error
	Close() error
}

type cubeLight struct{ bus cubeLightBus }

func setupCubeLight(b cubeLightBus) error {
	id, err := b.ReadReg(0x06)
	if err != nil {
		return err
	}
	if id != 0xb1 {
		return fmt.Errorf("unexpected LTR-578 part ID 0x%02x", id)
	}
	for _, p := range [][2]uint8{{0x00, 0x00}, {0x04, 0x05}, {0x05, 0x04}, {0x00, 0x02}} {
		if err := b.WriteReg(p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}

// Read high/middle before and after the low byte; retry the next poll if a
// conversion crossed a byte boundary. Reserved high bits are not data.
func cubeLightCount(b cubeLightBus) (int, bool, error) {
	hi1, err := b.ReadReg(0x0f)
	if err != nil {
		return 0, false, err
	}
	mid1, err := b.ReadReg(0x0e)
	if err != nil {
		return 0, false, err
	}
	lo, err := b.ReadReg(0x0d)
	if err != nil {
		return 0, false, err
	}
	mid2, err := b.ReadReg(0x0e)
	if err != nil {
		return 0, false, err
	}
	hi2, err := b.ReadReg(0x0f)
	if err != nil {
		return 0, false, err
	}
	if hi1&15 != hi2&15 || mid1 != mid2 {
		return 0, false, nil
	}
	return int(hi2&15)<<16 | int(mid2)<<8 | int(lo), true, nil
}

func (l *cubeLight) sample() (float64, bool, error) {
	n, ok, err := cubeLightCount(l.bus)
	return float64(n) * cubeLuxPerCount, ok, err
}
func (l *cubeLight) close() error { _ = l.bus.WriteReg(0x00, 0x00); return l.bus.Close() }

func startCubeLight(s *Sensor) error {
	b, err := i2c.Open(0, 0x53, false)
	if err != nil {
		return fmt.Errorf("ambient: %w", err)
	}
	if err := setupCubeLight(b); err != nil {
		_ = b.Close()
		return fmt.Errorf("ambient: %w", err)
	}
	time.Sleep(time.Second)
	s.mu.Lock()
	s.poll = &cubeLight{bus: b}
	s.mu.Unlock()
	return nil
}

func init() { boardStart = startCubeLight }
