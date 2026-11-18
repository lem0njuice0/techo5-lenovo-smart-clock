//go:build cube_slot

package main

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"strings"
)

// slotTransition is an offline metadata calculation only. No caller writes
// these records yet: an updater must first validate device identity and shared
// data, then sync and read back each write, image, and boot image in order.
func slotTransition(record []byte, cmdline, current, phase string) ([]byte, error) {
	if err := slotRecord(record); err != nil {
		return nil, err
	}
	if current != "a" && current != "b" {
		return nil, errors.New("invalid current slot")
	}
	n := 0
	for _, word := range strings.Fields(cmdline) {
		if strings.HasPrefix(word, "androidboot.slot_suffix=") {
			n++
			if word != "androidboot.slot_suffix=_"+current {
				return nil, errors.New("running slot differs")
			}
		}
	}
	if n != 1 {
		return nil, errors.New("missing or ambiguous running slot")
	}
	active, target := 8, 12
	if current == "b" {
		active, target = 12, 8
	}
	if record[active] == 0 || record[active+2] != 1 || record[active] < record[target] {
		return nil, errors.New("current slot is not preferred and successful")
	}
	out := append([]byte(nil), record...)
	switch phase {
	case "prepare":
		// The inactive slot cannot be selected while it is being overwritten.
		out[target], out[target+1], out[target+2] = 0, 0, 0
	case "activate":
		if record[target] != 0 || record[target+1] != 0 || record[target+2] != 0 {
			return nil, errors.New("target was not prepared")
		}
		out[active] = 14
		out[target], out[target+1], out[target+2] = 15, 3, 0
	default:
		return nil, errors.New("invalid transition phase")
	}
	binary.BigEndian.PutUint32(out[28:], crc32.ChecksumIEEE(out[:28]))
	return out, nil
}
