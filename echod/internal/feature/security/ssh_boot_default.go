//go:build !cube

package security

import "os"

func sshBootAvailable() bool {
	_, err := os.Stat(slotMarker)
	return err == nil
}
