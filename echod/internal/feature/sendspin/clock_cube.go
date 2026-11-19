//go:build cube

package sendspin

import (
	ssync "github.com/Sendspin/sendspin-go/pkg/sync"
	"sync"
	"time"
)

// The clock has no persistent RTC. NTP may step wall time after a Sendspin
// session starts; the protocol's client timestamps must remain monotonic.
var cubeEpoch = time.Now()

// Measurements run every syncEvery, not continuously. Keep a valid sample
// through the next burst and tolerate up to two missed intervals; a five-second
// deadline incorrectly made the clock stale between ten-second measurements.
const cubeClockMaxAge = 3 * syncEvery

func micros() int64 { return cubeEpoch.UnixMicro() + time.Since(cubeEpoch).Microseconds() }

type cubeClock struct {
	mu       sync.Mutex
	filter   *ssync.TimeFilter
	lastSync time.Time
}

func newSendspinClock() sessionClock {
	return &cubeClock{filter: ssync.NewTimeFilter(ssync.DefaultTimeFilterConfig())}
}

func (c *cubeClock) ProcessSyncResponse(t1, t2, t3, t4 int64) {
	rtt := (t4 - t1) - (t3 - t2)
	if rtt < 0 || t4 < t1 || t3 < t2 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	measurement := ((t2 - t1) + (t3 - t4)) / 2
	if c.filter.Synced() {
		predicted := c.filter.ComputeServerTime(t4) - t4
		// A server clock reset or an old poisoned filter must not leave music
		// scheduled minutes away. A normal Wi-Fi RTT stays far below this.
		if difference := measurement - predicted; difference > 3000000 || difference < -3000000 {
			c.filter.Reset()
		}
	}
	c.filter.Update(measurement, max(1, rtt/2), t4)
	c.lastSync = time.Now()
}

func (c *cubeClock) CheckQuality() ssync.Quality {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastSync.IsZero() || time.Since(c.lastSync) > cubeClockMaxAge || !c.filter.Synced() {
		return ssync.QualityLost
	}
	err := c.filter.GetError()
	if err < 100 {
		return ssync.QualityGood
	}
	if err < 5000 {
		return ssync.QualityDegraded
	}
	return ssync.QualityLost
}

func (c *cubeClock) ServerMicrosNow() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := micros()
	if !c.filter.Synced() {
		return now
	}
	return c.filter.ComputeServerTime(now)
}

func (c *cubeClock) ServerToLocalTime(server int64) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	client := server
	if c.filter.Synced() {
		client = c.filter.ComputeClientTime(server)
	}
	// Preserve Go's monotonic clock when deriving the deadline. Never interpret
	// the synthetic client timestamp as the current wall-clock timestamp.
	return time.Now().Add(time.Duration(client-micros()) * time.Microsecond)
}
