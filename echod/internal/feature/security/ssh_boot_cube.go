//go:build cube

package security

import "os"

// Cube boots its own marked root, not the Echo initramfs that creates slotMarker.
func sshBootAvailable() bool {
	b, err := os.ReadFile("/etc/techo5-cube-root")
	return err == nil && string(b) == "TECHO5-CUBE-ROOT-V1\n"
}
