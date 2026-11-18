//go:build cube_slot

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// parseTrialProcessStat extracts the PID and Linux starttime, so a reused PID
// cannot be mistaken for a continuously healthy daemon.
func parseTrialProcessStat(data string) (int, uint64, error) {
	open := strings.IndexByte(data, '(')
	close := strings.LastIndexByte(data, ')')
	if open <= 0 || close <= open || close+1 >= len(data) {
		return 0, 0, errors.New("invalid process stat")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(data[:open]))
	if err != nil || pid <= 0 {
		return 0, 0, errors.New("invalid process ID")
	}
	fields := strings.Fields(data[close+1:])
	if len(fields) < 20 || fields[0] == "Z" || fields[0] == "X" {
		return 0, 0, errors.New("process not running")
	}
	start, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || start == 0 {
		return 0, 0, errors.New("invalid process start time")
	}
	return pid, start, nil
}

// findTrialDaemon refuses missing or multiple matching daemons. This is only
// one health signal; mount, display and authenticated pending-update checks
// remain necessary before accepting a trial boot.
func findTrialDaemon(procroot, executable string) (int, uint64, error) {
	entries, err := os.ReadDir(procroot)
	if err != nil {
		return 0, 0, err
	}
	pidFound := 0
	var started uint64
	for _, entry := range entries {
		id, err := strconv.Atoi(entry.Name())
		if err != nil || id <= 0 {
			continue
		}
		base := filepath.Join(procroot, entry.Name())
		before, err := os.ReadFile(filepath.Join(base, "stat"))
		if err != nil {
			continue
		}
		pid, start, err := parseTrialProcessStat(string(before))
		if err != nil || pid != id {
			continue
		}
		name, err := os.Readlink(filepath.Join(base, "exe"))
		if err != nil || name != executable {
			continue
		}
		after, err := os.ReadFile(filepath.Join(base, "stat"))
		if err != nil {
			continue
		}
		p2, s2, err := parseTrialProcessStat(string(after))
		if err != nil || p2 != pid || s2 != start {
			continue
		}
		if pidFound != 0 {
			return 0, 0, errors.New("multiple trial daemons")
		}
		pidFound, started = pid, start
	}
	if pidFound == 0 {
		return 0, 0, errors.New("trial daemon not found")
	}
	return pidFound, started, nil
}
