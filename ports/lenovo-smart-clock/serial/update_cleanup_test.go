//go:build cube_slot && cube_update_trial

package main

import "testing"

func TestCleanupDoesNotDeleteDownloadedRetry(t *testing.T) {
	if eligibleCubeCleanup("b", "a", 3, 3, false) {
		t.Fatal("accepted an unstaged retry")
	}
	if eligibleCubeCleanup("a", "a", 3, 4, false) {
		t.Fatal("accepted an unaccepted sequence")
	}
	if !eligibleCubeCleanup("a", "a", 3, 3, false) {
		t.Fatal("confirmed update not eligible")
	}
	if !eligibleCubeCleanup("b", "a", 3, 3, true) {
		t.Fatal("resolved failure not eligible")
	}
}
