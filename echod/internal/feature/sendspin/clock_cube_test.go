//go:build cube

package sendspin

import (
	ssync "github.com/Sendspin/sendspin-go/pkg/sync"
	"testing"
	"time"
)

func TestCubeClockMapsServerDeadlineToMonotonicTime(t *testing.T) {
	c := newSendspinClock().(*cubeClock)
	if c.CheckQuality() != ssync.QualityLost {
		t.Fatal("unsynchronized clock reported ready")
	}
	const offset = int64(-1700000000000000)
	for range 4 {
		t1 := micros()
		c.ProcessSyncResponse(t1, t1+offset+100, t1+offset+120, t1+220)
		time.Sleep(time.Millisecond)
	}
	if c.CheckQuality() == ssync.QualityLost {
		t.Fatal("valid sync samples were not accepted")
	}
	serverNow := c.ServerMicrosNow()
	deadline := c.ServerToLocalTime(serverNow + 500000)
	if d := time.Until(deadline); d < 450*time.Millisecond || d > 550*time.Millisecond {
		t.Fatalf("deadline off by %v", d)
	}
	c.mu.Lock()
	c.lastSync = time.Now().Add(-6 * time.Second)
	c.mu.Unlock()
	if c.CheckQuality() != ssync.QualityLost {
		t.Fatal("stale clock reported ready")
	}
}

func TestCubeClockRejectsInvalidRoundTrip(t *testing.T) {
	c := newSendspinClock().(*cubeClock)
	c.ProcessSyncResponse(1000, 100, 900, 1100)
	if c.CheckQuality() != ssync.QualityLost {
		t.Fatal("invalid round trip accepted")
	}
}

func TestCubeClockRecoversFromServerTimeDiscontinuity(t *testing.T) {
	c := newSendspinClock().(*cubeClock)
	for range 3 {
		t1 := micros()
		c.ProcessSyncResponse(t1, t1+100, t1+120, t1+220)
		time.Sleep(time.Millisecond)
	}
	t1 := micros()
	const jump = int64(60 * 1000000)
	c.ProcessSyncResponse(t1, t1+jump+100, t1+jump+120, t1+220)
	if delta := c.ServerMicrosNow() - micros() - jump; delta < -10000 || delta > 10000 {
		t.Fatalf("did not follow new server clock, error=%d microseconds", delta)
	}
}
