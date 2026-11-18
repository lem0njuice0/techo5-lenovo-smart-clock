//go:build cube_shared

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

const sharedMarker = "/etc/techo5-cube/enable-shared-data"
const sharedFailure = "/run/techo5/shared-data-error"
const sharedMarkerValue = "TECHO5-CUBE-SHARED-ENABLE-V1\n"

func sharedHelp() string { return ", shared-status, shared-restore-local" }

func regular(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func restoreLocal(marker, failure string) error {
	if !regular(failure) || !regular(marker) {
		return errors.New("shared-data failure or marker not present")
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != sharedMarkerValue {
		return errors.New("invalid shared-data marker")
	}
	disabled := marker + ".disabled"
	if _, err := os.Lstat(disabled); err == nil || !os.IsNotExist(err) {
		return errors.New("disabled marker already exists or cannot be checked")
	}
	if err := os.Rename(marker, disabled); err != nil {
		return err
	}
	syscall.Sync()
	return nil
}

func sharedCommand(s string) (string, bool) {
	switch s {
	case "shared-status":
		if !regular(sharedMarker) {
			return "shared data is not enabled\n", true
		}
		if regular(sharedFailure) {
			return "shared data failed; daemon held; USB local-data recovery available\n", true
		}
		return "shared data is enabled; verify /data mount before updating\n", true
	case "shared-restore-local":
		if err := restoreLocal(sharedMarker, sharedFailure); err != nil {
			return fmt.Sprintf("refused: %v\n", err), true
		}
		return "shared data disabled for next boot; local data may be stale; shared data was not changed\n", true
	}
	return "", false
}
