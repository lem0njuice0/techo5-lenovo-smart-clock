//go:build cube_slot

package main

import (
	"errors"
	"strings"
)

// canResolvePending permits cleanup only after a confirmed current trial or a
// failed target that the bootloader has marked completely unbootable. It is a
// pure check; the caller must authenticate the pending record, confirm its
// sequence and hold the update lock before removing anything.
func canResolvePending(record []byte, cmdline, current, target string) error {
	if err := slotRecord(record); err != nil {
		return err
	}
	if (current != "a" && current != "b") || (target != "a" && target != "b") {
		return errors.New("invalid slot")
	}
	n := 0
	for _, word := range strings.Fields(cmdline) {
		if strings.HasPrefix(word, "androidboot.slot_suffix=") {
			n++
			if word != "androidboot.slot_suffix=_"+current {
				return errors.New("boot slot mismatch")
			}
		}
	}
	if n != 1 {
		return errors.New("ambiguous boot slot")
	}
	active, other := 8, 12
	if current == "b" {
		active, other = 12, 8
	}
	if record[active] == 0 || record[active+2] != 1 {
		return errors.New("running slot is not confirmed")
	}
	if target == current {
		if record[other] == 0 || record[other+2] != 1 || record[active] <= record[other] {
			return errors.New("confirmed fallback unavailable")
		}
		return nil
	}
	if record[other] != 0 || record[other+1] != 0 || record[other+2] != 0 {
		return errors.New("target trial may still be bootable")
	}
	return nil
}
