//go:build cube_slot

package main

import "time"

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
