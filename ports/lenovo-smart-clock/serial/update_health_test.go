//go:build cube_slot

package main

import (
	"testing"
	"time"
)

func TestCubeTrialHealthRequiresContinuousSameProcess(t *testing.T) {
	var h cubeTrialHealth
	now := time.Unix(1000, 0)
	wait := 2 * time.Minute
	gap := 90 * time.Second
	if h.observe(now, 42, 1, true, wait, gap) || h.observe(now.Add(time.Minute), 42, 1, true, wait, gap) {
		t.Fatal("accepted before health window")
	}
	if h.observe(now.Add(2*time.Minute), 43, 2, true, wait, gap) {
		t.Fatal("accepted restarted daemon")
	}
	if h.observe(now.Add(3*time.Minute), 43, 2, false, wait, gap) {
		t.Fatal("accepted unhealthy daemon")
	}
	if h.observe(now.Add(4*time.Minute), 43, 2, true, wait, gap) || h.observe(now.Add(5*time.Minute), 43, 2, true, wait, gap) {
		t.Fatal("failed check did not reset health window")
	}
	if !h.observe(now.Add(6*time.Minute), 43, 2, true, wait, gap) {
		t.Fatal("healthy window did not complete")
	}
	if h.observe(now, 43, 2, true, wait, gap) {
		t.Fatal("accepted backwards clock")
	}
	if h.observe(now.Add(5*time.Minute), 43, 2, true, wait, gap) || h.observe(now.Add(7*time.Minute), 43, 2, true, wait, gap) {
		t.Fatal("accepted missed checks")
	}
	if h.observe(now.Add(8*time.Minute), 43, 2, true, wait, gap) || h.observe(now.Add(9*time.Minute), 43, 3, true, wait, gap) {
		t.Fatal("accepted reused process ID")
	}
}
