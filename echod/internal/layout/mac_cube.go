//go:build cube && !dot && !spot && !panel

package layout

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
)

// The Lenovo board has no Amazon IDME. Derive a stable locally administered
// unicast identifier from the bootloader's device serial before Wi-Fi starts.
// This is an ESPHome identity, not a claim about the radio's factory MAC.
func cubeID(cmdline string) ([]byte, error) {
	var serial string
	for _, field := range strings.Fields(cmdline) {
		if v, ok := strings.CutPrefix(field, "androidboot.serialno="); ok {
			if serial != "" || len(v) == 0 || len(v) > 64 {
				return nil, fmt.Errorf("invalid or duplicate boot serial")
			}
			for _, c := range v {
				if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '-' || c == '_') {
					return nil, fmt.Errorf("invalid boot serial")
				}
			}
			serial = v
		}
	}
	if serial == "" {
		return nil, fmt.Errorf("boot serial unavailable")
	}
	h := sha256.Sum256([]byte("techo5-cube-device-id-v1:" + serial))
	h[0] = (h[0] & 0xfc) | 0x02
	return []byte(fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", h[0], h[1], h[2], h[3], h[4], h[5])), nil
}

func readMAC() ([]byte, error) {
	b, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return nil, err
	}
	return cubeID(string(b))
}
