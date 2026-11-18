//go:build cube_slot && cube_update_fault_test

package main

import (
	"fmt"
	"os"
	"time"
)

// Deliberately unhealthy, short-lived fault injection for a private recovery
// test only. Never use this build tag in a release or normal device image.
func cubeTrialTiming() (time.Duration, time.Duration, time.Duration, time.Duration, bool) {
	fmt.Fprintln(os.Stderr, "CUBE-OTA-FAULT-TEST-DO-NOT-RELEASE")
	return time.Second, 4 * time.Second, 3 * time.Second, 12 * time.Second, true
}
