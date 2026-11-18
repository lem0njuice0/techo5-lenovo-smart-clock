//go:build cube_slot && !cube_update_fault_test

package main

import "time"

func cubeTrialTiming() (time.Duration, time.Duration, time.Duration, time.Duration, bool) {
	return 5 * time.Second, 2 * time.Minute, 15 * time.Second, 10 * time.Minute, false
}
