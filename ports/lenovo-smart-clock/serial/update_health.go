//go:build cube_slot

package main

import (
	"context"
	"errors"
	"time"
)

// cubeTrialHealth records only continuous health of one daemon instance. A
// failed check, restarted daemon or backwards clock resets the window. The
// eventual caller must additionally authenticate the pending manifest and
// match it to the current slot before marking the slot successful.
type cubeTrialHealth struct {
	pid         int
	startTicks  uint64
	since, last time.Time
}

func (h *cubeTrialHealth) observe(now time.Time, pid int, startTicks uint64, healthy bool, required, maxGap time.Duration) bool {
	if !healthy || pid <= 0 || startTicks == 0 || required <= 0 || maxGap <= 0 || (!h.last.IsZero() && (now.Before(h.last) || now.Sub(h.last) > maxGap)) {
		*h = cubeTrialHealth{}
		return false
	}
	if h.pid != pid || h.startTicks != startTicks || h.since.IsZero() {
		h.pid, h.startTicks, h.since = pid, startTicks, now
	}
	h.last = now
	return now.Sub(h.since) >= required
}

// awaitCubeHealth is separate from slot writes so timing, cancellation and
// unhealthy trials can be tested without accessing a device.
func awaitCubeHealth(ctx context.Context, interval, required, maxGap, timeout time.Duration, probe func() (int, uint64, bool, error)) (int, uint64, error) {
	if interval <= 0 || required <= 0 || maxGap < interval || timeout < required || probe == nil {
		return 0, 0, errors.New("invalid trial health configuration")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var h cubeTrialHealth
	for {
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		case <-deadline.C:
			return 0, 0, errCubeTrialUnhealthy
		case now := <-ticker.C:
			pid, start, healthy, err := probe()
			if err != nil {
				return 0, 0, err
			}
			if h.observe(now, pid, start, healthy, required, maxGap) {
				return pid, start, nil
			}
		}
	}
}
