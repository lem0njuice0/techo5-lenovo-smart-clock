//go:build !cube_slot || !cube_update_trial

package main

import "errors"

func runOptionalUpdateCommand([]string) error {
	return errors.New("experimental update commands are not enabled")
}
